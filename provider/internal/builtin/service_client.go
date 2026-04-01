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
	// allParsedConditions holds pre-compiled conditions from Prepare(),
	// deduplicated by cache key. Chained conditions are excluded.
	allParsedConditions []parsedCondition

	// cacheRefreshChan receives requests from the working copy manager
	// after a file change has been written to disk. Workers on the
	// builtinServiceClient side consume from this channel and re-run
	// conditions against the updated content.
	cacheRefreshChan chan cacheRefreshRequest

	// cacheRefreshCtx is cancelled in Stop() to signal cache refresh
	// workers to exit and cancel in-progress refreshes.
	cacheRefreshCtx    context.Context
	cacheRefreshCancel context.CancelFunc

	// activeRefreshes tracks cancel funcs for in-progress per-file
	// cache refreshes, allowing cancellation when a newer change arrives
	// for the same file.
	activeRefreshes sync.Map // map[string]context.CancelFunc

	// cacheRefreshMu guards startCacheRefreshWorkers and
	// stopCacheRefreshWorkers against concurrent access.
	cacheRefreshMu sync.Mutex

	// pendingCacheRefresh tracks files whose cache entries have been
	// invalidated but not yet refreshed. Values are chan struct{},
	// closed when the refresh completes.
	pendingCacheRefresh sync.Map // map[string]chan struct{}
}

type fileTemplateContext struct {
	Filepaths []string `json:"filepaths,omitempty"`
}

var _ provider.ServiceClient = &builtinServiceClient{}

// readFileContent reads a file's content, applying encoding conversion if configured.
func (b *builtinServiceClient) readFileContent(filePath string) ([]byte, error) {
	var content []byte
	var err error
	if b.encoding != "" {
		content, err = engine.OpenFileWithEncoding(filePath, b.encoding)
		if err != nil {
			b.log.V(5).Error(err, "failed to convert file encoding, using original content", "file", filePath)
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
	if bytes.Contains(content, []byte("\r\n")) {
		content = bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
	}
	return content, nil
}

// parseXMLContent parses XML content into a DOM. Handles the XML 1.1 compatibility hack.
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

// queryXMLFile reads, parses, and queries an XML file in one step.
func (b *builtinServiceClient) queryXMLFile(filePath string, query *xpath.Expr) ([]*xmlquery.Node, error) {
	content, err := b.readFileContent(filePath)
	if err != nil {
		return nil, err
	}
	doc, err := b.parseXMLContent(filePath, content)
	if err != nil {
		return nil, err
	}
	return queryXMLDoc(doc, query)
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

// isLiteralPattern returns true if the pattern contains no regex metacharacters.
func isLiteralPattern(pattern string) bool {
	for _, ch := range pattern {
		if strings.ContainsRune(".*+?^$[]{}()|\\", ch) {
			return false
		}
	}
	return true
}

// matchSpan represents a byte-range match within file content.
type matchSpan struct {
	start int
	end   int
	text  string
}

// buildFileSearchResults converts matchSpans into fileSearchResults by calculating
// line numbers incrementally through the content.
func buildFileSearchResults(filePath string, content []byte, spans []matchSpan) []fileSearchResult {
	if len(spans) == 0 {
		return nil
	}
	results := make([]fileSearchResult, 0, len(spans))
	lineNumber := 1
	lineStart := 0
	lastPos := 0
	for _, span := range spans {
		for i := lastPos; i < span.start; i++ {
			if content[i] == '\n' {
				lineNumber++
				lineStart = i + 1
			}
		}
		lastPos = span.start
		charNumber := span.start - lineStart
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
			match: span.text,
		})
	}
	return results
}

// findLiteralMatches finds all occurrences of a literal string in content.
func findLiteralMatches(content []byte, pattern string) []matchSpan {
	literalBytes := []byte(pattern)
	var spans []matchSpan
	searchPos := 0
	for {
		idx := bytes.Index(content[searchPos:], literalBytes)
		if idx == -1 {
			break
		}
		actualPos := searchPos + idx
		spans = append(spans, matchSpan{
			start: actualPos,
			end:   actualPos + len(literalBytes),
			text:  pattern,
		})
		searchPos = actualPos + 1
	}
	return spans
}

// findStdRegexMatches finds all matches of a compiled standard regexp in content.
func findStdRegexMatches(content []byte, regex *regexp.Regexp) []matchSpan {
	matches := regex.FindAllIndex(content, -1)
	if len(matches) == 0 {
		return nil
	}
	spans := make([]matchSpan, 0, len(matches))
	for _, m := range matches {
		spans = append(spans, matchSpan{
			start: m[0],
			end:   m[1],
			text:  string(content[m[0]:m[1]]),
		})
	}
	return spans
}

// findRegexp2Matches finds all matches of a regexp2 pattern in content.
func findRegexp2Matches(content string, regex *regexp2.Regexp) ([]matchSpan, error) {
	match, err := regex.FindStringMatch(content)
	if err != nil {
		return nil, err
	}
	var spans []matchSpan
	for match != nil {
		spans = append(spans, matchSpan{
			start: match.Index,
			end:   match.Index + match.Length,
			text:  match.String(),
		})
		match, err = regex.FindNextMatch(match)
		if err != nil {
			return nil, err
		}
	}
	return spans, nil
}

// filecontentMatchesToIncidents converts fileSearchResults into IncidentContexts.
func filecontentMatchesToIncidents(matches []fileSearchResult) []provider.IncidentContext {
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
	return incidents
}

// xmlNodeToIncident converts an XML node match into an IncidentContext.
func (b *builtinServiceClient) xmlNodeToIncident(ctx context.Context, filePath string, node *xmlquery.Node) provider.IncidentContext {
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
	return incident
}

// xmlPublicIDNodeToIncident converts an xmlPublicID match into an IncidentContext.
func xmlPublicIDNodeToIncident(filePath string, node *xmlquery.Node) provider.IncidentContext {
	return provider.IncidentContext{
		FileURI: uri.File(filePath),
		Variables: map[string]interface{}{
			"matchingXML": compactXML(node.OutputXML(false)),
			"innerText":   node.InnerText(),
			"data":        node.Data,
		},
	}
}

// jsonNodeToIncident converts a JSON node match into an IncidentContext.
func (b *builtinServiceClient) jsonNodeToIncident(ctx context.Context, filePath string, node *jsonquery.Node) provider.IncidentContext {
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
	return incident
}

// searchContentForPattern searches pre-read content for a pattern.
// This is the shared core for both cached (Prepare) and uncached (Evaluate) paths.
func (b *builtinServiceClient) searchContentForPattern(filePath string, content []byte, trimmedPattern string) []fileSearchResult {
	if isLiteralPattern(trimmedPattern) {
		spans := findLiteralMatches(content, trimmedPattern)
		return buildFileSearchResults(filePath, content, spans)
	}

	// Regex path — try standard regexp first
	fullPattern := `(?m)` + trimmedPattern
	stdRegex, err := regexp.Compile(fullPattern)
	if err != nil {
		// Fall back to regexp2
		patternRegex, err := regexp2.Compile(fullPattern, regexp2.Multiline)
		if err != nil {
			b.log.V(5).Error(err, "failed to compile pattern", "pattern", trimmedPattern)
			return nil
		}
		spans, err := findRegexp2Matches(string(content), patternRegex)
		if err != nil {
			return nil
		}
		// regexp2 spans are byte offsets into the string; content bytes match
		return buildFileSearchResults(filePath, content, spans)
	}

	if !stdRegex.Match(content) {
		return nil
	}

	spans := findStdRegexMatches(content, stdRegex)
	return buildFileSearchResults(filePath, content, spans)
}

// Prepare — single filesystem walk, incident cache population
func (b *builtinServiceClient) Prepare(ctx context.Context, conditionsByCap []provider.ConditionsByCap) error {
	ctx, span := tracing.StartNewSpan(ctx, "builtin.Prepare")
	defer span.End()

	// Reset state so a failed re-Prepare does not leave stale caches active.
	b.prepared = false
	b.fileIndex = nil
	b.allParsedConditions = nil
	b.incidentCacheMutex.Lock()
	b.incidentCache = nil
	b.incidentCacheMutex.Unlock()
	b.pendingCacheRefresh.Range(func(key, val any) bool {
		if ch, ok := val.(chan struct{}); ok {
			close(ch)
		}
		b.pendingCacheRefresh.Delete(key)
		return true
	})
	// Stop old cache refresh workers so re-Prepare can start fresh ones.
	b.stopCacheRefreshWorkers()

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
	// NOTE: do not defer Unlock here – the worker pool below calls
	// mergeIntoCache which also acquires incidentCacheMutex and would deadlock.
	b.incidentCacheMutex.Lock()
	b.incidentCache = make(map[incidentCacheKey]map[string][]provider.IncidentContext)
	b.incidentCacheMutex.Unlock()

	// Process all files through worker pool to populate the incident cache.
	if len(parsedConds) > 0 && len(files) > 0 {
		fileChan := make(chan string, 10)
		wg := sync.WaitGroup{}

		for range numWorkers {
			wg.Go(func() {
				for file := range fileChan {
					b.processFileForAllCaps(ctx, file)
				}
			})
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

	b.startCacheRefreshWorkers()

	b.prepared = true
	b.log.V(5).Info("Prepare complete", "fileCount", len(files), "parsedConditions", len(parsedConds))
	return nil
}

// startCacheRefreshWorkers initialises the cache refresh channel and spawns
// worker goroutines the first time it is called. Subsequent calls are no-ops.
// Safe to call concurrently from Prepare and NotifyFileChanges.
func (b *builtinServiceClient) startCacheRefreshWorkers() {
	b.cacheRefreshMu.Lock()
	defer b.cacheRefreshMu.Unlock()
	if b.cacheRefreshChan != nil {
		return
	}
	b.cacheRefreshChan = make(chan cacheRefreshRequest, 1024)
	b.cacheRefreshCtx, b.cacheRefreshCancel = context.WithCancel(context.Background())
	b.workingCopyMgr.cacheRefreshChan = b.cacheRefreshChan
	for range numWorkers {
		go b.cacheRefreshWorker()
	}
}

// stopCacheRefreshWorkers cancels running workers and resets state so
// startCacheRefreshWorkers can re-initialise on the next call.
func (b *builtinServiceClient) stopCacheRefreshWorkers() {
	b.cacheRefreshMu.Lock()
	defer b.cacheRefreshMu.Unlock()
	if b.cacheRefreshCancel != nil {
		b.cacheRefreshCancel()
		b.cacheRefreshCancel = nil
		b.cacheRefreshChan = nil
		b.workingCopyMgr.cacheRefreshChan = nil
	}
}

func (b *builtinServiceClient) Stop() {
	b.workingCopyMgr.stop()
	b.cacheRefreshMu.Lock()
	defer b.cacheRefreshMu.Unlock()
	if b.cacheRefreshCancel != nil {
		b.cacheRefreshCancel()
	}
	b.cacheRefreshCancel = nil
	b.cacheRefreshChan = nil
	b.workingCopyMgr.cacheRefreshChan = nil
}

func (p *builtinServiceClient) NotifyFileChanges(ctx context.Context, changes ...provider.FileChange) error {
	filtered := []provider.FileChange{}
	for _, change := range changes {
		if strings.HasPrefix(change.Path, p.config.Location) {
			filtered = append(filtered, change)
		}
	}
	// Mark pending before invalidating to avoid a window where
	// Evaluate sees an empty cache with no pending flag.
	p.startCacheRefreshWorkers()
	if p.prepared {
		for _, change := range filtered {
			// Cancel any in-progress refresh for this file
			cancelFn, hasActive := p.activeRefreshes.LoadAndDelete(change.Path)
			if hasActive {
				cancelFn.(context.CancelFunc)()
			}

			ch := make(chan struct{})
			oldCh, hadPending := p.pendingCacheRefresh.Swap(change.Path, ch)
			if hadPending {
				close(oldCh.(chan struct{}))
			}

			p.invalidateCacheForFile(change.Path)

			// For saved changes, send cache refresh directly (re-read from disk).
			// The WC manager only handles temp file cleanup for saves.
			if change.Saved {
				select {
				case p.cacheRefreshChan <- cacheRefreshRequest{
					originalPath: change.Path,
					contentPath:  change.Path,
				}:
				case <-ctx.Done():
				}
			}
		}
	}
	// Pass to working copy manager — it writes the temp file (for unsaved
	// changes) and sends a cache refresh request for it.
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

	// searchFilesFromFS creates a FileSearcher and walks the filesystem (original behavior).
	searchFilesFromFS := func(criteria provider.SearchCriteria) ([]string, error) {
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

	// searchFiles resolves file lists from the cached index when available,
	// otherwise falls back to a full filesystem walk.
	searchFiles := func(criteria provider.SearchCriteria) ([]string, error) {
		if p.prepared {
			return p.filterCachedFiles(cond, criteria)
		}
		return searchFilesFromFS(criteria)
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
		patterns := []string{}
		if c.FilePattern != "" {
			patterns = append(patterns, c.FilePattern)
		}
		if incidents, ok, err := p.tryIncidentCache(ctx, cap, cond, patterns); err != nil {
			return response, fmt.Errorf("failed to filter cached files - %w", err)
		} else if ok {
			response.Incidents = p.workingCopyMgr.reformatIncidents(incidents...)
			response.Matched = len(response.Incidents) > 0
			return response, nil
		}

		// Fallback: full filesystem walk (chained condition, cache miss, or not prepared).
		// Uses searchFilesFromFS to bypass the index since chained conditions have
		// template-expanded Filepaths that aren't in the cached index.
		filePaths, err := searchFilesFromFS(provider.SearchCriteria{
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

		response.Incidents = filecontentMatchesToIncidents(matches)
		response.Incidents = p.workingCopyMgr.reformatIncidents(response.Incidents...)
		response.Matched = len(response.Incidents) > 0
		return response, nil
	case "xml":
		query, err := xpath.CompileWithNS(cond.XML.XPath, cond.XML.Namespaces)
		if query == nil || err != nil {
			return response, fmt.Errorf("could not parse provided xpath query '%s': %v", cond.XML.XPath, err)
		}

		// Try incident cache lookup
		if incidents, ok, err := p.tryIncidentCache(ctx, cap, cond, []string{"*.xml", "*.xhtml"}); err != nil {
			return response, fmt.Errorf("unable to find XML files: %v", err)
		} else if ok {
			response.Incidents = p.workingCopyMgr.reformatIncidents(incidents...)
			response.Matched = len(response.Incidents) > 0
			return response, nil
		}

		// Fallback: full filesystem walk
		xmlFiles, err := searchFilesFromFS(provider.SearchCriteria{
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
					response.Incidents = append(response.Incidents, p.xmlNodeToIncident(ctx, absPath, node))
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
		if incidents, ok, err := p.tryIncidentCache(ctx, cap, cond, []string{"*.xml", "*.xhtml"}); err != nil {
			return response, fmt.Errorf("unable to find XML files: %v", err)
		} else if ok {
			response.Incidents = p.workingCopyMgr.reformatIncidents(incidents...)
			response.Matched = len(response.Incidents) > 0
			return response, nil
		}

		// Fallback: full filesystem walk
		xmlFiles, err := searchFilesFromFS(provider.SearchCriteria{
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
							response.Incidents = append(response.Incidents, xmlPublicIDNodeToIncident(absPath, node))
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
		if incidents, ok, err := p.tryIncidentCache(ctx, cap, cond, []string{"*.json"}); err != nil {
			return response, fmt.Errorf("unable to find JSON files: %v", err)
		} else if ok {
			response.Incidents = p.workingCopyMgr.reformatIncidents(incidents...)
			response.Matched = len(response.Incidents) > 0
			return response, nil
		}

		// Fallback: full filesystem walk
		jsonFiles, err := searchFilesFromFS(provider.SearchCriteria{
			Patterns:           []string{"*.json"},
			ConditionFilepaths: cond.JSON.Filepaths,
		})
		if err != nil {
			return response, fmt.Errorf("unable to find JSON files: %v", err)
		}
		for _, file := range jsonFiles {
			content, err := p.readFileContent(file)
			if err != nil {
				log.V(5).Error(err, "error reading json file", "file", file)
				continue
			}
			doc, err := jsonquery.Parse(bytes.NewReader(content))
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
					response.Incidents = append(response.Incidents, p.jsonNodeToIncident(ctx, absPath, node))
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
	isLiteral := isLiteralPattern(trimmedPattern)

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
	fileInfo, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if fileInfo.Size() > maxCacheFileSize {
		b.log.V(5).Info("skipping large file", "file", path, "size", fileInfo.Size(), "limit", maxCacheFileSize)
		return []fileSearchResult{}, nil
	}

	content, err := b.readFileContent(path)
	if err != nil {
		return nil, err
	}

	// Get absolute path for results
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	// Fast path for literal string search using bytes.Index()
	if isLiteral {
		spans := findLiteralMatches(content, literalPattern)
		if len(spans) == 0 {
			return []fileSearchResult{}, nil
		}
		return buildFileSearchResults(absPath, content, spans), nil
	}

	// Fast pre-check: try to use Go's standard regexp for quick byte-based filtering
	// This avoids string allocation for files without matches
	foundMatch := false

	if stdRegex != nil {
		b.log.V(7).Info("using golang regex", "pattern", literalPattern)
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
	if stdRegex != nil {
		spans := findStdRegexMatches(content, stdRegex)
		if len(spans) == 0 {
			return []fileSearchResult{}, nil
		}
		return buildFileSearchResults(absPath, content, spans), nil
	}

	// Need regexp2 for full match - convert to string
	contentStr := string(content)
	spans, err := findRegexp2Matches(contentStr, regex)
	if err != nil {
		return nil, err
	}
	return buildFileSearchResults(absPath, content, spans), nil
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
	fileInfo, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if fileInfo.Size() > maxCacheFileSize {
		b.log.V(5).Info("skipping large file", "file", path, "size", fileInfo.Size(), "limit", maxCacheFileSize)
		return []fileSearchResult{}, nil
	}

	content, err := b.readFileContent(path)
	if err != nil {
		return nil, err
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
