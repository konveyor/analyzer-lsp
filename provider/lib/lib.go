package lib

import (
	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/progress"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/provider/grpc"
	"github.com/konveyor/analyzer-lsp/provider/internal/builtin"
)

// We need some wrapper that can deal with out of tree providers, this will be a call, that will mock it out, but go against in tree.
func GetProviderClient(config provider.Config, log logr.Logger, progress *progress.Progress) (provider.InternalProviderClient, error) {
	switch config.Name {
	case "builtin":
		return builtin.NewBuiltinProvider(config, log, progress), nil
	default:
		return grpc.NewGRPCClient(config, log, progress)
	}
}
