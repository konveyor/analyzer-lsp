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
}

type fileTemplateContext struct {
	Filepaths []string `json:"filepaths,omitempty"`
}

var _ provider.ServiceClient = &builtinServiceClient{}

func (b *builtinServiceClient) Prepare(ctx context.Context, conditionsByCap []provider.ConditionsByCap) error {
	return nil
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

	// in addition to base location, we have to look at working copies too
	wcIncludedPaths, wcExcludedPaths := p.getWorkingCopies()
	// get paths from providerContext
	includedPaths, excludedPaths := cond.ProviderContext.GetScopedFilepaths()
	excludedPaths = append(excludedPaths, wcExcludedPaths...)

	fileSearcher := provider.FileSearcher{
		BasePath:        p.config.Location,
		AdditionalPaths: wcIncludedPaths,
		// get global include / exclude paths from provider config
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

	switch cap {
	case "file":
		c := cond.File
		if c.Pattern == "" {
			return response, fmt.Errorf("could not parse provided file pattern as string: %v", conditionInfo)
		}
		matchingFiles, err := fileSearcher.Search(provider.SearchCriteria{
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

		patterns := []string{}
		if c.FilePattern != "" {
			patterns = append(patterns, c.FilePattern)
		}
		filePaths, err := fileSearcher.Search(provider.SearchCriteria{
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
		xmlFiles, err := fileSearcher.Search(provider.SearchCriteria{
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
		xmlFiles, err := fileSearcher.Search(provider.SearchCriteria{
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
		jsonFiles, err := fileSearcher.Search(provider.SearchCriteria{
			Patterns:           []string{"*.json"},
			ConditionFilepaths: cond.JSON.Filepaths,
		})
		if err != nil {
			return response, fmt.Errorf("unable to find XML files: %v", err)
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
		excludedPaths = append(excludedPaths, wc.filePath)
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
