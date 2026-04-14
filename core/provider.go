package core

import (
	"context"

	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
)

// Provider wraps an internal provider client and exposes its capabilities.
// It provides a simplified interface to interact with providers, including
// checking capabilities and supported features.
type Provider struct {
	Name     string
	provider provider.InternalProviderClient
}

func (p *Provider) Capabilities() []provider.Capability {
	return p.provider.Capabilities()
}

// NotifyFileChanges forwards file change notifications to the underlying provider.
func (p *Provider) NotifyFileChanges(ctx context.Context, changes ...provider.FileChange) error {
	return p.provider.NotifyFileChanges(ctx, changes...)
}

// GetDependencies returns the flat dependency list from the underlying provider.
func (p *Provider) GetDependencies(ctx context.Context) (map[uri.URI][]*provider.Dep, error) {
	return p.provider.GetDependencies(ctx)
}

// TODO: need to figure out a nice encapsulation of this.
// Is one of CodeSnip, Dependency, or Referenced
func (p *Provider) SupportsFeature(feature string) bool {
	return false
}
