package yq_provider

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"io"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/swaggest/openapi-go/openapi3"
)

type readWriteCloser struct {
	io.Reader
	io.Writer
	io.Closer
}

func (rwc readWriteCloser) Close() error {
	return nil
}

type stdioDialer struct {
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

func (d stdioDialer) Dial(ctx context.Context) (io.ReadWriteCloser, error) {
	return readWriteCloser{
		Reader: d.stdout,
		Writer: d.stdin,
		Closer: d.stdin,
	}, nil
}

// TODO(shawn-hurley): Pipe the logger through
// Determine how and where external providers will add the logs to make the logs viewable in a single location.
type yqProvider struct {
	ctx context.Context

	log logr.Logger
}

var _ provider.BaseClient = &yqProvider{}

func NewYqProvider() *yqProvider {
	return &yqProvider{}
}

func (p *yqProvider) Capabilities() []provider.Capability {
	caps := []provider.Capability{}
	r := openapi3.NewReflector()
	k8sResourceMatched, err := provider.ToProviderCap(r, p.log, k8sResourceCondition{}, "k8sResourceMatched")
	if err != nil {
		p.log.Error(err, "error generating capabilities")
		return caps
	}
	caps = append(caps, k8sResourceMatched)
	return caps
}

type yqCondition struct {
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

func (p *yqProvider) Init(ctx context.Context, log logr.Logger, c provider.InitConfig) (provider.ServiceClient, provider.InitConfig, error) {
	if c.AnalysisMode != provider.FullAnalysisMode {
		return nil, provider.InitConfig{}, fmt.Errorf("only full analysis is supported")
	}

	if c.Proxy != nil {
		// handle proxy settings
		for k, v := range c.Proxy.ToEnvVars() {
			os.Setenv(k, v)
		}
	}

	lspServerPath, ok := c.ProviderSpecificConfig[provider.LspServerPathConfigKey].(string)
	if !ok || lspServerPath == "" {
		return nil, provider.InitConfig{}, fmt.Errorf("invalid lspServerPath provided, unable to init yq provider")
	}

	ctx, cancelFunc := context.WithCancel(ctx)
	log = log.WithValues("provider", c.ProviderSpecificConfig["name"])
	var args []string
	if lspArgs, ok := c.ProviderSpecificConfig["lspArgs"]; ok {
		rawArgs, isArray := lspArgs.([]interface{})
		if !isArray {
			cancelFunc()
			return nil, provider.InitConfig{}, fmt.Errorf("lspArgs is not an array")
		}
		for _, rawArg := range rawArgs {
			if arg, ok := rawArg.(string); ok {
				args = append(args, arg)
			} else {
				cancelFunc()
				return nil, provider.InitConfig{}, fmt.Errorf("item of lspArgs is not a string")
			}
		}
	}
	cmd := exec.CommandContext(ctx, lspServerPath, args...)

	go func() {
		err := cmd.Run()
		if err != nil {
			log.Error(err, "failed to start LSP server")
			// TODO: Probably should cancel the ctx here, to shut everything down
			return
		}
	}()

	svcClient := yqServiceClient{
		cancelFunc: cancelFunc,
		log:        log,
		cmd:        cmd,
		config:     c,
	}

	return &svcClient, provider.InitConfig{}, nil
}

func (p *yqProvider) NotifyFileChanges(ctx context.Context, changes ...provider.FileChange) error {
	return nil
}
