package builtin

import (
	"context"
	"os"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/swaggest/openapi-go/openapi3"
	"gopkg.in/yaml.v2"
)

const TAGS_FILE_INIT_OPTION = "tagsFile"

var (
	filePathsDescription = "file pattern to search"
)

var capabilities = []provider.Capability{}

type builtinCondition struct {
	Filecontent              fileContentCondition `yaml:"filecontent"`
	File                     fileCondition        `yaml:"file"`
	XML                      xmlCondition         `yaml:"xml"`
	XMLPublicID              xmlPublicIDCondition `yaml:"xmlPublicID"`
	JSON                     jsonCondition        `yaml:"json"`
	HasTags                  []string             `yaml:"hasTags"`
	provider.ProviderContext `yaml:",inline"`
}

type fileContentCondition struct {
	FilePattern string `yaml:"filePattern" json:"filePattern,omitempty" title:"FilePattern" description:"Only search in files with names matching this pattern"`
	Pattern     string `yaml:"pattern" json:"pattern" title:"Pattern" description:"Regex pattern to match in content"`
}

type fileCondition struct {
	Pattern string `yaml:"pattern" json:"pattern" title:"Pattern" description:"Find files with names matching this pattern"`
}

var _ provider.InternalProviderClient = &builtinProvider{}

type xmlCondition struct {
	XPath      string            `yaml:"xpath" json:"xpath" title:"XPath" description:"Xpath query"`
	Namespaces map[string]string `yaml:"namespaces" json:"namespace,omitempty" title:"Namespaces" description:"A map to scope down query to namespaces"`
	Filepaths  []string          `yaml:"filepaths" json:"filepaths,omitempty" title:"Filepaths" description:"Optional list of files to scope down search"`
}

type xmlPublicIDCondition struct {
	Regex      string            `yaml:"regex" json:"regex"`
	Namespaces map[string]string `yaml:"namespaces" json:"namespaces" title:"Namespaces" description:"A map to scope down query to namespaces"`
	Filepaths  []string          `yaml:"filepaths" json:"filepaths" title:"Filepaths" description:"Optional list of files to scope down search"`
}

type jsonCondition struct {
	XPath     string   `yaml:"xpath" json:"xpath" title:"XPath" description:"Xpath query"`
	Filepaths []string `yaml:"filepaths" json:"filepaths,omitempty" title:"Filepaths" description:"Optional list of files to scope down search"`
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
	r := openapi3.NewReflector()

	caps := []provider.Capability{}
	jsonCap, err := provider.ToProviderCap(r, p.log, jsonCondition{}, "json")
	if err != nil {
		p.log.Error(err, "unable to get json capability")
	} else {
		caps = append(caps, jsonCap)
	}

	xmlCap, err := provider.ToProviderCap(r, p.log, xmlCondition{}, "xml")
	if err != nil {
		p.log.Error(err, "unable to get xml capability")
	} else {
		caps = append(caps, xmlCap)
	}

	filecontentCap, err := provider.ToProviderCap(r, p.log, fileContentCondition{}, "filecontent")
	if err != nil {
		p.log.Error(err, "unable to get filecontent capability")
	} else {
		caps = append(caps, filecontentCap)
	}

	fileCap, err := provider.ToProviderInputOutputCap(r, p.log, fileCondition{}, fileTemplateContext{}, "file")
	if err != nil {
		p.log.Error(err, "unable to get file capability")
	} else {
		caps = append(caps, fileCap)
	}

	xmlPublicIDCap, err := provider.ToProviderCap(r, p.log, xmlPublicIDCondition{}, "xmlPublicID")
	if err != nil {
		p.log.Error(err, "unable to get xmlPublicID capability")
	} else {
		caps = append(caps, xmlPublicIDCap)
	}

	hasTags, err := provider.ToProviderCap(r, p.log, []string{}, "hasTags")
	if err != nil {
		p.log.Error(err, "unable to get hasTags capability")
	} else {
		caps = append(caps, hasTags)
	}

	return caps
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
		includedPaths:                      provider.GetIncludedPathsFromConfig(config, true),
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
