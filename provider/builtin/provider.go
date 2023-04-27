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
	"github.com/antchfx/xpath"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/dependency/dependency"
	"github.com/konveyor/analyzer-lsp/jsonrpc2"
	"github.com/konveyor/analyzer-lsp/provider/lib"
	"go.lsp.dev/uri"
	"gopkg.in/yaml.v2"
)

const TAGS_FILE_INIT_OPTION = "tagsFile"

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
	XPath      string            `yaml:"xpath"`
	Namespaces map[string]string `yaml:"namespaces"`
	Filepaths  []string          `yaml:"filepaths"`
}

type jsonCondition struct {
	XPath string `yaml:'xpath'`
}

type builtinProvider struct {
	rpc *jsonrpc2.Conn
	ctx context.Context

	config lib.Config
	tags   map[string]bool
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
			ab, err := filepath.Abs(filepath.Join(p.config.Location, match))
			if err != nil {
				//TODO: Probably want to log or something to let us know we can't get absolute path here.
				fmt.Printf("\n\n\n%v", err)
				ab = match
			}
			fmt.Printf("\n\nPath Info: %#v\nmatch: %v\nconfigLocation: %v", ab, match, p.config.Location)
			response.Incidents = append(response.Incidents, lib.IncidentContext{
				FileURI: uri.File(ab),
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
			if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 1 {
				return response, nil
			}
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
			ab, err := filepath.Abs(pieces[0])
			if err != nil {
				ab = pieces[0]
			}
			response.Incidents = append(response.Incidents, lib.IncidentContext{
				FileURI: uri.File(ab),
				Variables: map[string]interface{}{
					"lineNumber":   pieces[1],
					"matchingText": pieces[2],
				},
			})
		}
		return response, nil
	case "xml":
		query, err := xpath.CompileWithNS(cond.XML.XPath, cond.XML.Namespaces)
		if query == nil || err != nil {
			return response, fmt.Errorf("Could not parse provided xpath query '%s': %v", cond.XML.XPath, err)
		}
		//TODO(fabianvf): how should we scope the files searched here?
		var xmlFiles []string
		if len(cond.XML.Filepaths) == 0 {
			pattern := "*.xml"
			xmlFiles, err = findFilesMatchingPattern(p.config.Location, pattern)
			if err != nil {
				return response, fmt.Errorf("Unable to find files using pattern `%s`: %v", pattern, err)
			}
			xhtmlFiles, err := findFilesMatchingPattern(p.config.Location, "*.xhtml")
			if err != nil {
				return response, fmt.Errorf("Unable to find files using pattern `%s`: %v", "*.xhtml", err)
			}
			xmlFiles = append(xmlFiles, xhtmlFiles...)
		} else if len(cond.XML.Filepaths) == 1 {
			// Currently, rendering will render a list as a space seperated paths as a single string.
			patterns := strings.Split(cond.XML.Filepaths[0], " ")
			for _, pattern := range patterns {
				files, err := findFilesMatchingPattern(p.config.Location, pattern)
				if err != nil {
					// Something went wrong dealing with the pattern, so we'll assume the user input
					// is good and pass it on
					// TODO(fabianvf): if we're ever hitting this for real we should investigate
					fmt.Printf("Unable to resolve pattern '%s': %v", pattern, err)
					xmlFiles = append(xmlFiles, pattern)
				} else {
					xmlFiles = append(xmlFiles, files...)
				}
			}
		} else {
			for _, pattern := range cond.XML.Filepaths {
				files, err := findFilesMatchingPattern(p.config.Location, pattern)
				if err != nil {
					xmlFiles = append(xmlFiles, pattern)
				} else {
					xmlFiles = append(xmlFiles, files...)
				}
			}
		}
		for _, file := range xmlFiles {
			if !strings.HasPrefix(file, "/") {
				file = filepath.Join(p.config.Location, file)
			}
			absPath, err := filepath.Abs(file)
			if err != nil {
				fmt.Printf("unable to get absolute path for '%s': %v\n", file, err)
				continue
			}
			f, err := os.Open(absPath)
			if err != nil {
				fmt.Printf("unable to open file '%s': %v\n", absPath, err)
				continue
			}
			// TODO This should start working if/when this merges and releases: https://github.com/golang/go/pull/56848
			var doc *xmlquery.Node
			doc, err = xmlquery.ParseWithOptions(f, xmlquery.ParserOptions{Decoder: &xmlquery.DecoderOptions{Strict: false}})
			if err != nil {
				if err.Error() == "xml: unsupported version \"1.1\"; only version 1.0 is supported" {
					// TODO HACK just pretend 1.1 xml documents are 1.0 for now while we wait for golang to support 1.1
					b, err := os.ReadFile(absPath)
					if err != nil {
						fmt.Printf("unable to parse xml file '%s': %v\n", absPath, err)
						continue
					}
					docString := strings.Replace(string(b), "<?xml version=\"1.1\"", "<?xml version = \"1.0\"", 1)
					doc, err = xmlquery.Parse(strings.NewReader(docString))
					if err != nil {
						fmt.Printf("unable to parse xml file '%s': %v\n", absPath, err)
						continue
					}
				} else {
					fmt.Printf("unable to parse xml file '%s': %v\n", absPath, err)
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
					response.Incidents = append(response.Incidents, lib.IncidentContext{
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
					ab, err := filepath.Abs(file)
					if err != nil {
						ab = file
					}
					response.Incidents = append(response.Incidents, lib.IncidentContext{
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
			response.Incidents = append(response.Incidents, lib.IncidentContext{
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
	err := p.loadTags()
	if err != nil {
		return err
	}
	return nil
}

func (p *builtinProvider) loadTags() error {
	tagsFile := p.config.ProviderSpecificConfig[TAGS_FILE_INIT_OPTION]

	p.tags = make(map[string]bool)
	if tagsFile == "" {
		return nil
	}
	content, err := os.ReadFile(tagsFile)
	if err != nil {
		return err
	}
	var tags []string
	err = yaml.Unmarshal(content, &tags)
	if err != nil {
		return err
	}
	for _, tag := range tags {
		p.tags[tag] = true
	}
	return nil
}

// We don't have dependencies
func (p *builtinProvider) GetDependencies() ([]dependency.Dep, uri.URI, error) {
	return nil, "", nil
}

// We don't have dependencies
func (p *builtinProvider) GetDependenciesLinkedList() (map[dependency.Dep][]dependency.Dep, uri.URI, error) {
	return nil, "", nil
}
