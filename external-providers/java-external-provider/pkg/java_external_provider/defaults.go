package java

import (
	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency/labels"
)

// defaultProviderSpecificConfig describes the setup the java provider image
// ships with (jdtls, the analyzer bundle and the open source dependency
// index). Anything the caller sets in providerSpecificConfig takes precedence.
func defaultProviderSpecificConfig() map[string]interface{} {
	return map[string]interface{}{
		"lspServerName":     "java",
		"lspServerPath":     "/jdtls/bin/jdtls",
		BUNDLES_INIT_OPTION: "/jdtls/java-analyzer-bundle/java-analyzer-bundle.core/target/java-analyzer-bundle.core-1.0.0-SNAPSHOT.jar",
		labels.ProviderSpecificConfigOpenSourceDepListKey: "/usr/local/etc/maven.default.index",
	}
}
