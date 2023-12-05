package generic

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/provider"
)

// TODO(shawn-hurley): Pipe the logger through
// Determine how and where external providers will add the logs to make the logs viewable in a single location.
type yqProvider struct {
	ctx context.Context
}

var _ provider.BaseClient = &yqProvider{}

func NewYqProvider() *yqProvider {
	return &yqProvider{}
}

func (p *yqProvider) Capabilities() []provider.Capability {
	return []provider.Capability{
		{
			Name:            "referenced",
			TemplateContext: openapi3.SchemaRef{},
		},
		{
			Name:            "dependency",
			TemplateContext: openapi3.SchemaRef{},
		},
		{
			Name:            "k8sResourceMatched",
			TemplateContext: openapi3.SchemaRef{},
		},
	}
}

type genericCondition struct {
	Referenced         referenceCondition   `yaml:"referenced"`
	K8sResourceMatched k8sResourceCondition `yaml:"k8sResourceMatched"`
}

type referenceCondition struct {
	Pattern string `yaml:"pattern"`
	Key     string `yaml:"key"`
	Value   string `yaml:"value"`
}

type k8sResourceCondition struct {
	ApiVersion     string `yaml:"apiVersion"`
	Kind           string `yaml:"kind"`
	DeprecatedIn   string `yaml:"deprecatedIn"`
	RemovedIn      string `yaml:"removedIn"`
	ReplacementAPI string `yaml:"replacementAPI"`
}

type k8sOutput struct {
	ApiVersion k8skey
	Kind       k8skey
	URI        string
}

type k8skey struct {
	Value      string
	LineNumber string
}

func (p *yqProvider) Init(ctx context.Context, log logr.Logger, c provider.InitConfig) (provider.ServiceClient, error) {
	if c.AnalysisMode != provider.FullAnalysisMode {
		return nil, fmt.Errorf("only full analysis is supported")
	}

	if c.Proxy != nil {
		// handle proxy settings
		for k, v := range c.Proxy.ToEnvVars() {
			os.Setenv(k, v)
		}
	}

	lspServerPath, ok := c.ProviderSpecificConfig[provider.LspServerPathConfigKey].(string)
	if !ok || lspServerPath == "" {
		return nil, fmt.Errorf("invalid lspServerPath provided, unable to init go provider")
	}

	ctx, cancelFunc := context.WithCancel(ctx)
	log = log.WithValues("provider", c.ProviderSpecificConfig["name"])
	var args []string
	if lspArgs, ok := c.ProviderSpecificConfig["lspArgs"]; ok {
		rawArgs, isArray := lspArgs.([]interface{})
		if !isArray {
			return nil, fmt.Errorf("lspArgs is not an array")
		}
		for _, rawArg := range rawArgs {
			if arg, ok := rawArg.(string); ok {
				args = append(args, arg)
			} else {
				return nil, fmt.Errorf("item of lspArgs is not a string")
			}
		}
	}
	cmd := exec.CommandContext(ctx, lspServerPath, args...)

	go func() {
		err := cmd.Run()
		if err != nil {
			fmt.Printf("cmd failed - %v", err)
			// TODO: Probably should cancel the ctx here, to shut everything down
			return
		}
	}()

	svcClient := genericServiceClient{
		cancelFunc: cancelFunc,
		log:        log,
		cmd:        cmd,
		config:     c,
	}

	return &svcClient, nil
}
