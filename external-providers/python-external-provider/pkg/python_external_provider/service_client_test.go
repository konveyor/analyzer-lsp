package python_external_provider_test

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	pythonext "github.com/konveyor/analyzer-lsp/external-providers/python-external-provider/pkg/python_external_provider"
	"github.com/konveyor/analyzer-lsp/provider"
)

func TestPythonServiceClientBuilderCapabilities(t *testing.T) {
	builder := &pythonext.PythonServiceClientBuilder{}
	caps := builder.GetPythonServiceClientCapabilities(logr.Discard())
	if len(caps) == 0 {
		t.Fatalf("expected non-empty capabilities")
	}
}

func TestPythonProviderInitErrorPath(t *testing.T) {
	p := pythonext.NewPythonProvider("pylsp", logr.Discard(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, _, err := p.Init(ctx, logr.Discard(), provider.InitConfig{
		ProviderSpecificConfig: map[string]interface{}{
			"lspServerName": "pylsp",
			"lspServerPath": "/definitely/not/a/real/lsp-server",
			"lspServerArgs": []interface{}{},
		},
	})
	if err == nil {
		t.Fatalf("expected init error with invalid lspServerPath")
	}
}
