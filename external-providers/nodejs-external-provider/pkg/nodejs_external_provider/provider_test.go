package nodejs_external_provider_test

import (
	"testing"

	"github.com/go-logr/logr"
	nodejsext "github.com/konveyor/analyzer-lsp/external-providers/nodejs-external-provider/pkg/nodejs_external_provider"
)

func TestNewNodejsProviderCapabilities(t *testing.T) {
	log := logr.Discard()
	p := nodejsext.NewNodejsProvider("nodejs", log, nil)
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
