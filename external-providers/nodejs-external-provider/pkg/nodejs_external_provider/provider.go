package nodejs_external_provider

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/progress"
	"github.com/konveyor/analyzer-lsp/provider"
)

type nodejsProvider struct {
	capabilities []provider.Capability
	progress     *progress.Progress
}

func (p *nodejsProvider) SetProgress(progress *progress.Progress) {
	p.progress = progress
}

// NewNodejsProvider constructs the gRPC-facing BaseClient for nodejs-external-provider.
// lspServerName should match rule provider ids (typically "nodejs").
func NewNodejsProvider(lspServerName string, log logr.Logger, progress *progress.Progress) provider.BaseClient {
	_ = lspServerName
	p := &nodejsProvider{
		progress: progress,
	}

	builder := &NodeServiceClientBuilder{}
	for _, cap := range builder.GetNodeServiceClientCapabilities(log) {
		p.capabilities = append(p.capabilities, provider.Capability{
			Name:   cap.Name,
			Input:  cap.Input,
			Output: cap.Output,
		})
	}

	return p
}

func (p *nodejsProvider) Capabilities() []provider.Capability {
	return p.capabilities
}

func (p *nodejsProvider) Init(ctx context.Context, log logr.Logger, c provider.InitConfig) (provider.ServiceClient, provider.InitConfig, error) {
	if p.progress == nil {
		var err error
		p.progress, err = progress.New()
		if err != nil {
			return nil, provider.InitConfig{}, fmt.Errorf("failed to create progress: %w", err)
		}
	}

	builder := &NodeServiceClientBuilder{Progress: p.progress}
	sc, err := builder.Init(ctx, log, c)
	if err != nil {
		return nil, provider.InitConfig{}, fmt.Errorf("nodejs provider init: %w", err)
	}

	return sc, provider.InitConfig{}, nil
}
