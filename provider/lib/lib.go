package lib

import (
	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/provider/grpc"
	"github.com/konveyor/analyzer-lsp/provider/internal/builtin"
	java "github.com/konveyor/java-external-provider/pkg/java_external_provider"
)

// We need some wrapper that can deal with out of tree providers, this will be a call, that will mock it out, but go against in tree.
func GetProviderClient(config provider.Config, log logr.Logger) (provider.InternalProviderClient, error) {
	switch config.Name {
	case "java":
		return java.NewJavaProvider(config, log), nil
	case "builtin":
		return builtin.NewBuiltinProvider(config, log), nil
	default:
		return grpc.NewGRPCClient(config, log), nil
	}
}
