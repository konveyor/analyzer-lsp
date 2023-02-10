package builtin

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/antchfx/jsonquery"
	"github.com/antchfx/xmlquery"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/dependency/dependency"
	"github.com/konveyor/analyzer-lsp/jsonrpc2"
	"github.com/konveyor/analyzer-lsp/provider/lib"
	"gopkg.in/yaml.v2"
)

var capabilities = []lib.Capability{
	{
		Name:            "filecontent",
		TemplateContext: openapi3.SchemaRef{},
	},
	{
		Name: "file",
		TemplateContext: openapi3.SchemaRef{
			Value: &openapi3.Schema{
				Properties: openapi3.Schemas{
					"filepaths": &openapi3.SchemaRef{
						Value: &openapi3.Schema{
							Description: "List of filepaths matching pattern",
							Items: &openapi3.SchemaRef{
								Value: &openapi3.Schema{
									Type: "string",
								},
							},
						},
					},
				},
			},
		},
	},
	{
		Name:            "xml",
		TemplateContext: openapi3.SchemaRef{},
	},
	{
		Name:            "json",
		TemplateContext: openapi3.SchemaRef{},
	},
	{
		Name:            "hasTags",
		TemplateContext: openapi3.SchemaRef{},
	},
}

type builtinCondition struct {
	Filecontent         string        `yaml:"filecontent"`
	File                string        `yaml:"file"`
	XML                 xmlCondition  `yaml:"xml"`
	JSON                jsonCondition `yaml:"json"`
	HasTags             []string      `yaml:"hasTags"`
	lib.ProviderContext `yaml:",inline"`
}

type xmlCondition struct {
	XPath     string   `yaml:"xpath"`
	Filepaths []string `yaml:"filepaths"`
}

type jsonCondition struct {
	XPath string `yaml:'xpath'`
}

type builtinProvider struct {
	rpc *jsonrpc2.Conn
	ctx context.Context

	config lib.Config
}

func NewBuiltinProvider(config lib.Config) *builtinProvider {
	return &builtinProvider{
		config: config,
	}
}

func (p *builtinProvider) Stop() {
	return
}

func (p *builtinProvider) Capabilities() []lib.Capability {
	return capabilities
}

func (p *builtinProvider) HasCapability(name string) bool {
	return lib.HasCapability(p.Capabilities(), name)
}

func (p *builtinProvider) Evaluate(cap string, conditionInfo []byte) (lib.ProviderEvaluateResponse, error) {
	var cond builtinCondition
	err := yaml.Unmarshal(conditionInfo, &cond)
	if err != nil {
		return lib.ProviderEvaluateResponse{}, fmt.Errorf("unable to get query info: %v", err)
	}
	response := lib.ProviderEvaluateResponse{Matched: false}
	switch cap {
	case "file":
		pattern := cond.File
		if pattern == "" {
			return response, fmt.Errorf("could not parse provided file pattern as string: %v", conditionInfo)
		}
		matchingFiles, err := findFilesMatchingPattern(p.config.Location, pattern)
		if err != nil {
			return response, fmt.Errorf("unable to find files using pattern `%s`: %v", pattern, err)
		}

		if len(matchingFiles) != 0 {
			response.Matched = true
		}

		response.TemplateContext = map[string]interface{}{"filepaths": matchingFiles}
		for _, match := range matchingFiles {
			response.Incidents = append(response.Incidents, lib.IncidentContext{
				FileURI: match,
			})
		}
		return response, nil
	case "filecontent":
		pattern := cond.Filecontent
		if pattern == "" {
			return response, fmt.Errorf("could not parse provided regex pattern as string: %v", conditionInfo)
		}
		var outputBytes []byte
		grep := exec.Command("grep", "-o", "-n", "-R", "-E", pattern, p.config.Location)
		outputBytes, err := grep.Output()
		if err != nil {
			return response, fmt.Errorf("could not run grep with provided pattern %+v", err)
		}
		matches := strings.Split(strings.TrimSpace(string(outputBytes)), "\n")
		if len(matches) != 0 {
			response.Matched = true
		}

		for _, match := range matches {
			//TODO(fabianvf): This will not work if there is a `:` in the filename, do we care?
			pieces := strings.SplitN(match, ":", 3)
			if len(pieces) != 3 {
				//TODO(fabianvf): Just log or return?
				return response, fmt.Errorf("Malformed response from grep, cannot parse %s with pattern {filepath}:{lineNumber}:{matchingText}", match)
			}
			response.Incidents = append(response.Incidents, lib.IncidentContext{
				FileURI: fmt.Sprintf("file://%s", pieces[0]),
				Extras: map[string]interface{}{
					"lineNumber":   pieces[1],
					"matchingText": pieces[2],
				},
			})
		}
		return response, nil
	case "xml":
		query := cond.XML.XPath
		if query == "" {
			return response, fmt.Errorf("Could not parse provided xpath query as string: %v", conditionInfo)
		}
		//TODO(fabianvf): how should we scope the files searched here?
		var xmlFiles []string
		if len(cond.XML.Filepaths) == 0 {
			pattern := "*.xml"
			xmlFiles, err = findFilesMatchingPattern(p.config.Location, pattern)
			if err != nil {
				return response, fmt.Errorf("Unable to find files using pattern `%s`: %v", pattern, err)
			}
		} else if len(cond.XML.Filepaths) == 1 {
			// Currently, rendering will render a list as a space seperated paths as a single string.
			xmlFiles = strings.Split(cond.XML.Filepaths[0], " ")
		} else {
			xmlFiles = cond.XML.Filepaths
		}
		for _, file := range xmlFiles {
			absPath, err := filepath.Abs(file)
			if err != nil {
				fmt.Printf("unable to get abs path: %v", err)
			}
			f, err := os.Open(absPath)
			if err != nil {
				fmt.Printf("unable to open file: %v", err)
			}
			doc, err := xmlquery.Parse(f)
			if err != nil {
				fmt.Printf("unable to query file: %v", err)
				continue
				// return response, err
			}
			list, err := xmlquery.QueryAll(doc, query)
			if err != nil {
				continue
				// return response, err
			}
			if len(list) != 0 {
				response.Matched = true
				for _, node := range list {
					response.Incidents = append(response.Incidents, lib.IncidentContext{
						FileURI: fmt.Sprintf("file://%s", file),
						Extras: map[string]interface{}{
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
		jsonFiles, err := findFilesMatchingPattern(p.config.Location, pattern)
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
					response.Incidents = append(response.Incidents, lib.IncidentContext{
						FileURI: fmt.Sprintf("file://%s", file),
						Extras: map[string]interface{}{
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
				found = false
				break
			}
		}
		if found {
			response.Matched = true
			response.Incidents = append(response.Incidents, lib.IncidentContext{
				Extras: map[string]interface{}{
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
	matches := []string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// TODO(fabianvf): is a fileglob style pattern sufficient or do we need regexes?
		matched, err := filepath.Match(pattern, d.Name())
		if err != nil {
			return err
		}
		if matched {
			matches = append(matches, path)
		}
		return nil
	})
	return matches, err
}

// We don't need to init anything
func (p *builtinProvider) Init(_ context.Context, _ logr.Logger) error {
	return nil
}

// We don't have dependencies
func (p *builtinProvider) GetDependencies() ([]dependency.Dep, error) {
	return nil, nil
}

// We don't have dependencies
func (p *builtinProvider) GetDependenciesLinkedList() (map[dependency.Dep][]dependency.Dep, error) {
	return nil, nil
}
