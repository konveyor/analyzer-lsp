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
	l := log.WithValues("builtin", "builtin")
	builtin := builtin.NewBuiltinProvider(provider.Config{}, l)
	return &builtinServer{
		server: server{
			Client:  builtin,
			mutex:   sync.RWMutex{},
			clients: map[int64]clientMapItem{},
			rand:    rand.Rand{},
			Log:     l,
		},
	}, nil
}

var _ libgrpc.ProviderServiceServer = &server{}
