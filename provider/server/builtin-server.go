package server

import (
	"math/rand"
	"sync"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/provider/internal/builtin"
	libgrpc "github.com/konveyor/analyzer-lsp/provider/internal/grpc"
)

type builtinServer struct {
	server
}

type BuiltinGRPCServer interface {
	libgrpc.ProviderCodeLocationServiceServer
	libgrpc.ProviderServiceServer
}

func NewBuiltinProviderServer(log logr.Logger, contextLines int) (BuiltinGRPCServer, error) {
	l := log.WithValues("builtin", "builtin")
	builtin := builtin.NewBuiltinProvider(provider.Config{
		ContextLines: contextLines,
	}, l)
	return &builtinServer{
		server: server{
			Client:            builtin,
			mutex:             sync.RWMutex{},
			clients:           map[int64]clientMapItem{},
			rand:              rand.Rand{},
			Log:               l,
			CodeSnipeResolver: builtin,
		},
	}, nil
}

var _ libgrpc.ProviderServiceServer = &builtinServer{}
var _ libgrpc.ProviderCodeLocationServiceServer = &builtinServer{}
