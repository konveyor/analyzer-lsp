package builtin

import (
	"context"
	"os"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/provider"
	"gopkg.in/yaml.v2"
)

const TAGS_FILE_INIT_OPTION = "tagsFile"

var capabilities = []provider.Capability{
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
	Filecontent              fileContentCondition `yaml:"filecontent"`
	File                     fileCondition        `yaml:"file"`
	XML                      xmlCondition         `yaml:"xml"`
	JSON                     jsonCondition        `yaml:"json"`
	HasTags                  []string             `yaml:"hasTags"`
	provider.ProviderContext `yaml:",inline"`
}

type fileContentCondition struct {
	FilePattern string `yaml:"filePattern"`
	Pattern     string `yaml:"pattern`
}

type fileCondition struct {
	Pattern string `yaml:"pattern"`
}

var _ provider.InternalProviderClient = &builtinProvider{}

type xmlCondition struct {
	XPath      string            `yaml:"xpath"`
	Namespaces map[string]string `yaml:"namespaces"`
	Filepaths  []string          `yaml:"filepaths"`
}

type jsonCondition struct {
	XPath     string   `yaml:'xpath'`
	Filepaths []string `yaml:"filepaths"`
}

type builtinProvider struct {
	ctx context.Context
	log logr.Logger

	config provider.Config
	tags   map[string]bool
	provider.UnimplementedDependenciesComponent

	clients []provider.ServiceClient
}

func NewBuiltinProvider(config provider.Config, log logr.Logger) *builtinProvider {
	return &builtinProvider{
		config: config,
		log:    log,
	}
}

func (p *builtinProvider) Capabilities() []provider.Capability {
	return capabilities
}

func (p *builtinProvider) ProviderInit(ctx context.Context) error {
	// First load all the tags for all init configs.
	for _, c := range p.config.InitConfig {
		p.loadTags(c)
	}

	for _, c := range p.config.InitConfig {
		client, err := p.Init(ctx, p.log, c)
		if err != nil {
			return nil
		}
		p.clients = append(p.clients, client)
	}
	return nil
}

// We don't need to init anything
func (p *builtinProvider) Init(ctx context.Context, log logr.Logger, config provider.InitConfig) (provider.ServiceClient, error) {
	if config.AnalysisMode != provider.AnalysisMode("") {
		p.log.V(5).Info("skipping analysis mode setting for builtin")
	}
	return &builtinServiceClient{
		config:                             config,
		tags:                               p.tags,
		UnimplementedDependenciesComponent: provider.UnimplementedDependenciesComponent{},
		locationCache:                      make(map[string]float64),
		log:                                log,
	}, nil
}

func (p *builtinProvider) loadTags(config provider.InitConfig) error {
	tagsFile, ok := config.ProviderSpecificConfig[TAGS_FILE_INIT_OPTION].(string)
	// for now, if the tags file is invalid, lets ignore
	if !ok {
		return nil
	}

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

func (p *builtinProvider) Evaluate(ctx context.Context, cap string, conditionInfo []byte) (provider.ProviderEvaluateResponse, error) {
	return provider.FullResponseFromServiceClients(ctx, p.clients, cap, conditionInfo)
}

func (p *builtinProvider) Stop() {
	return
}
