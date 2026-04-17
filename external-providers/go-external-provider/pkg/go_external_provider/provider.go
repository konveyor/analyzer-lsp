package go_external_provider

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	base "github.com/konveyor/analyzer-lsp/lsp/base_service_client"
	"github.com/konveyor/analyzer-lsp/progress"
	"github.com/konveyor/analyzer-lsp/provider"
)

// goProvider is a dedicated gopls-backed external provider (logic evolved from the
// pre-split combined LSP provider, without multi-language switching).
type goProvider struct {
	capabilities     []provider.Capability
	lspCapabilities  []base.LSPServiceClientCapability
	progress         *progress.Progress
}

// SetProgress is invoked by provider.NewServer before serving.
func (p *goProvider) SetProgress(progress *progress.Progress) {
	p.progress = progress
}

// NewGoProvider constructs the gRPC-facing BaseClient for go-external-provider.
// lspServerName should match rule provider ids (typically "go" for golang).
// progress may be nil; Init will create one if needed when SetProgress was not used.
func NewGoProvider(lspServerName string, log logr.Logger, progress *progress.Progress) provider.BaseClient {
	_ = lspServerName // matches main --name / rules provider id; reserved for future use
	p := &goProvider{
		progress: progress,
	}

	builder := &GoServiceClientBuilder{}
	p.lspCapabilities = builder.GetGoServiceClientCapabilities(log)
	for _, cap := range p.lspCapabilities {
		p.capabilities = append(p.capabilities, provider.Capability{
			Name:   cap.Name,
			Input:  cap.Input,
			Output: cap.Output,
		})
	}

	return p
}

func (p *goProvider) Capabilities() []provider.Capability {
	return p.capabilities
}

func (p *goProvider) Init(ctx context.Context, log logr.Logger, c provider.InitConfig) (provider.ServiceClient, provider.InitConfig, error) {
	if p.progress == nil {
		var err error
		p.progress, err = progress.New()
		if err != nil {
			return nil, provider.InitConfig{}, fmt.Errorf("failed to create progress: %w", err)
		}
	}

	builder := &GoServiceClientBuilder{
		Progress:       p.progress,
		lspCapabilities: p.lspCapabilities,
	}
	sc, err := builder.Init(ctx, log, c)
	if err != nil {
		return nil, provider.InitConfig{}, fmt.Errorf("go provider init: %w", err)
	}

	return sc, provider.InitConfig{}, nil
}
