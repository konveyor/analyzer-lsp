package yq_provider

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/provider"
)

func TestInitDefaultsLspServerPath(t *testing.T) {
	tests := map[string]map[string]interface{}{
		"nil":    nil,
		"empty":  {},
		"blank":  {"lspServerPath": ""},
		"others": {"name": "yaml"},
	}
	for name, psc := range tests {
		t.Run(name, func(t *testing.T) {
			p := NewYqProvider()
			sc, _, err := p.Init(context.Background(), logr.Discard(), provider.InitConfig{
				AnalysisMode:           provider.FullAnalysisMode,
				ProviderSpecificConfig: psc,
			})
			if err != nil {
				t.Fatalf("init should accept a config without lspServerPath: %v", err)
			}
			defer sc.Stop()

			got := sc.(*yqServiceClient).config.ProviderSpecificConfig[provider.LspServerPathConfigKey]
			if got != "/usr/local/bin/yq" {
				t.Errorf("lspServerPath = %v, want the image default", got)
			}
		})
	}
}

func TestInitKeepsExplicitLspServerPath(t *testing.T) {
	p := NewYqProvider()
	sc, _, err := p.Init(context.Background(), logr.Discard(), provider.InitConfig{
		AnalysisMode: provider.FullAnalysisMode,
		ProviderSpecificConfig: map[string]interface{}{
			provider.LspServerPathConfigKey: "/opt/custom/yq",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sc.Stop()

	got := sc.(*yqServiceClient).config.ProviderSpecificConfig[provider.LspServerPathConfigKey]
	if got != "/opt/custom/yq" {
		t.Errorf("explicit lspServerPath was replaced: %v", got)
	}
}
