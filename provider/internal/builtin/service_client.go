package builtin

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/antchfx/jsonquery"
	"github.com/antchfx/xmlquery"
	"github.com/antchfx/xpath"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
	"gopkg.in/yaml.v2"
)

type builtintServiceClient struct {
	config provider.InitConfig
	tags   map[string]bool
	provider.UnimplementedDependenciesComponent
}

var _ provider.ServiceClient = &builtintServiceClient{}

func (p *builtintServiceClient) Stop() {
	return
}

func (p *builtintServiceClient) Evaluate(cap string, conditionInfo []byte) (provider.ProviderEvaluateResponse, error) {
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
		grep := exec.Command("grep", "-o", "-n", "-R", "-E", c.Pattern, p.config.Location)
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
				return response, fmt.Errorf("Cannot convert line number string to integer")
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
			return response, fmt.Errorf("Could not parse provided xpath query '%s': %v", cond.XML.XPath, err)
		}
		//TODO(fabianvf): how should we scope the files searched here?
		var xmlFiles []string
		patterns := []string{"*.xml", "*.xhtml"}
		xmlFiles, err = provider.GetFiles(p.config.Location, cond.XML.Filepaths, patterns...)
		if err != nil {
			return response, fmt.Errorf("Unable to find files using pattern `%s`: %v", patterns, err)
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
					response.Incidents = append(response.Incidents, provider.IncidentContext{
						FileURI: uri.File(ab),
						Variables: map[string]interface{}{
							"matchingXML": node.OutputXML(false),
							"innerText":   node.InnerText(),
							"data":        node.Data,
						},
					})
				}
			}
		}

		return response, nil
	case "json":
		query := cond.JSON.XPath
		if query == "" {
			return response, fmt.Errorf("Could not parse provided xpath query as string: %v", conditionInfo)
		}
		pattern := "*.json"
		jsonFiles, err := provider.GetFiles(p.config.Location, cond.JSON.Filepaths, pattern)
		if err != nil {
			return response, fmt.Errorf("Unable to find files using pattern `%s`: %v", pattern, err)
		}
		for _, file := range jsonFiles {
			f, err := os.Open(file)
			doc, err := jsonquery.Parse(f)
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
					response.Incidents = append(response.Incidents, provider.IncidentContext{
						FileURI: uri.File(ab),
						Variables: map[string]interface{}{
							"matchingJSON": node.InnerText(),
							"data":         node.Data,
						},
					})
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
