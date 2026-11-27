package yq_provider

// defaultProviderSpecificConfig describes the setup the provider image ships
// with. Anything the caller sets in providerSpecificConfig takes precedence.
func defaultProviderSpecificConfig() map[string]interface{} {
	return map[string]interface{}{
		"name":          "yq",
		"lspServerPath": "/usr/local/bin/yq",
	}
}
