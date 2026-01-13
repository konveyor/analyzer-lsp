package konveyor

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
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
			location, err := filepath.Abs(initConfig.Location)
			_, err = os.Stat(location)
			if _, statErr := os.Stat(location); errors.Is(statErr, &os.PathError{}) {
				continue
			}
			if err != nil {
				continue
			}
			// If a extra config is sent back but a builtin config is also passed to use
			// Then we should overwrite the config with the builtin one.
			if builtinConfig, ok := defaultBuiltinConfigs[location]; ok {
				if config.Name == "builtin" {
					builtinConfig.ProviderSpecificConfig = initConfig.ProviderSpecificConfig
					defaultBuiltinConfigs[location] = builtinConfig
				}
				continue
			}
			builtinInitConfig := provider.InitConfig{Location: location}
			// If the builtin config then make sure to use the initConfig
			if config.Name == "builtin" {
				builtinInitConfig.ProviderSpecificConfig = initConfig.ProviderSpecificConfig
			}
			defaultBuiltinConfigs[location] = builtinInitConfig
		}

		finalConfigs = append(finalConfigs, provider.Config{
			Name:       "builtin",
			InitConfig: slices.Collect(maps.Values(defaultBuiltinConfigs)),
		})
	}

	return finalConfigs, slices.Collect(maps.Keys(defaultBuiltinConfigs))

}
