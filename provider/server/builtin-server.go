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

func NewBuiltinProviderServer(log logr.Logger) (libgrpc.ProviderServiceServer, error) {
	builtin := builtin.NewBuiltinProvider(provider.Config{}, log)
	return &builtinServer{
		server: server{
			Client:  builtin,
			mutex:   sync.RWMutex{},
			clients: map[int64]clientMapItem{},
			rand:    rand.Rand{},
		},
	}, nil
}

var _ libgrpc.ProviderServiceServer = &server{}
