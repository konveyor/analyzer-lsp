package builtin

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/antchfx/jsonquery"
	"github.com/antchfx/xmlquery"
	"github.com/antchfx/xpath"
	"github.com/go-logr/logr"
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
		var outputBytes []byte
		grep := exec.Command("grep", "-o", "-n", "-R", "-P", c.Pattern, p.config.Location)
		outputBytes, err := grep.Output()
		if err != nil {
			if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 1 {
				return response, nil
			}
			return response, fmt.Errorf("could not run grep with provided pattern %+v", err)
		}

		matches := []string{}
		outputString := strings.TrimSpace(string(outputBytes))
		if outputString != "" {
			matches = append(matches, strings.Split(outputString, "\n")...)
		}

		for _, match := range matches {
			//TODO(fabianvf): This will not work if there is a `:` in the filename, do we care?
			pieces := strings.SplitN(match, ":", 3)
			if len(pieces) != 3 {
				//TODO(fabianvf): Just log or return?
				//(shawn-hurley): I think the return is good personally
				return response, fmt.Errorf(
					"malformed response from grep, cannot parse grep output '%s' with pattern {filepath}:{lineNumber}:{matchingText}", match)
			}

			containsFile, err := provider.FilterFilePattern(c.FilePattern, pieces[0])
			if err != nil {
				return response, err
			}
			if !containsFile {
				continue
			}

			ab, err := filepath.Abs(pieces[0])
			if err != nil {
				ab = pieces[0]
			}
			lineNumber, err := strconv.Atoi(pieces[1])
			if err != nil {
				return response, fmt.Errorf("cannot convert line number string to integer")
			}
			response.Incidents = append(response.Incidents, provider.IncidentContext{
				FileURI:    uri.File(ab),
				LineNumber: &lineNumber,
				Variables: map[string]interface{}{
					"matchingText": pieces[2],
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
		xmlFiles, err := findXMLFiles(p.config.Location, cond.XMLPublicID.Filepaths)
		if err != nil {
			return response, fmt.Errorf("unable to find XML files: %v", err)
		}

		for _, file := range xmlFiles {
			nodes, err := queryXMLFile(file, query)
			if err != nil {
				p.log.V(5).Error(err, "failed to query xml file", "file", file)
				continue
			}
			if len(nodes) != 0 {
				response.Matched = true
				for _, node := range nodes {
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
					content := strings.TrimSpace(node.InnerText())
					if content == "" {
						content = node.Data
					}
					location, err := p.getLocation(ctx, ab, content)
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
	case "xmlPublicID":
		regex, err := regexp.Compile(cond.XMLPublicID.Regex)
		if err != nil {
			return response, fmt.Errorf("could not parse provided public-id regex '%s': %v", cond.XMLPublicID.Regex, err)
		}
		query, err := xpath.CompileWithNS("//*[@public-id]", cond.XMLPublicID.Namespaces)
		if query == nil || err != nil {
			return response, fmt.Errorf("could not parse public-id xml query '%s': %v", cond.XML.XPath, err)
		}
		xmlFiles, err := findXMLFiles(p.config.Location, cond.XMLPublicID.Filepaths)
		if err != nil {
			return response, fmt.Errorf("unable to find XML files: %v", err)
		}

		for _, file := range xmlFiles {
			nodes, err := queryXMLFile(file, query)
			if err != nil {
				p.log.V(5).Error(err, "failed to query xml file", "file", file)
				continue
			}

			for _, node := range nodes {
				// public-id attribute regex match check
				for _, attr := range node.Attr {
					if attr.Name.Local == "public-id" {
						if regex.MatchString(attr.Value) {
							response.Matched = true
							ab, err := filepath.Abs(file)
							if err != nil {
								ab = file
							}
							response.Incidents = append(response.Incidents, provider.IncidentContext{
								FileURI: uri.File(ab),
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

func findXMLFiles(baseLocation string, filePaths []string) ([]string, error) {
	patterns := []string{"*.xml", "*.xhtml"}
	// TODO(fabianvf): how should we scope the files searched here?
	xmlFiles, err := provider.GetFiles(baseLocation, filePaths, patterns...)
	return xmlFiles, err
}

func queryXMLFile(filePath string, query *xpath.Expr) (nodes []*xmlquery.Node, err error) {
	f, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("unable to open file '%s': %v\n", filePath, err)
		return nil, err
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
				fmt.Printf("unable to parse xml file '%s': %v\n", filePath, err)
				return nil, err
			}
			docString := strings.Replace(string(b), "<?xml version=\"1.1\"", "<?xml version = \"1.0\"", 1)
			doc, err = xmlquery.Parse(strings.NewReader(docString))
			if err != nil {
				fmt.Printf("unable to parse xml file '%s': %v\n", filePath, err)
				return nil, err
			}
		} else {
			fmt.Printf("unable to parse xml file '%s': %v\n", filePath, err)
			return nil, err
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
