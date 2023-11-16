package builtin

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/antchfx/jsonquery"
	"github.com/antchfx/xmlquery"
	"github.com/antchfx/xpath"
	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/tracing"
	"go.lsp.dev/uri"
	"gopkg.in/yaml.v2"
)

type builtinServiceClient struct {
	config provider.InitConfig
	tags   map[string]bool
	provider.UnimplementedDependenciesComponent
	log logr.Logger

	cacheMutex    sync.RWMutex
	locationCache map[string]float64
}

var _ provider.ServiceClient = &builtinServiceClient{}

func (p *builtinServiceClient) Stop() {}

func (p *builtinServiceClient) Evaluate(ctx context.Context, cap string, conditionInfo []byte) (provider.ProviderEvaluateResponse, error) {
	var cond builtinCondition
	err := yaml.Unmarshal(conditionInfo, &cond)
	if err != nil {
		return provider.ProviderEvaluateResponse{}, fmt.Errorf("unable to get query info: %v", err)
	}
	response := provider.ProviderEvaluateResponse{Matched: false}
	switch cap {
	case "file":
		c := cond.File
		if c.Pattern == "" {
			return response, fmt.Errorf("could not parse provided file pattern as string: %v", conditionInfo)
		}
		matchingFiles, err := findFilesMatchingPattern(p.config.Location, c.Pattern)
		if err != nil {
			return response, fmt.Errorf("unable to find files using pattern `%s`: %v", c.Pattern, err)
		}

		if len(matchingFiles) != 0 {
			response.Matched = true
		}

		response.TemplateContext = map[string]interface{}{"filepaths": matchingFiles}
		for _, match := range matchingFiles {
			if filepath.IsAbs(match) {
				response.Incidents = append(response.Incidents, provider.IncidentContext{
					FileURI: uri.File(match),
				})
				continue

			}
			ab, err := filepath.Abs(match)
			if err != nil {
				//TODO: Probably want to log or something to let us know we can't get absolute path here.
				fmt.Printf("\n%v\n\n", err)
				ab = match
			}
			response.Incidents = append(response.Incidents, provider.IncidentContext{
				FileURI: uri.File(ab),
			})

		}
		return response, nil
	case "filecontent":
		c := cond.Filecontent
		if c.Pattern == "" {
			return response, fmt.Errorf("could not parse provided regex pattern as string: %v", conditionInfo)
		}
		patternRegex, err := regexp.Compile(c.Pattern)
		if err != nil {
			return response, err
		}
		matches, err := parallelWalk(p.config.Location, patternRegex)
		if err != nil {
			return response, err
		}

		for _, match := range matches {
			//TODO(fabianvf): This will not work if there is a `:` in the filename, do we care?
			containsFile, err := provider.FilterFilePattern(c.FilePattern, match.positionParams.TextDocument.URI)
			if err != nil {
				return response, err
			}
			if !containsFile {
				continue
			}
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
		return response, nil
	case "xml":
		query, err := xpath.CompileWithNS(cond.XML.XPath, cond.XML.Namespaces)
		if query == nil || err != nil {
			return response, fmt.Errorf("could not parse provided xpath query '%s': %v", cond.XML.XPath, err)
		}
		//TODO(fabianvf): how should we scope the files searched here?
		var xmlFiles []string
		patterns := []string{"*.xml", "*.xhtml"}
		xmlFiles, err = provider.GetFiles(p.config.Location, cond.XML.Filepaths, patterns...)
		if err != nil {
			return response, fmt.Errorf("unable to find files using pattern `%s`: %v", patterns, err)
		}

		for _, file := range xmlFiles {
			f, err := os.Open(file)
			if err != nil {
				fmt.Printf("unable to open file '%s': %v\n", file, err)
				continue
			}
			// TODO This should start working if/when this merges and releases: https://github.com/golang/go/pull/56848
			var doc *xmlquery.Node
			doc, err = xmlquery.ParseWithOptions(f, xmlquery.ParserOptions{Decoder: &xmlquery.DecoderOptions{Strict: false}})
			if err != nil {
				if err.Error() == "xml: unsupported version \"1.1\"; only version 1.0 is supported" {
					// TODO HACK just pretend 1.1 xml documents are 1.0 for now while we wait for golang to support 1.1
					b, err := os.ReadFile(file)
					if err != nil {
						fmt.Printf("unable to parse xml file '%s': %v\n", file, err)
						continue
					}
					docString := strings.Replace(string(b), "<?xml version=\"1.1\"", "<?xml version = \"1.0\"", 1)
					doc, err = xmlquery.Parse(strings.NewReader(docString))
					if err != nil {
						fmt.Printf("unable to parse xml file '%s': %v\n", file, err)
						continue
					}
				} else {
					fmt.Printf("unable to parse xml file '%s': %v\n", file, err)
					continue
				}
			}
			list := xmlquery.QuerySelectorAll(doc, query)
			if len(list) != 0 {
				response.Matched = true
				for _, node := range list {
					ab, err := filepath.Abs(file)
					if err != nil {
						ab = file
					}
					incident := provider.IncidentContext{
						FileURI: uri.File(ab),
						Variables: map[string]interface{}{
							"matchingXML": node.OutputXML(false),
							"innerText":   node.InnerText(),
							"data":        node.Data,
						},
					}
					location, err := p.getLocation(ctx, ab, node.InnerText())
					if err == nil {
						incident.CodeLocation = &location
						lineNo := int(location.StartPosition.Line)
						incident.LineNumber = &lineNo
					}
					response.Incidents = append(response.Incidents, incident)
				}
			}
		}

		return response, nil
	case "json":
		query := cond.JSON.XPath
		if query == "" {
			return response, fmt.Errorf("could not parse provided xpath query as string: %v", conditionInfo)
		}
		pattern := "*.json"
		jsonFiles, err := provider.GetFiles(p.config.Location, cond.JSON.Filepaths, pattern)
		if err != nil {
			return response, fmt.Errorf("unable to find files using pattern `%s`: %v", pattern, err)
		}
		for _, file := range jsonFiles {
			f, err := os.Open(file)
			if err != nil {
				p.log.V(5).Error(err, "error opening json file", "file", file)
				continue
			}
			doc, err := jsonquery.Parse(f)
			if err != nil {
				p.log.V(5).Error(err, "error parsing json file", "file", file)
				continue
			}
			list, err := jsonquery.QueryAll(doc, query)
			if err != nil {
				return response, err
			}
			if len(list) != 0 {
				response.Matched = true
				for _, node := range list {
					ab, err := filepath.Abs(file)
					if err != nil {
						ab = file
					}
					incident := provider.IncidentContext{
						FileURI: uri.File(ab),
						Variables: map[string]interface{}{
							"matchingJSON": node.InnerText(),
							"data":         node.Data,
						},
					}
					location, err := p.getLocation(ctx, ab, node.InnerText())
					if err == nil {
						incident.CodeLocation = &location
						lineNo := int(location.StartPosition.Line)
						incident.LineNumber = &lineNo
					}
					response.Incidents = append(response.Incidents, incident)
				}
			}
		}
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
		if line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) < 1 {
		return location, fmt.Errorf("unable to get code location, no-op content")
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

func findFilesMatchingPattern(root, pattern string) ([]string, error) {
	var regex *regexp.Regexp
	// if the regex doesn't compile, we'll default to using filepath.Match on the pattern directly
	regex, _ = regexp.Compile(pattern)
	matches := []string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		var matched bool
		if regex != nil {
			matched = regex.MatchString(d.Name())
		} else {
			// TODO(fabianvf): is a fileglob style pattern sufficient or do we need regexes?
			matched, err = filepath.Match(pattern, d.Name())
			if err != nil {
				return err
			}
		}
		if matched {
			matches = append(matches, path)
		}
		return nil
	})
	return matches, err
}

type walkResult struct {
	positionParams protocol.TextDocumentPositionParams
	match          string
}

func parallelWalk(location string, regex *regexp.Regexp) ([]walkResult, error) {
	var positions []walkResult
	positionsChan := make(chan walkResult)
	wg := &sync.WaitGroup{}

	go func() {
		err := filepath.Walk(location, func(path string, f os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if f.Mode().IsRegular() {
				wg.Add(1)
				go processFile(path, regex, positionsChan, wg)
			}

			return nil
		})

		if err != nil {
			return
		}

		wg.Wait()
		close(positionsChan)
	}()

	for pos := range positionsChan {
		positions = append(positions, pos)
	}

	return positions, nil
}

func processFile(path string, regex *regexp.Regexp, positionsChan chan<- walkResult, wg *sync.WaitGroup) {
	defer wg.Done()

	content, err := os.ReadFile(path)
	if err != nil {
		return
	}

	// Must go through each line,
	forceFullFileScan := false
	if strings.Contains(regex.String(), "^") {
		forceFullFileScan = true
	}

	if regex.Match(content) || forceFullFileScan {
		scanner := bufio.NewScanner(strings.NewReader(string(content)))
		lineNumber := 1
		for scanner.Scan() {
			matchLocations := regex.FindAllStringIndex(scanner.Text(), -1)
			matchStrings := regex.FindAllString(scanner.Text(), -1)
			for i, loc := range matchLocations {
				absPath, err := filepath.Abs(path)
				if err != nil {
					return
				}
				positionsChan <- walkResult{
					positionParams: protocol.TextDocumentPositionParams{
						TextDocument: protocol.TextDocumentIdentifier{
							URI: fmt.Sprintf("file://%s", absPath),
						},
						Position: protocol.Position{
							Line:      uint32(lineNumber),
							Character: uint32(loc[1]),
						},
					},
					match: matchStrings[i],
				}
			}
			lineNumber++
		}
	}
}
