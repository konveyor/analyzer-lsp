package nodejs_external_provider_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	nodejsext "github.com/konveyor/analyzer-lsp/external-providers/nodejs-external-provider/pkg/nodejs_external_provider"
	"github.com/konveyor/analyzer-lsp/provider"
)

// Init used to refuse any config without lspServerPath. It now falls back to
// the typescript-language-server that ships in the provider image, so a missing or empty
// providerSpecificConfig must get past config validation.
func TestNodejsProviderInitDefaultsLspServerPath(t *testing.T) {
	configs := map[string]map[string]interface{}{
		"nil":     nil,
		"empty":   {},
		"partial": {"lspServerName": "nodejs"},
	}
	for name, psc := range configs {
		psc := psc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p := nodejsext.NewNodejsProvider("nodejs", logr.Discard(), nil)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			sc, _, err := p.Init(ctx, logr.Discard(), provider.InitConfig{ProviderSpecificConfig: psc})
			if sc != nil {
				defer sc.Stop()
			}
			if err != nil && strings.Contains(err.Error(), "must provide lspServerPath") {
				t.Fatalf("lspServerPath was not defaulted: %v", err)
			}
		})
	}
}
