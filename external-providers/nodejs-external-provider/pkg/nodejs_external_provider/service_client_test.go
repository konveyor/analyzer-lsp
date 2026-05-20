package nodejs_external_provider_test

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	nodejsext "github.com/konveyor/analyzer-lsp/external-providers/nodejs-external-provider/pkg/nodejs_external_provider"
	"github.com/konveyor/analyzer-lsp/provider"
)

func TestNodejsServiceClientBuilderCapabilities(t *testing.T) {
	builder := &nodejsext.NodeServiceClientBuilder{}
	caps := builder.GetNodeServiceClientCapabilities(logr.Discard())
	if len(caps) == 0 {
		t.Fatalf("expected non-empty capabilities")
	}
}

func TestNodejsProviderInitErrorPath(t *testing.T) {
	p := nodejsext.NewNodejsProvider("nodejs", logr.Discard(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, _, err := p.Init(ctx, logr.Discard(), provider.InitConfig{
		ProviderSpecificConfig: map[string]interface{}{
			"lspServerName": "nodejs",
			"lspServerPath": "/definitely/not/a/real/lsp-server",
			"lspServerArgs": []interface{}{"--stdio"},
		},
	})
	if err == nil {
		t.Fatalf("expected init error with invalid lspServerPath")
	}
}
