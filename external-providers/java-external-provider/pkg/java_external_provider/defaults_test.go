package java

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency/labels"
	"github.com/konveyor/analyzer-lsp/provider"
)

func TestDefaultProviderSpecificConfigIsMergedUnderUserValues(t *testing.T) {
	c := provider.WithProviderSpecificDefaults(provider.InitConfig{
		ProviderSpecificConfig: map[string]interface{}{
			provider.LspServerPathConfigKey: "/opt/jdtls/bin/jdtls",
			"jvmMaxMem":                     "2G",
		},
	}, defaultProviderSpecificConfig())

	psc := c.ProviderSpecificConfig
	if psc[provider.LspServerPathConfigKey] != "/opt/jdtls/bin/jdtls" {
		t.Errorf("explicit lspServerPath was replaced: %v", psc[provider.LspServerPathConfigKey])
	}
	if psc["jvmMaxMem"] != "2G" {
		t.Errorf("unrelated key lost: %v", psc["jvmMaxMem"])
	}
	index := filepath.Join(t.TempDir(), "maven.default.index")
	if err := os.WriteFile(index, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	defer withDefaultIndex(index)()
	psc = provider.WithProviderSpecificDefaults(provider.InitConfig{}, defaultProviderSpecificConfig()).ProviderSpecificConfig
	for _, key := range []string{"lspServerName", BUNDLES_INIT_OPTION, "depOpenSourceLabelsFile"} {
		if v, _ := psc[key].(string); v == "" {
			t.Errorf("%s was not defaulted", key)
		}
	}
}

// Init used to fail straight away with "invalid lspServerPath" when the
// provider specific config was empty. It must now fall back to the jdtls that
// ships in the image and fail later, if at all, for a different reason.
func TestInitDefaultsLspServerPath(t *testing.T) {
	p := NewJavaProvider(logr.Discard(), "java", 0, 0, provider.Config{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// The label index lives at a fixed path in the image, so point it at a
	// scratch file to keep this test about lspServerPath alone.
	index := filepath.Join(t.TempDir(), "index")
	if err := os.WriteFile(index, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "pom.xml"), []byte("<project/>"), 0o600); err != nil {
		t.Fatal(err)
	}

	sc, _, err := p.Init(ctx, logr.Discard(), provider.InitConfig{
		Location:     project,
		AnalysisMode: provider.SourceOnlyAnalysisMode,
		ProviderSpecificConfig: map[string]interface{}{
			"depOpenSourceLabelsFile": index,
		},
	})
	if sc != nil {
		defer sc.Stop()
	}
	if err != nil && strings.Contains(err.Error(), "invalid lspServerPath") {
		t.Fatalf("lspServerPath was not defaulted: %v", err)
	}
}

func withDefaultIndex(path string) func() {
	old := defaultOpenSourceDepIndex
	defaultOpenSourceDepIndex = path
	return func() { defaultOpenSourceDepIndex = old }
}

// A local install has no index at the image path. Leaving the key out must
// keep working, so the default is dropped instead of pointing at a missing file.
func TestMissingDefaultIndexIsSkipped(t *testing.T) {
	defer withDefaultIndex(filepath.Join(t.TempDir(), "absent"))()

	c := provider.WithProviderSpecificDefaults(provider.InitConfig{}, defaultProviderSpecificConfig())
	if _, ok := c.ProviderSpecificConfig["depOpenSourceLabelsFile"]; ok {
		t.Fatalf("missing default index was still applied: %v", c.ProviderSpecificConfig)
	}
	if _, err := labels.GetOpenSourceLabeler(c.ProviderSpecificConfig, logr.Discard()); err != nil {
		t.Fatalf("omitted depOpenSourceLabelsFile should not fail: %v", err)
	}
}

// A path the caller typed is theirs to get right, so it still errors.
func TestExplicitMissingIndexStillFails(t *testing.T) {
	defer withDefaultIndex(filepath.Join(t.TempDir(), "absent"))()

	c := provider.WithProviderSpecificDefaults(provider.InitConfig{
		ProviderSpecificConfig: map[string]interface{}{
			"depOpenSourceLabelsFile": filepath.Join(t.TempDir(), "nope"),
		},
	}, defaultProviderSpecificConfig())
	if _, err := labels.GetOpenSourceLabeler(c.ProviderSpecificConfig, logr.Discard()); err == nil {
		t.Fatal("expected an error for an explicit index that does not exist")
	}
}
