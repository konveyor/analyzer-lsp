package go_external_provider

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/provider"
)

// StubProvider is a Step 1 scaffold: it satisfies provider.BaseClient but Init
// always fails until real gopls wiring lands (implementation plan Step 2).
type StubProvider struct {
	lspServerName string
}

func NewStubProvider(lspServerName string) *StubProvider {
	return &StubProvider{lspServerName: lspServerName}
}

func (s *StubProvider) Capabilities() []provider.Capability {
	return nil
}

func (s *StubProvider) Init(ctx context.Context, log logr.Logger, cfg provider.InitConfig) (provider.ServiceClient, provider.InitConfig, error) {
	_ = ctx
	_ = s.lspServerName
	_ = log
	_ = cfg
	return nil, provider.InitConfig{}, fmt.Errorf("go-external-provider: scaffold only; Init not implemented (see implementation plan Step 2)")
}
