package konveyor

import (
	"github.com/konveyor/analyzer-lsp/provider"
)

// PROVIDERS
// TODO: ADD DOCS
type Provider struct {
	Name     string
	provider provider.InternalProviderClient
}

func (p *Provider) Capabilities() []provider.Capability {
	return p.provider.Capabilities()
}

// TODO: need to figure out a nice encapsulation of this.
// Is one of CodeSnip, Dependency, or Referenced
func (p *Provider) SupportsFeature(feature string) bool {
	return false
}
