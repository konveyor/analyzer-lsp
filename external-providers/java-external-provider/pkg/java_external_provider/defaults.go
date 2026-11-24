package java

import (
	"os"

	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency/labels"
)

// defaultOpenSourceDepIndex is where the image keeps the maven label index.
// It is a variable so tests can point it somewhere else.
var defaultOpenSourceDepIndex = "/usr/local/etc/maven.default.index"

// defaultProviderSpecificConfig describes the setup the java provider image
// ships with (jdtls, the analyzer bundle and the open source dependency
// index). Anything the caller sets in providerSpecificConfig takes precedence.
//
// The index is only defaulted when the file is there. A local install does not
// have it, and a default pointing at a missing file would make Init fail for
// callers that never asked for open source labels.
func defaultProviderSpecificConfig() map[string]interface{} {
	defaults := map[string]interface{}{
		"lspServerName":     "java",
		"lspServerPath":     "/jdtls/bin/jdtls",
		BUNDLES_INIT_OPTION: "/jdtls/java-analyzer-bundle/java-analyzer-bundle.core/target/java-analyzer-bundle.core-1.0.0-SNAPSHOT.jar",
	}
	if fi, err := os.Stat(defaultOpenSourceDepIndex); err == nil && !fi.IsDir() {
		defaults[labels.ProviderSpecificConfigOpenSourceDepListKey] = defaultOpenSourceDepIndex
	}
	return defaults
}
