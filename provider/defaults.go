package provider

// WithProviderSpecificDefaults returns a copy of c whose ProviderSpecificConfig
// carries every key from defaults that the caller did not set. A key counts as
// not set when it is missing, nil or an empty string, so the empty
// "lspServerInitializationOptions": "" style entries found in existing settings
// files do not hide a default.
//
// Values the caller did set always win. The map on c is never modified, a new
// one is built, so a config shared between several init calls stays untouched.
// A nil ProviderSpecificConfig is valid and is treated like an empty one.
//
// External providers use this at the top of Init to describe the setup their
// own image ships with, which lets callers leave providerSpecificConfig out
// unless they are doing something unusual.
func WithProviderSpecificDefaults(c InitConfig, defaults map[string]interface{}) InitConfig {
	merged := make(map[string]interface{}, len(c.ProviderSpecificConfig)+len(defaults))
	for k, v := range c.ProviderSpecificConfig {
		merged[k] = v
	}
	for k, v := range defaults {
		if isUnset(merged[k]) {
			merged[k] = v
		}
	}
	c.ProviderSpecificConfig = merged
	return c
}

func isUnset(v interface{}) bool {
	if v == nil {
		return true
	}
	s, ok := v.(string)
	return ok && s == ""
}
