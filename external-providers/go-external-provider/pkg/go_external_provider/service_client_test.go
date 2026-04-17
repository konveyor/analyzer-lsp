package go_external_provider_test

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	goext "github.com/konveyor/analyzer-lsp/external-providers/go-external-provider/pkg/go_external_provider"
	"github.com/konveyor/analyzer-lsp/provider"
)

func TestGoServiceClientBuilderCapabilities(t *testing.T) {
	builder := &goext.GoServiceClientBuilder{}
	caps := builder.GetGoServiceClientCapabilities(logr.Discard())
	if len(caps) == 0 {
		t.Fatalf("expected non-empty capabilities")
	}
}

func TestGoProviderInitErrorPath(t *testing.T) {
	p := goext.NewGoProvider("go", logr.Discard(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, _, err := p.Init(ctx, logr.Discard(), provider.InitConfig{
		ProviderSpecificConfig: map[string]interface{}{
			"lspServerName": "go",
			"lspServerPath": "/definitely/not/a/real/lsp-server",
			"lspServerArgs": []interface{}{},
		},
	})
	if err == nil {
		t.Fatalf("expected init error with invalid lspServerPath")
	}
}
