package golang

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/jsonrpc2"
	"github.com/konveyor/analyzer-lsp/provider"
)

// TODO(shawn-hurley): Pipe the logger through
// Determine how and where external providers will add the logs to make the logs viewable in a single location.
type golangProvider struct {
	ctx context.Context
}

var _ provider.BaseClient = &golangProvider{}

func NewGolangProvider() *golangProvider {
	return &golangProvider{}
}

func (p *golangProvider) Capabilities() []provider.Capability {
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

type golangCondition struct {
	Referenced string `yaml:"referenced"`
}

func (p *golangProvider) Init(ctx context.Context, log logr.Logger, c provider.InitConfig) (provider.ServiceClient, error) {
	ctx, cancelFunc := context.WithCancel(ctx)
	log = log.WithValues("provider", "golang")
	cmd := exec.CommandContext(ctx, c.LSPServerPath)
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

	svcClient := golangServiceClient{
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
