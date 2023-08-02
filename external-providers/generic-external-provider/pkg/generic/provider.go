package generic

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/jsonrpc2"
	"github.com/konveyor/analyzer-lsp/provider"
)

// TODO(shawn-hurley): Pipe the logger through
// Determine how and where external providers will add the logs to make the logs viewable in a single location.
type genericProvider struct {
	ctx context.Context
}

var _ provider.BaseClient = &genericProvider{}

func NewGenericProvider() *genericProvider {
	return &genericProvider{}
}

func (p *genericProvider) Capabilities() []provider.Capability {
	return []provider.Capability{
		{
			Name:            "referenced",
			TemplateContext: openapi3.SchemaRef{},
		},
		{
			Name:            "dependency",
			TemplateContext: openapi3.SchemaRef{},
		},
	}
}

type genericCondition struct {
	Referenced referenceCondition `yaml:"referenced"`
}

type referenceCondition struct {
	Pattern string `yaml:"pattern"`
}

func (p *genericProvider) Init(ctx context.Context, log logr.Logger, c provider.InitConfig) (provider.ServiceClient, error) {
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
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	go func() {
		err := cmd.Start()
		if err != nil {
			fmt.Printf("cmd failed - %v", err)
			// TODO: Probably should cancel the ctx here, to shut everything down
			return
		}
	}()
	rpc := jsonrpc2.NewConn(jsonrpc2.NewHeaderStream(stdout, stdin), log)

	go func() {
		err := rpc.Run(ctx)
		if err != nil {
			//TODO: we need to pipe the ctx further into the stream header and run.
			// basically it is checking if done, then reading. When it gets EOF it errors.
			// We need the read to be at the same level of selection to fully implment graceful shutdown
			return
		}
	}()

	svcClient := genericServiceClient{
		rpc:        rpc,
		ctx:        ctx,
		cancelFunc: cancelFunc,
		cmd:        cmd,
		config:     c,
	}

	// Lets Initiallize before returning
	svcClient.initialization(ctx, log)
	return &svcClient, nil
}
