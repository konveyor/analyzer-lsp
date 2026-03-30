package python_external_provider

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/progress"
	"github.com/konveyor/analyzer-lsp/provider"
)

type pythonProvider struct {
	capabilities []provider.Capability
	progress     *progress.Progress
}

func (p *pythonProvider) SetProgress(progress *progress.Progress) {
	p.progress = progress
}

// NewPythonProvider constructs the gRPC-facing BaseClient for python-external-provider (pylsp).
// lspServerName should match rule provider ids (typically "pylsp" or "generic" depending on config).
func NewPythonProvider(lspServerName string, log logr.Logger, progress *progress.Progress) provider.BaseClient {
	_ = lspServerName
	p := &pythonProvider{
		progress: progress,
	}

	builder := &PythonServiceClientBuilder{}
	for _, cap := range builder.GetPythonServiceClientCapabilities(log) {
		p.capabilities = append(p.capabilities, provider.Capability{
			Name:   cap.Name,
			Input:  cap.Input,
			Output: cap.Output,
		})
	}

	return p
}

func (p *pythonProvider) Capabilities() []provider.Capability {
	return p.capabilities
}

func (p *pythonProvider) Init(ctx context.Context, log logr.Logger, c provider.InitConfig) (provider.ServiceClient, provider.InitConfig, error) {
	if p.progress == nil {
		var err error
		p.progress, err = progress.New()
		if err != nil {
			return nil, provider.InitConfig{}, fmt.Errorf("failed to create progress: %w", err)
		}
	}

	builder := &PythonServiceClientBuilder{Progress: p.progress}
	sc, err := builder.Init(ctx, log, c)
	if err != nil {
		return nil, provider.InitConfig{}, fmt.Errorf("python provider init: %w", err)
	}

	return sc, provider.InitConfig{}, nil
}
