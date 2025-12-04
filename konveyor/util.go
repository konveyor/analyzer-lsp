package konveyor

import (
	"maps"
	"slices"

	"github.com/konveyor/analyzer-lsp/provider"
)

func setupProviderConfigs(providerConfigs []provider.Config) ([]provider.Config, []string) {
	finalConfigs := []provider.Config{}
	defaultBuiltinConfigs := map[string]provider.InitConfig{}

	for _, config := range providerConfigs {
		// TODO: make this a constant in the provider package.
		if config.Name != "builtin" {
			finalConfigs = append(finalConfigs, config)
		}
		for _, initConfig := range config.InitConfig {
			if builtinConfig, ok := defaultBuiltinConfigs[initConfig.Location]; ok {
				if config.Name == "builtin" {
					builtinConfig.ProviderSpecificConfig = initConfig.ProviderSpecificConfig
				}
				continue
			}
			builtinInitConfig := provider.InitConfig{Location: initConfig.Location}
			if analysisMode != "" {

			// If the builtin config then make sure to use the initConfig
			if config.Name == "builtin" {
				builtinInitConfig.ProviderSpecificConfig = initConfig.ProviderSpecificConfig
			}
			defaultBuiltinConfigs[initConfig.Location] = builtinInitConfig
		}
	}

	finalConfigs = append(finalConfigs, provider.Config{
		Name:       "builtin",
		InitConfig: slices.Collect(maps.Values(defaultBuiltinConfigs)),
	})

	return finalConfigs, slices.Collect(maps.Keys(defaultBuiltinConfigs))

}
