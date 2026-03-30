package go_external_provider_test

import (
	"testing"

	"github.com/go-logr/logr"
	goext "github.com/konveyor/analyzer-lsp/external-providers/go-external-provider/pkg/go_external_provider"
)

func TestNewGoProviderCapabilities(t *testing.T) {
	log := logr.Discard()
	p := goext.NewGoProvider("generic", log, nil)
	caps := p.Capabilities()
	if len(caps) == 0 {
		t.Fatalf("expected non-empty capabilities, got %d", len(caps))
	}
	names := make(map[string]struct{})
	for _, c := range caps {
		names[c.Name] = struct{}{}
	}
	for _, need := range []string{"referenced", "dependency", "echo"} {
		if _, ok := names[need]; !ok {
			t.Errorf("missing capability %q", need)
		}
	}
}
