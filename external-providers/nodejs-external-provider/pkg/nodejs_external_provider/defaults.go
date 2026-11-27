package nodejs_external_provider

// defaultProviderSpecificConfig describes the setup the provider image ships
// with. Anything the caller sets in providerSpecificConfig takes precedence.
func defaultProviderSpecificConfig() map[string]interface{} {
	return map[string]interface{}{
		"lspServerName": "nodejs",
		"lspServerPath": "/usr/local/bin/typescript-language-server",
		"lspServerArgs": []interface{}{"--stdio"},
	}
}
