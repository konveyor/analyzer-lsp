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

	workingCopyMgr *workingCopyManager
}

type fileTemplateContext struct {
	Filepaths []string `json:"filepaths,omitempty"`
}

var _ provider.ServiceClient = &builtinServiceClient{}

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

		filePaths, err := fileSearcher.Search(provider.SearchCriteria{
			Patterns: []string{c.FilePattern},
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
			nodes, err := queryXMLFile(file, query)
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
							"matchingXML": node.OutputXML(false),
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
			ConditionFilepaths: cond.XML.Filepaths,
		})
		if err != nil {
			return response, fmt.Errorf("unable to find XML files: %v", err)
		}
		for _, file := range xmlFiles {
			nodes, err := queryXMLFile(file, query)
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
									"matchingXML": node.OutputXML(false),
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
			ConditionFilepaths: cond.XML.Filepaths,
		})
		if err != nil {
			return response, fmt.Errorf("unable to find XML files: %v", err)
		}
		for _, file := range jsonFiles {
			f, err := os.Open(file)
			if err != nil {
				log.V(5).Error(err, "error opening json file", "file", file)
				continue
			}
			doc, err := jsonquery.Parse(f)
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

func queryXMLFile(filePath string, query *xpath.Expr) (nodes []*xmlquery.Node, err error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("unable to open file '%s': %w", filePath, err)
	}
	defer f.Close()
	// TODO This should start working if/when this merges and releases: https://github.com/golang/go/pull/56848
	var doc *xmlquery.Node
	doc, err = xmlquery.ParseWithOptions(f, xmlquery.ParserOptions{Decoder: &xmlquery.DecoderOptions{Strict: false}})
	if err != nil {
		if err.Error() == "xml: unsupported version \"1.1\"; only version 1.0 is supported" {
			// TODO HACK just pretend 1.1 xml documents are 1.0 for now while we wait for golang to support 1.1
			var b []byte
			b, err = os.ReadFile(filePath)
			if err != nil {
				return nil, fmt.Errorf("unable to parse xml file '%s': %w", filePath, err)
			}
			docString := strings.Replace(string(b), "<?xml version=\"1.1\"", "<?xml version = \"1.0\"", 1)
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
	var err error

	if runtime.GOOS == "windows" {
		// Have to trim quotes around the pattern to keep backwards compatibility
		trimmedPattern := strings.Trim(pattern, "\"")
		patternRegex, err := regexp2.Compile(trimmedPattern, regexp2.Multiline)
		if err != nil {
			return nil, fmt.Errorf("could not compile provided regex pattern '%s': %v", pattern, err)
		}
		matches, err := b.parallelWalk(locations, patternRegex)
		if err != nil {
			return nil, fmt.Errorf("failed to perform search - %w", err)
		}
		return matches, nil
	}

	var outputBytes bytes.Buffer
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
			escapedPattern = strings.ReplaceAll(escapedPattern, "$", "\\$")
			var fileList bytes.Buffer
			for _, f := range locations {
				fileList.WriteString(f)
				fileList.WriteByte('\x00')
			}
			cmdStr := fmt.Sprintf(
				`xargs -0 perl -ne '/%v/ && print "$ARGV:$.:$1\n";'`,
				escapedPattern,
			)
			b.log.V(7).Info("running perl", "cmd", cmdStr)
			cmd := exec.Command("/bin/sh", "-c", cmdStr)
			cmd.Stdin = &fileList
			currOutput, err = cmd.Output()
		default:
			args := []string{"-o", "-n", "--with-filename", "-R", "-P", pattern}
			b.log.V(7).Info("running grep with args", "args", args)
			args = append(args, locations...)
			cmd := exec.Command("grep", args...)
			currOutput, err = cmd.Output()
		}
		if err != nil {
			if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 1 {
				return nil, nil
			}
			return nil, fmt.Errorf("could not run grep with provided pattern %+v", err)
		}
		outputBytes.Write(currOutput)
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

func (b *builtinServiceClient) parallelWalk(paths []string, regex *regexp2.Regexp) ([]fileSearchResult, error) {
	var positions []fileSearchResult
	var positionsMu sync.Mutex
	var eg errgroup.Group

	// Set a parallelism limit to avoid hitting limits related to opening too many files.
	// On Windows, this can show up as a runtime failure due to a thread limit.
	eg.SetLimit(20)

	for _, filePath := range paths {
		eg.Go(func() error {
			pos, err := b.processFile(filePath, regex)
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

func (b *builtinServiceClient) processFile(path string, regex *regexp2.Regexp) ([]fileSearchResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	nBytes := int64(0)
	nCh := int64(0)
	buffer := make([]byte, 1024*1024) // Create a buffer to hold 1MB
	foundMatch := false
	for {
		n, readErr := io.ReadFull(f, buffer)
		if readErr != io.ErrUnexpectedEOF && readErr != io.EOF {
			return nil, err
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
		if readErr != nil {
			// We didn't find a match, we read the full file, return no matches
			return []fileSearchResult{}, nil
		}
		buffer = make([]byte, 1024*1024) // Create a buffer to hold 1MB
	}
	// This shouldn't happen, but lets be safe and not read files more then we have to.
	if !foundMatch {
		return []fileSearchResult{}, nil
	}

	// Now we we need to go line by line to find the line numbers.
	f.Seek(0, io.SeekStart)
	var r []fileSearchResult

	scanner := bufio.NewScanner(f)
	lineNumber := 1
	for scanner.Scan() {
		line := scanner.Text()
		match, err := regex.FindStringMatch(line)
		if err != nil {
			return nil, err
		}
		for match != nil {
			absPath, err := filepath.Abs(path)
			if err != nil {
				return nil, err
			}

			r = append(r, fileSearchResult{
				positionParams: protocol.TextDocumentPositionParams{
					TextDocument: protocol.TextDocumentIdentifier{
						URI: protocol.DocumentURI(uri.File(absPath)),
					},
					Position: protocol.Position{
						Line:      uint32(lineNumber),
						Character: uint32(match.Index),
					},
				},
				match: match.String(),
			})
			match, err = regex.FindNextMatch(match)
			if err != nil {
				return nil, err
			}
		}
		lineNumber++
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
