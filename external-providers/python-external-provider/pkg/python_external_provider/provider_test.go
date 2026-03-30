package python_external_provider_test

import (
	"testing"

	"github.com/go-logr/logr"
	pythonext "github.com/konveyor/analyzer-lsp/external-providers/python-external-provider/pkg/python_external_provider"
)

func TestNewPythonProviderCapabilities(t *testing.T) {
	log := logr.Discard()
	p := pythonext.NewPythonProvider("pylsp", log, nil)
	caps := p.Capabilities()
	if len(caps) == 0 {
		t.Fatalf("expected non-empty capabilities, got %d", len(caps))
	}
	names := make(map[string]struct{})
	for _, c := range caps {
		names[c.Name] = struct{}{}
	}
	if _, ok := names["referenced"]; !ok {
		t.Errorf("missing capability %q", "referenced")
	}
}
