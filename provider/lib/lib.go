package lib

import (
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/provider/grpc"
	"github.com/konveyor/analyzer-lsp/provider/internal/builtin"
	"github.com/konveyor/analyzer-lsp/provider/internal/java"
)

// We need some wrapper that can deal with out of tree providers, this will be a call, that will mock it out, but go against in tree.
func GetProviderClient(config provider.Config) (provider.Client, error) {
	switch config.Name {
	case "java":
		return java.NewJavaProvider(config), nil
	case "builtin":
		return builtin.NewBuiltinProvider(config), nil
	default:
		return grpc.NewGRPCClient(config), nil
	}
}
