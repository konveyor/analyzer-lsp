package core

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/konveyor/analyzer-lsp/provider"
)

func setupProviderConfigs(
	providerConfigs []provider.Config,
	ignoreAdditionalBuiltinConfigs bool,
	pathMappings []provider.PathMapping,
) ([]provider.Config, []string) {
	finalConfigs := []provider.Config{}
	defaultBuiltinConfigs := map[string]provider.InitConfig{}

	for _, config := range providerConfigs {
		// TODO: make this a constant in the provider package.
		if config.Name != "builtin" {
			finalConfigs = append(finalConfigs, config)
			// When ignoring additional builtin configs, don't auto-generate
			// builtin InitConfigs from non-builtin provider locations.
			if ignoreAdditionalBuiltinConfigs {
				continue
			}
		}
		for _, initConfig := range config.InitConfig {
			location := initConfig.Location
			// Apply path mappings to translate provider paths (e.g., container
			// paths) to engine-local paths before resolving.
			if config.Name != "builtin" {
				location = provider.TranslatePath(location, pathMappings)
			}
			location, err := filepath.Abs(location)
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
	}

	// Append builtin config once after processing all providers
	// Only add builtin if we have any builtin locations
	if len(defaultBuiltinConfigs) > 0 {
		finalConfigs = append(finalConfigs, provider.Config{
			Name:       "builtin",
			InitConfig: slices.Collect(maps.Values(defaultBuiltinConfigs)),
		})
	}

	return finalConfigs, slices.Collect(maps.Keys(defaultBuiltinConfigs))

}
