package konveyor

import "github.com/konveyor/analyzer-lsp/provider"

func FilterByCapability(capability string) Filter {
	return func(p Provider) bool {
		return provider.HasCapability(p.Capabilities(), capability)
	}
}
