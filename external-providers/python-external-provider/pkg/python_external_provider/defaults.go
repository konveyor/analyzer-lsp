package python_external_provider

// defaultProviderSpecificConfig describes the setup the provider image ships
// with. Anything the caller sets in providerSpecificConfig takes precedence.
func defaultProviderSpecificConfig() map[string]interface{} {
	return map[string]interface{}{
		"lspServerName": "python",
		"lspServerPath": "/usr/local/bin/pylsp",
	}
}
