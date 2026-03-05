package builtin

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/antchfx/jsonquery"
	"github.com/antchfx/xmlquery"
	"github.com/antchfx/xpath"
	"github.com/dlclark/regexp2"
	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/tracing"
	"go.lsp.dev/uri"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v2"
)

type builtinServiceClient struct {
	config provider.InitConfig
	tags   map[string]bool
	provider.UnimplementedDependenciesComponent
	log logr.Logger

	cacheMutex    sync.RWMutex
	locationCache map[string]float64
	includedPaths []string
	excludedDirs  []string
	encoding      string

	workingCopyMgr *workingCopyManager

	// fileIndex is the cached list of all file paths under the base path
	fileIndex []string
	// prepared indicates Prepare() completed successfully and fileIndex is valid
	prepared bool

	// incidentCache stores pre-computed incidents keyed by (capability, search params)
	// then by file path.
	incidentCache      map[incidentCacheKey]map[string][]provider.IncidentContext
	incidentCacheMutex sync.RWMutex
	// allParsedConditions holds pre-compiled conditions from Prepare(), deduplicated
	// by cache key. Conditions with chained Filepaths are excluded (they use
	// template expansion at Evaluate time and must fall back to current behavior).
	allParsedConditions []parsedCondition
}

type fileTemplateContext struct {
	Filepaths []string `json:"filepaths,omitempty"`
}

type incidentCacheKey struct {
	cap    string
	params string
}

// parsedCondition holds a pre-parsed, pre-compiled condition ready for execution
// during Prepare()'s per-file processing. Compiling regexes and XPath expressions
// once here avoids re-compilation for every file.
type parsedCondition struct {
	cacheKey            incidentCacheKey
	cap                 string
	condition           builtinCondition
	compiledXPath       *xpath.Expr    // xml, xmlPublicID
	compiledRegex       *regexp.Regexp // xmlPublicID
	compiledFilePattern *regexp.Regexp // filecontent FilePattern, nil if empty or invalid regex
	jsonXPath           string         // json
}

var _ provider.ServiceClient = &builtinServiceClient{}

func (b *builtinServiceClient) Prepare(ctx context.Context, conditionsByCap []provider.ConditionsByCap) error {
	ctx, span := tracing.StartNewSpan(ctx, "builtin.Prepare")
	defer span.End()

	// Parse all conditions to:
	// 1. Collect the union of include scopes for the filesystem walk
	// 2. Pre-compile patterns for per-file processing during incident caching
	var allIncluded []string
	seen := map[incidentCacheKey]bool{}
	var parsedConds []parsedCondition

	for _, cbc := range conditionsByCap {
		for _, condBytes := range cbc.Conditions {
			var cond builtinCondition
			if err := yaml.Unmarshal(condBytes, &cond); err != nil {
				b.log.V(5).Error(err, "failed to unmarshal condition in Prepare, skipping")
				continue
			}
			inc, _ := cond.ProviderContext.GetScopedFilepaths()
			allIncluded = append(allIncluded, inc...)

			if cbc.Cap == "file" || cbc.Cap == "hasTags" {
				continue
			}
			if isChainedCondition(cbc.Cap, cond) {
				b.log.V(5).Info("skipping chained condition in Prepare", "cap", cbc.Cap)
				continue
			}
			key := makeIncidentCacheKey(cbc.Cap, cond)
			if seen[key] {
				continue
			}
			seen[key] = true

			pc := parsedCondition{
				cacheKey:  key,
				cap:       cbc.Cap,
				condition: cond,
			}

			// Pre-compile patterns to avoid per-file compilation cost
			switch cbc.Cap {
			case "filecontent":
				if cond.Filecontent.Pattern == "" {
					continue
				}
				if cond.Filecontent.FilePattern != "" {
					if compiled, err := regexp.Compile(cond.Filecontent.FilePattern); err == nil {
						pc.compiledFilePattern = compiled
					}
				}
			case "xml":
				compiled, err := xpath.CompileWithNS(cond.XML.XPath, cond.XML.Namespaces)
				if err != nil || compiled == nil {
					b.log.V(5).Error(err, "failed to compile xpath in Prepare, skipping", "xpath", cond.XML.XPath)
					continue
				}
				pc.compiledXPath = compiled
			case "xmlPublicID":
				compiled, err := xpath.CompileWithNS("//*[@public-id]", cond.XMLPublicID.Namespaces)
				if err != nil || compiled == nil {
					b.log.V(5).Error(err, "failed to compile xmlPublicID xpath in Prepare, skipping")
					continue
				}
				pc.compiledXPath = compiled
				regex, err := regexp.Compile(cond.XMLPublicID.Regex)
				if err != nil {
					b.log.V(5).Error(err, "failed to compile xmlPublicID regex in Prepare, skipping", "regex", cond.XMLPublicID.Regex)
					continue
				}
				pc.compiledRegex = regex
			case "json":
				if cond.JSON.XPath == "" {
					continue
				}
				pc.jsonXPath = cond.JSON.XPath
			}

			parsedConds = append(parsedConds, pc)
		}
	}

	// Build a single FileSearcher with the union of all include scopes
	searcher := provider.FileSearcher{
		BasePath: b.config.Location,
		ProviderConfigConstraints: provider.IncludeExcludeConstraints{
			IncludePathsOrPatterns: b.includedPaths,
			ExcludePathsOrPatterns: b.excludedDirs,
		},
		RuleScopeConstraints: provider.IncludeExcludeConstraints{
			IncludePathsOrPatterns: allIncluded,
		},
		FailFast: true,
		Log:      b.log,
	}

	// Single filesystem walk
	files, err := searcher.Search(provider.SearchCriteria{})
	if err != nil {
		b.log.Error(err, "failed to build file index in Prepare, falling back to per-Evaluate walks")
		return nil
	}

	b.fileIndex = files
	b.allParsedConditions = parsedConds
	b.incidentCache = make(map[incidentCacheKey]map[string][]provider.IncidentContext)

	// Process all files through worker pool to populate the incident cache.
	if len(parsedConds) > 0 && len(files) > 0 {
		const numWorkers = 5
		fileChan := make(chan string, 10)
		var wg sync.WaitGroup

		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for file := range fileChan {
					b.processFileForAllCaps(ctx, file)
				}
			}()
		}

		for _, file := range files {
			fileChan <- file
		}
		close(fileChan)
		wg.Wait()

		b.log.V(5).Info("incident cache populated during Prepare",
			"fileCount", len(files),
			"parsedConditions", len(parsedConds),
			"cacheEntries", len(b.incidentCache))
	}

	b.prepared = true
	b.log.V(5).Info("Prepare complete", "fileCount", len(files), "parsedConditions", len(parsedConds))
	return nil
}

// filterCachedFiles applies per-rule scope constraints and search criteria
// against the cached file index, returning matching files.
func (b *builtinServiceClient) filterCachedFiles(cond builtinCondition, criteria provider.SearchCriteria) ([]string, error) {
	wcIncludedPaths, wcExcludedPaths := b.getWorkingCopies()
	includedPaths, excludedPaths := cond.ProviderContext.GetScopedFilepaths()
	excludedPaths = append(excludedPaths, wcExcludedPaths...)

	searcher := provider.FileSearcher{
		BasePath:        b.config.Location,
		AdditionalPaths: wcIncludedPaths,
		ProviderConfigConstraints: provider.IncludeExcludeConstraints{
			IncludePathsOrPatterns: b.includedPaths,
			ExcludePathsOrPatterns: b.excludedDirs,
		},
		RuleScopeConstraints: provider.IncludeExcludeConstraints{
			IncludePathsOrPatterns: includedPaths,
			ExcludePathsOrPatterns: excludedPaths,
		},
		FailFast: true,
		Log:      b.log,
	}

	return searcher.SearchFromIndex(b.fileIndex, criteria)
}

// makeIncidentCacheKey builds a canonical cache key for a condition.
func makeIncidentCacheKey(cap string, cond builtinCondition) incidentCacheKey {
	switch cap {
	case "filecontent":
		return incidentCacheKey{
			cap:    cap,
			params: cond.Filecontent.Pattern + "\x00" + cond.Filecontent.FilePattern,
		}
	case "xml":
		return incidentCacheKey{
			cap:    cap,
			params: cond.XML.XPath + "\x00" + sortedNamespaces(cond.XML.Namespaces),
		}
	case "xmlPublicID":
		return incidentCacheKey{
			cap:    cap,
			params: cond.XMLPublicID.Regex + "\x00" + sortedNamespaces(cond.XMLPublicID.Namespaces),
		}
	case "json":
		return incidentCacheKey{
			cap:    cap,
			params: cond.JSON.XPath,
		}
	default:
		return incidentCacheKey{cap: cap}
	}
}

func sortedNamespaces(ns map[string]string) string {
	if len(ns) == 0 {
		return ""
	}
	keys := make([]string, 0, len(ns))
	for k := range ns {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('\x00')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(ns[k])
	}
	return b.String()
}

// isChainedCondition returns true if the condition has non-empty Filepaths,
// which are template-expanded at Evaluate() time and cannot be pre-cached.
func isChainedCondition(cap string, cond builtinCondition) bool {
	switch cap {
	case "filecontent":
		return len(cond.Filecontent.Filepaths) > 0
	case "xml":
		return len(cond.XML.Filepaths) > 0
	case "xmlPublicID":
		return len(cond.XMLPublicID.Filepaths) > 0
	case "json":
		return len(cond.JSON.Filepaths) > 0
	default:
		return false
	}
}

// mergeIntoCache thread-safely merges incidents for a given cache key and file
// into the incident cache.
func (b *builtinServiceClient) mergeIntoCache(key incidentCacheKey, filePath string, incidents []provider.IncidentContext) {
	if len(incidents) == 0 {
		return
	}
	b.incidentCacheMutex.Lock()
	defer b.incidentCacheMutex.Unlock()
	if b.incidentCache[key] == nil {
		b.incidentCache[key] = make(map[string][]provider.IncidentContext)
	}
	b.incidentCache[key][filePath] = append(b.incidentCache[key][filePath], incidents...)
}

// incidentsFromCache looks up cached incidents for a cache key, filtered to
// only include files in the provided scope set. Returns nil and false on cache miss.
func (b *builtinServiceClient) incidentsFromCache(key incidentCacheKey, scopedFiles []string) ([]provider.IncidentContext, bool) {
	b.incidentCacheMutex.RLock()
	fileMap, exists := b.incidentCache[key]
	b.incidentCacheMutex.RUnlock()
	if !exists {
		return nil, false
	}

	var result []provider.IncidentContext
	for _, f := range scopedFiles {
		if incidents, ok := fileMap[f]; ok {
			result = append(result, incidents...)
		}
	}
	return result, true
}

// matchesFilePattern checks if a file matches a pattern
func matchesFilePattern(compiledRegex *regexp.Regexp, pattern, relPath, baseName string) bool {
	// Normalize path separators for cross-platform regex matching
	normalizedRelPath := filepath.ToSlash(relPath)
	if compiledRegex != nil && (compiledRegex.MatchString(normalizedRelPath) || compiledRegex.MatchString(baseName)) {
		return true
	}
	if strings.Contains(pattern, "*") || strings.Contains(pattern, "?") {
		if m, err := filepath.Match(pattern, normalizedRelPath); err == nil && m {
			return true
		}
		if m, err := filepath.Match(pattern, baseName); err == nil && m {
			return true
		}
	}
	return false
}

// readFileContent reads a file's content, applying encoding conversion if configured
func (b *builtinServiceClient) readFileContent(filePath string) ([]byte, error) {
	if b.encoding != "" {
		content, err := engine.OpenFileWithEncoding(filePath, b.encoding)
		if err != nil {
			b.log.V(5).Error(err, "failed to convert file encoding, using original content", "file", filePath)
			return os.ReadFile(filePath)
		}
		return content, nil
	}
	return os.ReadFile(filePath)
}

// parseXMLContent parses XML content into a DOM. Extracted from queryXMLFile to
// allow parsing once and running multiple queries against the same DOM.
func (b *builtinServiceClient) parseXMLContent(filePath string, content []byte) (*xmlquery.Node, error) {
	doc, err := xmlquery.ParseWithOptions(
		strings.NewReader(string(content)),
		xmlquery.ParserOptions{Decoder: &xmlquery.DecoderOptions{Strict: false}, WithLineNumbers: true},
	)
	if err != nil {
		if err.Error() == "xml: unsupported version \"1.1\"; only version 1.0 is supported" {
			docString := strings.Replace(string(content), "<?xml version=\"1.1\"", "<?xml version = \"1.0\"", 1)
			doc, err = xmlquery.Parse(strings.NewReader(docString))
			if err != nil {
				return nil, fmt.Errorf("unable to parse xml file '%s': %w", filePath, err)
			}
		} else {
			return nil, fmt.Errorf("unable to parse xml file '%s': %w", filePath, err)
		}
	}
	return doc, nil
}

// queryXMLDoc runs an XPath query against a pre-parsed XML DOM
func queryXMLDoc(doc *xmlquery.Node, query *xpath.Expr) (nodes []*xmlquery.Node, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered panic from xpath query search with err - %v", r)
		}
	}()
	nodes = xmlquery.QuerySelectorAll(doc, query)
	return nodes, nil
}

// processFileForAllCaps reads a file once and runs all applicable pre-compiled
// condition searches against it, merging results into the incident cache.
// This is the core of the "open each file once" optimization.
func (b *builtinServiceClient) processFileForAllCaps(ctx context.Context, filePath string) {
	// Skip files that are too large
	const maxFileSize = 100 * 1024 * 1024 // 100MB
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		b.log.V(5).Error(err, "failed to stat file during Prepare", "file", filePath)
		return
	}
	if fileInfo.Size() > maxFileSize {
		b.log.V(5).Info("skipping large file during Prepare", "file", filePath, "size", fileInfo.Size())
		return
	}

	// Compute relative path from base location for pattern matching
	relPath, err := filepath.Rel(b.config.Location, filePath)
	if err != nil {
		relPath = filepath.Base(filePath)
	}
	baseName := filepath.Base(filePath)

	ext := strings.ToLower(filepath.Ext(filePath))
	isXML := ext == ".xml" || ext == ".xhtml"
	isJSON := ext == ".json"

	// Determine which capabilities apply to this file
	needsContent := false
	needsXML := false
	needsJSON := false

	for i := range b.allParsedConditions {
		pc := &b.allParsedConditions[i]
		switch pc.cap {
		case "filecontent":
			// filecontent applies to all files, optionally filtered by FilePattern
			if pc.condition.Filecontent.FilePattern != "" {
				if !matchesFilePattern(pc.compiledFilePattern, pc.condition.Filecontent.FilePattern, relPath, baseName) {
					continue
				}
			}
			needsContent = true
		case "xml", "xmlPublicID":
			if isXML {
				needsXML = true
			}
		case "json":
			if isJSON {
				needsJSON = true
			}
		}
	}

	// Nothing to do for this file
	if !needsContent && !needsXML && !needsJSON {
		return
	}

	// Read file content once
	content, err := b.readFileContent(filePath)
	if err != nil {
		b.log.V(5).Error(err, "failed to read file during Prepare", "file", filePath)
		return
	}

	// Process filecontent conditions
	if needsContent {
		b.processFilecontentForFile(filePath, content)
	}

	// Parse and process XML conditions
	if needsXML {
		doc, err := b.parseXMLContent(filePath, content)
		if err != nil {
			b.log.V(5).Error(err, "failed to parse XML during Prepare", "file", filePath)
		} else {
			b.processXMLForFile(ctx, filePath, doc)
		}
	}

	// Parse and process JSON conditions
	if needsJSON {
		doc, err := jsonquery.Parse(bytes.NewReader(content))
		if err != nil {
			b.log.V(5).Error(err, "failed to parse JSON during Prepare", "file", filePath)
		} else {
			b.processJSONForFile(ctx, filePath, doc)
		}
	}
}

// processFilecontentForFile runs all filecontent conditions against pre-read content.
func (b *builtinServiceClient) processFilecontentForFile(filePath string, content []byte) {
	relPath, err := filepath.Rel(b.config.Location, filePath)
	if err != nil {
		relPath = filepath.Base(filePath)
	}
	baseName := filepath.Base(filePath)

	for i := range b.allParsedConditions {
		pc := &b.allParsedConditions[i]
		if pc.cap != "filecontent" {
			continue
		}

		// Check file pattern filter
		if pc.condition.Filecontent.FilePattern != "" {
			if !matchesFilePattern(pc.compiledFilePattern, pc.condition.Filecontent.FilePattern, relPath, baseName) {
				continue
			}
		}

		pattern := pc.condition.Filecontent.Pattern
		trimmedPattern := strings.TrimPrefix(pattern, "\"")
		if !strings.HasSuffix(trimmedPattern, `\"`) {
			trimmedPattern = strings.TrimSuffix(trimmedPattern, `"`)
		}

		// Use the existing multiline search logic on pre-read content
		matches := b.searchContentForPattern(filePath, content, trimmedPattern)
		if len(matches) == 0 {
			continue
		}

		incidents := make([]provider.IncidentContext, 0, len(matches))
		for _, match := range matches {
			lineNumber := int(match.positionParams.Position.Line)
			incidents = append(incidents, provider.IncidentContext{
				FileURI:    uri.URI(match.positionParams.TextDocument.URI),
				LineNumber: &lineNumber,
				Variables: map[string]interface{}{
					"matchingText": match.match,
				},
				CodeLocation: &provider.Location{
					StartPosition: provider.Position{Line: float64(lineNumber)},
					EndPosition:   provider.Position{Line: float64(lineNumber)},
				},
			})
		}
		b.mergeIntoCache(pc.cacheKey, filePath, incidents)
	}
}

// searchContentForPattern searches pre-read content for a pattern.
func (b *builtinServiceClient) searchContentForPattern(filePath string, content []byte, trimmedPattern string) []fileSearchResult {
	// Check if pattern is literal
	isLiteral := true
	for _, ch := range trimmedPattern {
		if strings.ContainsRune(".*+?^$[]{}()|\\", ch) {
			isLiteral = false
			break
		}
	}

	if isLiteral {
		return b.searchContentLiteral(filePath, content, trimmedPattern)
	}

	// Regex path — try standard regexp first
	fullPattern := `(?m)` + trimmedPattern
	stdRegex, err := regexp.Compile(fullPattern)
	if err != nil {
		// Fall back to regexp2
		patternRegex, err := regexp2.Compile(fullPattern, regexp2.Multiline)
		if err != nil {
			b.log.V(5).Error(err, "failed to compile pattern during Prepare", "pattern", trimmedPattern)
			return nil
		}
		return b.searchContentRegexp2(filePath, content, patternRegex)
	}

	if !stdRegex.Match(content) {
		return nil
	}

	matches := stdRegex.FindAllIndex(content, -1)
	if len(matches) == 0 {
		return nil
	}

	results := make([]fileSearchResult, 0, len(matches))
	lineNumber := 1
	lineStart := 0
	lastPos := 0

	for _, match := range matches {
		matchStart := match[0]
		matchEnd := match[1]
		for i := lastPos; i < matchStart; i++ {
			if content[i] == '\n' {
				lineNumber++
				lineStart = i + 1
			}
		}
		lastPos = matchStart
		charNumber := matchStart - lineStart
		results = append(results, fileSearchResult{
			positionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{
					URI: protocol.DocumentURI(uri.File(filePath)),
				},
				Position: protocol.Position{
					Line:      uint32(lineNumber),
					Character: uint32(charNumber),
				},
			},
			match: string(content[matchStart:matchEnd]),
		})
	}
	return results
}

func (b *builtinServiceClient) searchContentLiteral(filePath string, content []byte, pattern string) []fileSearchResult {
	literalBytes := []byte(pattern)
	var matches [][]int
	searchPos := 0
	for {
		idx := bytes.Index(content[searchPos:], literalBytes)
		if idx == -1 {
			break
		}
		actualPos := searchPos + idx
		matches = append(matches, []int{actualPos, actualPos + len(literalBytes)})
		searchPos = actualPos + 1
	}
	if len(matches) == 0 {
		return nil
	}

	results := make([]fileSearchResult, 0, len(matches))
	lineNumber := 1
	lineStart := 0
	lastPos := 0
	for _, match := range matches {
		matchStart := match[0]
		matchEnd := match[1]
		for i := lastPos; i < matchStart; i++ {
			if content[i] == '\n' {
				lineNumber++
				lineStart = i + 1
			}
		}
		lastPos = matchStart
		charNumber := matchStart - lineStart
		results = append(results, fileSearchResult{
			positionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{
					URI: protocol.DocumentURI(uri.File(filePath)),
				},
				Position: protocol.Position{
					Line:      uint32(lineNumber),
					Character: uint32(charNumber),
				},
			},
			match: string(content[matchStart:matchEnd]),
		})
	}
	return results
}

func (b *builtinServiceClient) searchContentRegexp2(filePath string, content []byte, regex *regexp2.Regexp) []fileSearchResult {
	contentStr := string(content)
	match, err := regex.FindStringMatch(contentStr)
	if err != nil {
		return nil
	}

	var results []fileSearchResult
	lineNumber := 1
	lineStart := 0
	lastPos := 0

	for match != nil {
		for i := lastPos; i < match.Index; i++ {
			if contentStr[i] == '\n' {
				lineNumber++
				lineStart = i + 1
			}
		}
		lastPos = match.Index
		charNumber := match.Index - lineStart
		results = append(results, fileSearchResult{
			positionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{
					URI: protocol.DocumentURI(uri.File(filePath)),
				},
				Position: protocol.Position{
					Line:      uint32(lineNumber),
					Character: uint32(charNumber),
				},
			},
			match: match.String(),
		})
		match, err = regex.FindNextMatch(match)
		if err != nil {
			break
		}
	}
	return results
}

// processXMLForFile runs all xml and xmlPublicID conditions against a pre-parsed XML DOM.
func (b *builtinServiceClient) processXMLForFile(ctx context.Context, filePath string, doc *xmlquery.Node) {
	for i := range b.allParsedConditions {
		pc := &b.allParsedConditions[i]
		if pc.cap != "xml" && pc.cap != "xmlPublicID" {
			continue
		}
		if pc.compiledXPath == nil {
			continue
		}

		nodes, err := queryXMLDoc(doc, pc.compiledXPath)
		if err != nil {
			b.log.V(5).Error(err, "failed to query XML during Prepare", "file", filePath, "cap", pc.cap)
			continue
		}

		if pc.cap == "xml" {
			if len(nodes) == 0 {
				continue
			}
			incidents := make([]provider.IncidentContext, 0, len(nodes))
			for _, node := range nodes {
				incident := provider.IncidentContext{
					FileURI: uri.File(filePath),
					Variables: map[string]interface{}{
						"matchingXML": compactXML(node.OutputXML(false)),
						"innerText":   node.InnerText(),
						"data":        node.Data,
					},
				}
				content := strings.TrimSpace(node.InnerText())
				if content == "" {
					content = node.Data
				}
				location, err := b.getLocation(ctx, filePath, content)
				if err == nil {
					incident.CodeLocation = &location
					lineNo := int(location.StartPosition.Line)
					incident.LineNumber = &lineNo
				} else {
					lineNum := node.LineNumber
					incident.LineNumber = &lineNum
				}
				incidents = append(incidents, incident)
			}
			b.mergeIntoCache(pc.cacheKey, filePath, incidents)
		} else {
			// xmlPublicID: filter by public-id attribute regex
			var incidents []provider.IncidentContext
			for _, node := range nodes {
				for _, attr := range node.Attr {
					if attr.Name.Local == "public-id" {
						if pc.compiledRegex != nil && pc.compiledRegex.MatchString(attr.Value) {
							incidents = append(incidents, provider.IncidentContext{
								FileURI: uri.File(filePath),
								Variables: map[string]interface{}{
									"matchingXML": compactXML(node.OutputXML(false)),
									"innerText":   node.InnerText(),
									"data":        node.Data,
								},
							})
						}
						break
					}
				}
			}
			b.mergeIntoCache(pc.cacheKey, filePath, incidents)
		}
	}
}

// processJSONForFile runs all json conditions against a pre-parsed JSON DOM.
func (b *builtinServiceClient) processJSONForFile(ctx context.Context, filePath string, doc *jsonquery.Node) {
	for i := range b.allParsedConditions {
		pc := &b.allParsedConditions[i]
		if pc.cap != "json" || pc.jsonXPath == "" {
			continue
		}

		list, err := jsonquery.QueryAll(doc, pc.jsonXPath)
		if err != nil {
			b.log.V(5).Error(err, "failed to query JSON during Prepare", "file", filePath)
			continue
		}
		if len(list) == 0 {
			continue
		}

		incidents := make([]provider.IncidentContext, 0, len(list))
		for _, node := range list {
			incident := provider.IncidentContext{
				FileURI: uri.File(filePath),
				Variables: map[string]interface{}{
					"matchingJSON": node.InnerText(),
					"data":         node.Data,
				},
			}
			location, err := b.getLocation(ctx, filePath, node.InnerText())
			if err == nil {
				incident.CodeLocation = &location
				lineNo := int(location.StartPosition.Line)
				incident.LineNumber = &lineNo
			}
			incidents = append(incidents, incident)
		}
		b.mergeIntoCache(pc.cacheKey, filePath, incidents)
	}
}

func (b *builtinServiceClient) openFileWithEncoding(filePath string) (io.Reader, error) {
	var content []byte
	var err error
	if b.encoding != "" {
		content, err = engine.OpenFileWithEncoding(filePath, b.encoding)
		if err != nil {
			b.log.V(5).Error(err, "failed to convert file encoding, using original content", "file", filePath)
			content, readErr := os.ReadFile(filePath)
			if readErr != nil {
				return nil, readErr
			}
			return bytes.NewReader(content), nil
		}
	} else {
		content, err = os.ReadFile(filePath)
		if err != nil {
			return nil, err
		}
	}

	return bytes.NewReader(content), nil
}

func (p *builtinServiceClient) Stop() {
	p.workingCopyMgr.stop()
}

func (p *builtinServiceClient) NotifyFileChanges(ctx context.Context, changes ...provider.FileChange) error {
	filtered := []provider.FileChange{}
	for _, change := range changes {
		if strings.HasPrefix(change.Path, p.config.Location) {
			filtered = append(filtered, change)
		}
	}
	p.workingCopyMgr.notifyChanges(filtered...)
	return nil
}

func (p *builtinServiceClient) Evaluate(ctx context.Context, cap string, conditionInfo []byte) (provider.ProviderEvaluateResponse, error) {
	var cond builtinCondition
	err := yaml.Unmarshal(conditionInfo, &cond)
	if err != nil {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf("unable to get query info: %v", err)
	}
	log := p.log.WithValues("ruleID", cond.ProviderContext.RuleID)
	log.V(5).Info("builtin condition context", "condition", cond, "provider context", cond.ProviderContext)
	response := provider.ProviderEvaluateResponse{Matched: false}

	// searchFiles resolves file lists either from the cached index (if Prepare()
	// was called) or by creating a fresh FileSearcher and walking the filesystem.
	searchFiles := func(criteria provider.SearchCriteria) ([]string, error) {
		if p.prepared {
			return p.filterCachedFiles(cond, criteria)
		}
		// Fallback: create FileSearcher per call (original behavior)
		wcIncludedPaths, wcExcludedPaths := p.getWorkingCopies()
		includedPaths, excludedPaths := cond.ProviderContext.GetScopedFilepaths()
		excludedPaths = append(excludedPaths, wcExcludedPaths...)
		fileSearcher := provider.FileSearcher{
			BasePath:        p.config.Location,
			AdditionalPaths: wcIncludedPaths,
			ProviderConfigConstraints: provider.IncludeExcludeConstraints{
				IncludePathsOrPatterns: p.includedPaths,
				ExcludePathsOrPatterns: p.excludedDirs,
			},
			RuleScopeConstraints: provider.IncludeExcludeConstraints{
				IncludePathsOrPatterns: includedPaths,
				ExcludePathsOrPatterns: excludedPaths,
			},
			FailFast: true,
			Log:      p.log,
		}
		return fileSearcher.Search(criteria)
	}

	switch cap {
	case "file":
		c := cond.File
		if c.Pattern == "" {
			return response, fmt.Errorf("could not parse provided file pattern as string: %v", conditionInfo)
		}
		matchingFiles, err := searchFiles(provider.SearchCriteria{
			Patterns: []string{c.Pattern},
		})
		if err != nil {
			return response, fmt.Errorf("failed to search for files - %w", err)
		}
		response.TemplateContext = map[string]interface{}{"filepaths": matchingFiles}
		for _, match := range matchingFiles {
			absPath := match
			if !filepath.IsAbs(match) {
				absPath, err = filepath.Abs(match)
				if err != nil {
					p.log.V(5).Error(err, "failed to get absolute path to file", "path", match)
					absPath = match
				}
			}
			response.Incidents = append(response.Incidents, provider.IncidentContext{
				FileURI: uri.File(absPath),
			})
		}
		response.Incidents = p.workingCopyMgr.reformatIncidents(response.Incidents...)
		response.Matched = len(response.Incidents) > 0
		return response, nil
	case "filecontent":
		c := cond.Filecontent
		if c.Pattern == "" {
			return response, fmt.Errorf("could not parse provided regex pattern as string: %v", conditionInfo)
		}

		// Try incident cache lookup
		if p.prepared && len(p.allParsedConditions) > 0 && !isChainedCondition(cap, cond) {
			key := makeIncidentCacheKey(cap, cond)
			var patterns []string
			if c.FilePattern != "" {
				patterns = []string{c.FilePattern}
			}
			scopedFiles, err := p.filterCachedFiles(cond, provider.SearchCriteria{
				Patterns: patterns,
			})
			if err != nil {
				return response, fmt.Errorf("failed to filter cached files - %w", err)
			}
			if incidents, ok := p.incidentsFromCache(key, scopedFiles); ok {
				response.Incidents = p.workingCopyMgr.reformatIncidents(incidents...)
				response.Matched = len(response.Incidents) > 0
				return response, nil
			}
		}

		// Fallback: full search (chained condition, cache miss, or not prepared)
		patterns := []string{}
		if c.FilePattern != "" {
			patterns = append(patterns, c.FilePattern)
		}
		filePaths, err := searchFiles(provider.SearchCriteria{
			Patterns:           patterns,
			ConditionFilepaths: c.Filepaths,
		})
		if err != nil {
			return response, fmt.Errorf("failed to perform search - %w", err)
		}

		matches, err := p.performFileContentSearch(c.Pattern, filePaths)
		if err != nil {
			return response, fmt.Errorf("failed to perform file content search - %w", err)
		}

		for _, match := range matches {
			lineNumber := int(match.positionParams.Position.Line)

			response.Incidents = append(response.Incidents, provider.IncidentContext{
				FileURI:    uri.URI(match.positionParams.TextDocument.URI),
				LineNumber: &lineNumber,
				Variables: map[string]interface{}{
					"matchingText": match.match,
				},
				CodeLocation: &provider.Location{
					StartPosition: provider.Position{Line: float64(lineNumber)},
					EndPosition:   provider.Position{Line: float64(lineNumber)},
				},
			})
		}
		if len(response.Incidents) != 0 {
			response.Matched = true
		}
		response.Incidents = p.workingCopyMgr.reformatIncidents(response.Incidents...)
		return response, nil
	case "xml":
		query, err := xpath.CompileWithNS(cond.XML.XPath, cond.XML.Namespaces)
		if query == nil || err != nil {
			return response, fmt.Errorf("could not parse provided xpath query '%s': %v", cond.XML.XPath, err)
		}

		// Try incident cache lookup
		if p.prepared && len(p.allParsedConditions) > 0 && !isChainedCondition(cap, cond) {
			key := makeIncidentCacheKey(cap, cond)
			scopedFiles, err := p.filterCachedFiles(cond, provider.SearchCriteria{
				Patterns: []string{"*.xml", "*.xhtml"},
			})
			if err != nil {
				return response, fmt.Errorf("unable to find XML files: %v", err)
			}
			if incidents, ok := p.incidentsFromCache(key, scopedFiles); ok {
				response.Incidents = p.workingCopyMgr.reformatIncidents(incidents...)
				response.Matched = len(response.Incidents) > 0
				return response, nil
			}
		}

		// Fallback
		xmlFiles, err := searchFiles(provider.SearchCriteria{
			Patterns:           []string{"*.xml", "*.xhtml"},
			ConditionFilepaths: cond.XML.Filepaths,
		})
		if err != nil {
			return response, fmt.Errorf("unable to find XML files: %v", err)
		}
		for _, file := range xmlFiles {
			nodes, err := p.queryXMLFile(file, query)
			if err != nil {
				log.V(5).Error(err, "failed to query xml file", "file", file)
				continue
			}
			if len(nodes) != 0 {
				response.Matched = true
				for _, node := range nodes {
					absPath, err := filepath.Abs(file)
					if err != nil {
						absPath = file
					}
					incident := provider.IncidentContext{
						FileURI: uri.File(absPath),
						Variables: map[string]interface{}{
							"matchingXML": compactXML(node.OutputXML(false)),
							"innerText":   node.InnerText(),
							"data":        node.Data,
						},
					}
					content := strings.TrimSpace(node.InnerText())
					if content == "" {
						content = node.Data
					}
					location, err := p.getLocation(ctx, absPath, content)
					if err == nil {
						incident.CodeLocation = &location
						lineNo := int(location.StartPosition.Line)
						incident.LineNumber = &lineNo
					} else {
						lineNum := node.LineNumber
						incident.LineNumber = &lineNum
					}
					response.Incidents = append(response.Incidents, incident)
				}
			}
		}
		response.Incidents = p.workingCopyMgr.reformatIncidents(response.Incidents...)
		return response, nil
	case "xmlPublicID":
		regex, err := regexp.Compile(cond.XMLPublicID.Regex)
		if err != nil {
			return response, fmt.Errorf("could not parse provided public-id regex '%s': %v", cond.XMLPublicID.Regex, err)
		}
		query, err := xpath.CompileWithNS("//*[@public-id]", cond.XMLPublicID.Namespaces)
		if query == nil || err != nil {
			return response, fmt.Errorf("could not parse public-id xml query '%s': %v", cond.XML.XPath, err)
		}

		// Try incident cache lookup
		if p.prepared && len(p.allParsedConditions) > 0 && !isChainedCondition(cap, cond) {
			key := makeIncidentCacheKey(cap, cond)
			scopedFiles, err := p.filterCachedFiles(cond, provider.SearchCriteria{
				Patterns: []string{"*.xml", "*.xhtml"},
			})
			if err != nil {
				return response, fmt.Errorf("unable to find XML files: %v", err)
			}
			if incidents, ok := p.incidentsFromCache(key, scopedFiles); ok {
				response.Incidents = p.workingCopyMgr.reformatIncidents(incidents...)
				response.Matched = len(response.Incidents) > 0
				return response, nil
			}
		}

		// Fallback
		xmlFiles, err := searchFiles(provider.SearchCriteria{
			Patterns:           []string{"*.xml", "*.xhtml"},
			ConditionFilepaths: cond.XMLPublicID.Filepaths,
		})
		if err != nil {
			return response, fmt.Errorf("unable to find XML files: %v", err)
		}
		for _, file := range xmlFiles {
			nodes, err := p.queryXMLFile(file, query)
			if err != nil {
				log.Error(err, "failed to query xml file", "file", file)
				continue
			}

			for _, node := range nodes {
				// public-id attribute regex match check
				for _, attr := range node.Attr {
					if attr.Name.Local == "public-id" {
						if regex.MatchString(attr.Value) {
							response.Matched = true
							absPath, err := filepath.Abs(file)
							if err != nil {
								absPath = file
							}
							response.Incidents = append(response.Incidents, provider.IncidentContext{
								FileURI: uri.File(absPath),
								Variables: map[string]interface{}{
									"matchingXML": compactXML(node.OutputXML(false)),
									"innerText":   node.InnerText(),
									"data":        node.Data,
								},
							})
						}
						break
					}
				}
			}
		}
		response.Incidents = p.workingCopyMgr.reformatIncidents(response.Incidents...)
		return response, nil
	case "json":
		query := cond.JSON.XPath
		if query == "" {
			return response, fmt.Errorf("could not parse provided xpath query as string: %v", conditionInfo)
		}

		// Try incident cache lookup
		if p.prepared && len(p.allParsedConditions) > 0 && !isChainedCondition(cap, cond) {
			key := makeIncidentCacheKey(cap, cond)
			scopedFiles, err := p.filterCachedFiles(cond, provider.SearchCriteria{
				Patterns: []string{"*.json"},
			})
			if err != nil {
				return response, fmt.Errorf("unable to find JSON files: %v", err)
			}
			if incidents, ok := p.incidentsFromCache(key, scopedFiles); ok {
				response.Incidents = p.workingCopyMgr.reformatIncidents(incidents...)
				response.Matched = len(response.Incidents) > 0
				return response, nil
			}
		}

		// Fallback
		jsonFiles, err := searchFiles(provider.SearchCriteria{
			Patterns:           []string{"*.json"},
			ConditionFilepaths: cond.JSON.Filepaths,
		})
		if err != nil {
			return response, fmt.Errorf("unable to find JSON files: %v", err)
		}
		for _, file := range jsonFiles {
			reader, err := p.openFileWithEncoding(file)
			if err != nil {
				log.V(5).Error(err, "error opening json file", "file", file)
				continue
			}
			doc, err := jsonquery.Parse(reader)
			if err != nil {
				log.V(5).Error(err, "error parsing json file", "file", file)
				continue
			}
			list, err := jsonquery.QueryAll(doc, query)
			if err != nil {
				return response, err
			}
			if len(list) != 0 {
				response.Matched = true
				for _, node := range list {
					absPath, err := filepath.Abs(file)
					if err != nil {
						absPath = file
					}
					incident := provider.IncidentContext{
						FileURI: uri.File(absPath),
						Variables: map[string]interface{}{
							"matchingJSON": node.InnerText(),
							"data":         node.Data,
						},
					}
					location, err := p.getLocation(ctx, absPath, node.InnerText())
					if err == nil {
						incident.CodeLocation = &location
						lineNo := int(location.StartPosition.Line)
						incident.LineNumber = &lineNo
					}
					response.Incidents = append(response.Incidents, incident)
				}
			}
		}
		response.Incidents = p.workingCopyMgr.reformatIncidents(response.Incidents...)
		return response, nil
	case "hasTags":
		found := true
		for _, tag := range cond.HasTags {
			if _, exists := cond.ProviderContext.Tags[tag]; !exists {
				if _, exists := p.tags[tag]; !exists {
					found = false
					break
				}
			}
		}
		if found {
			response.Matched = true
			response.Incidents = append(response.Incidents, provider.IncidentContext{
				Variables: map[string]interface{}{
					"tags": cond.HasTags,
				},
			})
		}
		return response, nil
	default:
		return response, fmt.Errorf("capability must be one of %v, not %s", capabilities, cap)
	}
}

// getLocation attempts to get code location for given content in JSON / XML files
func (b *builtinServiceClient) getLocation(ctx context.Context, path, content string) (provider.Location, error) {
	ctx, span := tracing.StartNewSpan(ctx, "getLocation")
	defer span.End()
	location := provider.Location{}

	parts := strings.Split(content, "\n")
	if len(parts) < 1 {
		return location, fmt.Errorf("unable to get code location, empty content")
	} else if len(parts) > 5 {
		// limit content to search
		parts = parts[:5]
	}
	lines := []string{}
	for _, part := range parts {
		line := strings.Trim(part, " ")
		line = strings.ReplaceAll(line, "\t", "")
		line = regexp.QuoteMeta(line)
		lines = append(lines, line)
	}
	// remove leading and trailing empty lines
	if len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) < 1 || strings.Join(lines, "") == "" {
		return location, fmt.Errorf("unable to get code location, empty content")
	}
	pattern := fmt.Sprintf(".*?%s", strings.Join(lines, ".*?"))
	cacheKey := fmt.Sprintf("%s-%s", path, pattern)
	b.cacheMutex.RLock()
	val, exists := b.locationCache[cacheKey]
	b.cacheMutex.RUnlock()
	if exists {
		if val == -1 {
			return location, fmt.Errorf("unable to get location due to a previous error")
		}
		return provider.Location{
			StartPosition: provider.Position{
				Line: float64(val),
			},
			EndPosition: provider.Position{
				Line: float64(val),
			},
		}, nil
	}

	defer func() {
		b.cacheMutex.Lock()
		b.locationCache[cacheKey] = location.StartPosition.Line
		b.cacheMutex.Unlock()
	}()

	location.StartPosition.Line = -1
	lineNumber, err := provider.MultilineGrep(ctx, len(lines), path, pattern)
	if err != nil || lineNumber == -1 {
		return location, fmt.Errorf("unable to get location in file %s - %w", path, err)
	}
	location.StartPosition.Line = float64(lineNumber)
	location.EndPosition.Line = float64(lineNumber)
	return location, nil
}

// compactXML strips formatting from XML output to produce single-line compact
// format to match to previous behavior of xmlquery (pre-1.4 release)
func compactXML(xml string) string {
	// Use regex to remove whitespace between XML tags
	re := regexp.MustCompile(`>\s+<`)
	compacted := re.ReplaceAllString(xml, "><")

	// Remove leading/trailing whitespace
	compacted = strings.TrimSpace(compacted)

	return compacted
}

func (b *builtinServiceClient) queryXMLFile(filePath string, query *xpath.Expr) (nodes []*xmlquery.Node, err error) {
	var content []byte
	if b.encoding != "" {
		content, err = engine.OpenFileWithEncoding(filePath, b.encoding)
		if err != nil {
			b.log.V(5).Error(err, "failed to convert file encoding for XML, using original file", "file", filePath)
			content, err = os.ReadFile(filePath)
			if err != nil {
				return nil, err
			}
		}
	} else {
		content, err = os.ReadFile(filePath)
		if err != nil {
			return nil, err
		}
	}

	// TODO This should start working if/when this merges and releases: https://github.com/golang/go/pull/56848
	var doc *xmlquery.Node
	doc, err = xmlquery.ParseWithOptions(strings.NewReader(string(content)), xmlquery.ParserOptions{Decoder: &xmlquery.DecoderOptions{Strict: false}, WithLineNumbers: true})
	if err != nil {
		if err.Error() == "xml: unsupported version \"1.1\"; only version 1.0 is supported" {
			// TODO HACK just pretend 1.1 xml documents are 1.0 for now while we wait for golang to support 1.1
			docString := strings.Replace(string(content), "<?xml version=\"1.1\"", "<?xml version = \"1.0\"", 1)
			doc, err = xmlquery.Parse(strings.NewReader(docString))
			if err != nil {
				return nil, fmt.Errorf("unable to parse xml file '%s': %w", filePath, err)
			}
		} else {
			return nil, fmt.Errorf("unable to parse xml file '%s': %w", filePath, err)
		}
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered panic from xpath query search with err - %v", r)
		}
	}()
	nodes = xmlquery.QuerySelectorAll(doc, query)
	return nodes, err
}

func (b *builtinServiceClient) getWorkingCopies() ([]string, []string) {
	additionalIncludedPaths := []string{}
	excludedPaths := []string{}
	for _, wc := range b.workingCopyMgr.getWorkingCopies() {
		additionalIncludedPaths = append(additionalIncludedPaths, wc.wcPath)
		if runtime.GOOS == "windows" && wc.drive != "" {
			path := filepath.Join(wc.drive, wc.filePath[1:])
			excludedPaths = append(excludedPaths, path)
		} else {
			excludedPaths = append(excludedPaths, wc.filePath)
		}
	}
	return additionalIncludedPaths, excludedPaths
}

type fileSearchResult struct {
	positionParams protocol.TextDocumentPositionParams
	match          string
}

func (b *builtinServiceClient) performFileContentSearch(pattern string, locations []string) ([]fileSearchResult, error) {
	// Trim quotes around the pattern to keep backwards compatibility
	trimmedPattern := strings.TrimPrefix(pattern, "\"")
	if !strings.HasSuffix(trimmedPattern, `\"`) {
		trimmedPattern = strings.TrimSuffix(trimmedPattern, `"`)
	}

	// Check if the pattern needs multiline support
	// Patterns need multiline support if they:
	// 1. Explicitly reference newlines (\n, \r)
	// 2. Use regex flags for multiline/dotall ((?s), (?m))
	// 3. Use whitespace patterns that include newlines (\s)
	// 4. Use negated character classes for markup matching ([^>], [^<])
	//    These are common in HTML/XML/JSX patterns and can span newlines
	// Note: This is a conservative heuristic. Patterns not matching these criteria
	// will use the faster grep-based search which processes files line-by-line.
	needsMultiline := strings.Contains(trimmedPattern, "\\n") ||
		strings.Contains(trimmedPattern, "\\r") ||
		strings.Contains(trimmedPattern, "(?s)") ||
		strings.Contains(trimmedPattern, "(?m)") ||
		strings.Contains(trimmedPattern, "\\s") || // whitespace (includes newlines)
		strings.Contains(trimmedPattern, "[^>") || // negated char class for markup (e.g., [^>]*)
		strings.Contains(trimmedPattern, "[^<") // negated char class for markup (e.g., [^<]*)

	b.log.V(5).Info("analyzing pattern", "pattern", pattern, "needsMultiline", needsMultiline)

	// For multiline patterns, use the new Go-based implementation
	if needsMultiline {
		return b.performMultilineSearch(trimmedPattern, locations)
	}

	// For simple patterns, use the old grep/perl implementation for performance
	return b.performGrepSearch(pattern, trimmedPattern, locations)
}

func (b *builtinServiceClient) performMultilineSearch(trimmedPattern string, locations []string) ([]fileSearchResult, error) {
	// Check if pattern is a literal string (no regex metacharacters)
	// This allows us to use fast bytes.Index() instead of regexp
	isLiteral := true
	for _, ch := range trimmedPattern {
		if strings.ContainsRune(".*+?^$[]{}()|\\", ch) {
			isLiteral = false
			break
		}
	}

	b.log.V(7).Info("pattern", "trimmed pattern", trimmedPattern, "isLiteral", isLiteral)

	var stdRegex *regexp.Regexp
	var patternRegex *regexp2.Regexp

	if !isLiteral {
		// Try to compile with standard regexp first (better performance)
		// This is nil if the pattern uses regexp2-specific features

		// To match behavior of the xargs searching, we will set multimode line to true
		trimmedPattern = `(?m)` + trimmedPattern
		var stdErr error
		stdRegex, stdErr = regexp.Compile(trimmedPattern)

		if stdErr != nil {
			// Pattern uses regexp2-specific features, compile with regexp2
			var err error
			patternRegex, err = regexp2.Compile(trimmedPattern, regexp2.Multiline)
			if err != nil {
				return nil, fmt.Errorf("could not compile provided regex pattern '%s': %v", trimmedPattern, err)
			}
		}
	}

	b.log.V(5).Info("searching for multiline pattern using parallelWalk", "pattern", trimmedPattern, "totalFiles", len(locations), "literal", isLiteral)

	matches, err := b.parallelWalkWithLiteralCheck(locations, patternRegex, stdRegex, trimmedPattern, isLiteral)
	if err != nil {
		return nil, fmt.Errorf("failed to perform search - %w", err)
	}
	return matches, nil
}

func (b *builtinServiceClient) parallelWalkWithLiteralCheck(paths []string, regex *regexp2.Regexp, stdRegex *regexp.Regexp, literalPattern string, isLiteral bool) ([]fileSearchResult, error) {
	var positions []fileSearchResult
	var positionsMu sync.Mutex
	var eg errgroup.Group

	// Set a parallelism limit to avoid hitting limits related to opening too many files.
	// On Windows, this can show up as a runtime failure due to a thread limit.
	eg.SetLimit(20)

	for _, filePath := range paths {
		eg.Go(func() error {
			pos, err := b.processFileWithLiteralCheck(filePath, regex, stdRegex, literalPattern, isLiteral)
			if err != nil {
				return err
			}

			if len(pos) == 0 {
				return nil
			}

			positionsMu.Lock()
			defer positionsMu.Unlock()
			positions = append(positions, pos...)
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return positions, nil
}

func (b *builtinServiceClient) processFileWithLiteralCheck(path string, regex *regexp2.Regexp, stdRegex *regexp.Regexp, literalPattern string, isLiteral bool) ([]fileSearchResult, error) {
	// Check file size before loading to prevent OOM on large files (e.g., JARs)
	const maxFileSize = 100 * 1024 * 1024 // 100MB limit
	fileInfo, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if fileInfo.Size() > maxFileSize {
		b.log.V(5).Info("skipping large file", "file", path, "size", fileInfo.Size(), "limit", maxFileSize)
		return []fileSearchResult{}, nil
	}

	var content []byte
	if b.encoding != "" {
		content, err = engine.OpenFileWithEncoding(path, b.encoding)
		if err != nil {
			b.log.V(5).Error(err, "failed to convert file encoding, using original content", "file", path)
			content, err = os.ReadFile(path)
			if err != nil {
				return nil, err
			}
		}
	} else {
		content, err = os.ReadFile(path)
		if err != nil {
			return nil, err
		}
	}

	// Get absolute path for results
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	// Pre-allocate result slice
	var r []fileSearchResult

	// Fast path for literal string search using bytes.Index()
	if isLiteral {
		literalBytes := []byte(literalPattern)
		searchPos := 0
		matches := [][]int{}

		for {
			idx := bytes.Index(content[searchPos:], literalBytes)
			if idx == -1 {
				break
			}
			actualPos := searchPos + idx
			matches = append(matches, []int{actualPos, actualPos + len(literalBytes)})
			searchPos = actualPos + 1
		}

		if len(matches) == 0 {
			return []fileSearchResult{}, nil
		}

		r = make([]fileSearchResult, 0, len(matches))

		// Calculate line numbers incrementally (1-based for display)
		lineNumber := 1
		lineStart := 0
		lastPos := 0

		for _, match := range matches {
			matchStart := match[0]
			matchEnd := match[1]

			// Count newlines only from last position to current match
			for i := lastPos; i < matchStart; i++ {
				if content[i] == '\n' {
					lineNumber++
					lineStart = i + 1
				}
			}
			lastPos = matchStart

			// Character position as byte offset from start of line
			charNumber := matchStart - lineStart

			r = append(r, fileSearchResult{
				positionParams: protocol.TextDocumentPositionParams{
					TextDocument: protocol.TextDocumentIdentifier{
						URI: protocol.DocumentURI(uri.File(absPath)),
					},
					Position: protocol.Position{
						Line:      uint32(lineNumber),
						Character: uint32(charNumber),
					},
				},
				match: string(content[matchStart:matchEnd]),
			})
		}

		return r, nil
	}

	// Fast pre-check: try to use Go's standard regexp for quick byte-based filtering
	// This avoids string allocation for files without matches
	foundMatch := false

	if stdRegex != nil {
		b.log.V(7).Info("using golang regex", "pattern", literalPattern)
		if runtime.GOOS == "windows" {
			b.log.V(7).Info("remove CRLF line endings")
			if bytes.Contains(content, []byte("\r\n")) {
				content = bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
			}
		}
		// Use byte-based matching for pre-check (no allocation)
		foundMatch = stdRegex.Match(content)
	} else {
		b.log.V(7).Info("using regexp2 regex", "pattern", literalPattern)
		// Pattern uses regexp2-specific features, fall back to regexp2 check
		// Use chunked approach to limit memory usage
		const chunkSize = 1024 * 1024 // 1MB chunks
		const overlap = 8 * 1024      // 8KB overlap to catch patterns spanning chunk boundaries

		for offset := 0; offset < len(content); offset += chunkSize - overlap {
			end := offset + chunkSize
			if end > len(content) {
				end = len(content)
			}
			chunk := content[offset:end]

			ok, err := regex.MatchString(string(chunk))
			if err != nil {
				return nil, err
			}
			if ok {
				foundMatch = true
				break
			}

			// If this was the last chunk, exit
			if end == len(content) {
				break
			}
		}
	}

	if !foundMatch {
		return []fileSearchResult{}, nil
	}

	// Match found in pre-check, now search for all matches and their positions
	// If we can use standard regexp, do the full search with it to avoid string conversion overhead
	if stdRegex != nil {
		// Use standard regexp for better performance
		matches := stdRegex.FindAllIndex(content, -1)
		if len(matches) == 0 {
			return []fileSearchResult{}, nil
		}

		r = make([]fileSearchResult, 0, len(matches))

		// Calculate line numbers incrementally to avoid recounting (1-based for display)
		lineNumber := 1
		lineStart := 0
		lastPos := 0

		for _, match := range matches {
			matchStart := match[0]
			matchEnd := match[1]

			// Count newlines only from last position to current match
			for i := lastPos; i < matchStart; i++ {
				if content[i] == '\n' {
					lineNumber++
					lineStart = i + 1
				}
			}
			lastPos = matchStart

			// Character position as byte offset from start of line
			charNumber := matchStart - lineStart

			r = append(r, fileSearchResult{
				positionParams: protocol.TextDocumentPositionParams{
					TextDocument: protocol.TextDocumentIdentifier{
						URI: protocol.DocumentURI(uri.File(absPath)),
					},
					Position: protocol.Position{
						Line:      uint32(lineNumber),
						Character: uint32(charNumber),
					},
				},
				match: string(content[matchStart:matchEnd]),
			})
		}
	} else {
		// Need regexp2 for full match - convert to string
		contentStr := string(content)

		// Find all matches in the file content using regexp2
		match, err := regex.FindStringMatch(contentStr)
		if err != nil {
			return nil, err
		}

		// Calculate line numbers incrementally to avoid recounting (1-based for display)
		lineNumber := 1
		lineStart := 0
		lastPos := 0

		for match != nil {
			// Count newlines only from last position to current match
			for i := lastPos; i < match.Index; i++ {
				if contentStr[i] == '\n' {
					lineNumber++
					lineStart = i + 1
				}
			}
			lastPos = match.Index

			// Character position as byte offset from start of line
			charNumber := match.Index - lineStart

			r = append(r, fileSearchResult{
				positionParams: protocol.TextDocumentPositionParams{
					TextDocument: protocol.TextDocumentIdentifier{
						URI: protocol.DocumentURI(uri.File(absPath)),
					},
					Position: protocol.Position{
						Line:      uint32(lineNumber),
						Character: uint32(charNumber),
					},
				},
				match: match.String(),
			})

			match, err = regex.FindNextMatch(match)
			if err != nil {
				return nil, err
			}
		}
	}

	return r, nil
}

// performGrepSearch uses platform-native grep/perl for simple (non-multiline) patterns
// This provides optimal performance for the common case
func (b *builtinServiceClient) performGrepSearch(pattern, trimmedPattern string, locations []string) ([]fileSearchResult, error) {
	var err error

	if runtime.GOOS == "windows" {
		// Windows doesn't have grep, use the optimized Go-based search
		// This includes literal string optimization via bytes.Index()
		return b.performMultilineSearch(trimmedPattern, locations)
	}

	// Calculate total argument length to determine if we can use direct grep
	// ARG_MAX on Linux is typically 2MB, use conservative 512KB threshold for safety
	const argMaxSafeThreshold = 512 * 1024
	totalArgLength := len(pattern) + 50 // pattern + grep flags overhead
	for _, loc := range locations {
		totalArgLength += len(loc) + 1 // +1 for space separator
	}

	var outputBytes bytes.Buffer

	// Fast path for small projects: use direct grep if args fit within ARG_MAX
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" && totalArgLength < argMaxSafeThreshold {
		b.log.V(5).Info("using direct grep (fast path)", "pattern", pattern, "totalFiles", len(locations), "argLength", totalArgLength)
		// Build grep command with all files as arguments
		// Use -- to mark end of options, preventing patterns like --pf- from being interpreted as options
		args := []string{"-o", "-n", "--with-filename", "-P", "--", pattern}
		args = append(args, locations...)
		cmd := exec.Command("grep", args...)
		output, err := cmd.Output()
		if err != nil {
			if exitError, ok := err.(*exec.ExitError); ok {
				if exitError.ExitCode() == 1 {
					// No matches found, not an error
					err = nil
				}
			}
			if err != nil {
				return nil, fmt.Errorf("could not run grep with provided pattern %+v", err)
			}
		}
		outputBytes.Write(output)
	} else {
		// Slow path for large projects or macOS: use batching with xargs
		batchSize := 500
		for start := 0; start < len(locations); start += batchSize {
			end := int(math.Min(float64(start+batchSize), float64(len(locations))))
			currBatch := locations[start:end]
			b.log.V(5).Info("searching for pattern", "pattern", pattern, "batchSize", len(currBatch), "totalFiles", len(locations))
			var currOutput []byte
			switch runtime.GOOS {
			case "darwin":
				isEscaped := isSlashEscaped(pattern)
				escapedPattern := pattern
				// some rules already escape '/' while others do not
				if !isEscaped {
					escapedPattern = strings.ReplaceAll(escapedPattern, "/", "\\/")
				}
				// escape other chars used in perl pattern
				escapedPattern = strings.ReplaceAll(escapedPattern, "'", "'\\''")
				//escapedPattern = strings.ReplaceAll(escapedPattern, "$", "\\$")
				var fileList bytes.Buffer
				for _, f := range currBatch {
					fileList.WriteString(f)
					fileList.WriteByte('\x00')
				}
				cmdStr := fmt.Sprintf(
					`xargs -0 perl -ne 'if (/%v/) { print "$ARGV:$.:$&\n" } close ARGV if eof;'`,
					escapedPattern,
				)
				b.log.V(7).Info("running perl", "cmd", cmdStr)
				cmd := exec.Command("/bin/sh", "-c", cmdStr)
				cmd.Stdin = &fileList
				currOutput, err = cmd.Output()
			default:
				// Use xargs to avoid ARG_MAX limits when processing large numbers of files
				// This prevents "argument list too long" errors when analyzing projects
				// with many files (e.g., node_modules with 30,000+ files)
				var fileList bytes.Buffer
				for _, f := range currBatch {
					fileList.WriteString(f)
					fileList.WriteByte('\x00')
				}
				// Escape pattern for safe shell interpolation
				escapedPattern := strings.ReplaceAll(pattern, "'", "'\"'\"'")
				// Use -- to mark end of options, preventing patterns like --pf- from being interpreted as options
				cmdStr := fmt.Sprintf(
					`xargs -0 grep -o -n --with-filename -P -- '%s'`,
					escapedPattern,
				)
				b.log.V(7).Info("running grep via xargs", "cmd", cmdStr)
				cmd := exec.Command("/bin/sh", "-c", cmdStr)
				cmd.Stdin = &fileList
				currOutput, err = cmd.Output()
			}
			if err != nil {
				if exitError, ok := err.(*exec.ExitError); ok {
					// Exit code 1: grep found no matches
					if exitError.ExitCode() == 1 {
						err = nil // Clear error; no matches in this batch
					}
					// Exit code 123: GNU xargs exits with 123 when any child exits with 1-125
					// This includes both "no matches" (grep exit 1) and real errors (grep exit 2)
					// Only clear 123 if stderr is empty; otherwise surface the error
					if exitError.ExitCode() == 123 {
						stderrStr := strings.TrimSpace(string(exitError.Stderr))
						if stderrStr == "" {
							err = nil // mixed batches, but no actual error output
						} else {
							// Real grep error (e.g., exit 2) - include stderr in error message
							return nil, fmt.Errorf("could not run grep with provided pattern %+v; stderr=%s", err, stderrStr)
						}
					}
				}
				if err != nil {
					return nil, fmt.Errorf("could not run grep with provided pattern %+v", err)
				}
			}
			outputBytes.Write(currOutput)
		}
	}

	matches := []string{}
	outputString := strings.TrimSpace(outputBytes.String())
	if outputString != "" {
		matches = append(matches, strings.Split(outputString, "\n")...)
	}

	fileSearchResults := []fileSearchResult{}
	for _, match := range matches {
		var pieces []string
		pieces, err := parseGrepOutputForFileContent(match)
		if err != nil {
			return nil, fmt.Errorf("could not parse grep output '%s' for the Pattern '%v': %v ", match, pattern, err)
		}

		absPath, err := filepath.Abs(pieces[0])
		if err != nil {
			absPath = pieces[0]
		}

		lineNumber, err := strconv.Atoi(pieces[1])
		if err != nil {
			return nil, fmt.Errorf("cannot convert line number string to integer")
		}

		fileSearchResults = append(fileSearchResults, fileSearchResult{
			positionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{
					URI: protocol.DocumentURI(uri.File(absPath)),
				},
				Position: protocol.Position{
					Line: uint32(lineNumber),
				},
			},
			match: pieces[2],
		})
	}

	return fileSearchResults, nil
}

// parallelWalkWindows is used on Windows for non-multiline patterns
func (b *builtinServiceClient) parallelWalkWindows(paths []string, regex *regexp2.Regexp) ([]fileSearchResult, error) {
	var positions []fileSearchResult
	var positionsMu sync.Mutex
	var eg errgroup.Group

	// Set a parallelism limit to avoid hitting limits related to opening too many files.
	// On Windows, this can show up as a runtime failure due to a thread limit.
	eg.SetLimit(20)

	for _, filePath := range paths {
		eg.Go(func() error {
			pos, err := b.processFileWindows(filePath, regex)
			if err != nil {
				return err
			}

			positionsMu.Lock()
			defer positionsMu.Unlock()
			positions = append(positions, pos...)
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return positions, nil
}

// processFileWindows processes a file on Windows using line-by-line scanning
func (b *builtinServiceClient) processFileWindows(path string, regex *regexp2.Regexp) ([]fileSearchResult, error) {
	// Check file size before loading to prevent OOM on large files (e.g., JARs)
	const maxFileSize = 100 * 1024 * 1024 // 100MB limit
	fileInfo, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if fileInfo.Size() > maxFileSize {
		b.log.V(5).Info("skipping large file", "file", path, "size", fileInfo.Size(), "limit", maxFileSize)
		return []fileSearchResult{}, nil
	}

	var content []byte
	if b.encoding != "" {
		content, err = engine.OpenFileWithEncoding(path, b.encoding)
		if err != nil {
			b.log.V(5).Error(err, "failed to convert file encoding, using original content", "file", path)
			content, err = os.ReadFile(path)
			if err != nil {
				return nil, err
			}
		}
	} else {
		content, err = os.ReadFile(path)
		if err != nil {
			return nil, err
		}
	}

	reader := bytes.NewReader(content)
	nBytes := int64(0)
	nCh := int64(0)
	buffer := make([]byte, 1024*1024) // Create a buffer to hold 1MB
	foundMatch := false
	for {
		n, readErr := io.ReadFull(reader, buffer)
		if readErr != io.ErrUnexpectedEOF && readErr != io.EOF {
			return nil, readErr
		} else if readErr != nil && foundMatch {
			// This case probably shouldn't happen.
			break
		}
		nBytes += int64(n)
		nCh++
		b.log.V(7).Info("read bytes for processing file", "file", path, "bytes_read", nBytes, "chunk_read", nCh)
		ok, err := regex.MatchString(string(buffer))
		b.log.V(7).Info("finding match regex", "file", path, "ok", ok, "err", err, "regex", regex)
		if err != nil {
			return nil, err
		}
		// If we find a single match we have to go through the file anyway to find the line numbers.
		if ok {
			foundMatch = true
			break
		}
		if readErr == io.EOF {
			break
		}
		buffer = make([]byte, 1024*1024) // Create a buffer to hold 1MB
	}

	if !foundMatch {
		return []fileSearchResult{}, nil
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	reader = bytes.NewReader(content)
	r := []fileSearchResult{}
	scanner := bufio.NewScanner(reader)
	var lineNumber uint32 = 1
	for scanner.Scan() {
		line := scanner.Text()
		match, err := regex.FindStringMatch(line)
		if err != nil {
			return nil, err
		}
		if match != nil {
			r = append(r, fileSearchResult{
				positionParams: protocol.TextDocumentPositionParams{
					TextDocument: protocol.TextDocumentIdentifier{
						URI: protocol.DocumentURI(uri.File(absPath)),
					},
					Position: protocol.Position{
						Line:      lineNumber,
						Character: uint32(match.Index),
					},
				},
				match: match.String(),
			})
		}
		lineNumber += 1
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return r, nil
}

func isSlashEscaped(str string) bool {
	for i := 0; i < len(str); i++ {
		if str[i] == '/' && i > 0 && str[i-1] == '\\' {
			return true
		}
	}
	return false
}

func parseGrepOutputForFileContent(match string) ([]string, error) {
	// This will parse the output of the PowerShell/grep in the form
	// "Filepath:Linenumber:Matchingtext" to return string array of path, line number and matching text
	// works with handling both windows and unix based file paths eg: "C:\path\to\file" and "/path/to/file"
	re, err := regexp.Compile(`^(.*?):(\d+):(.*)$`)
	if err != nil {
		return nil, fmt.Errorf("failed to compile regular expression: %v", err)
	}
	submatches := re.FindStringSubmatch(match)
	if len(submatches) != 4 {
		return nil, fmt.Errorf(
			"malformed response from file search, cannot parse result '%s' with pattern %#q", match, re)
	}
	return submatches[1:], nil
}
