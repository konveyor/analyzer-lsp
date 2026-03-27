package core

import (
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
			if err != nil {
				continue
			}
			stat, err := os.Stat(location)
			if err != nil || !stat.IsDir() {
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

	// Always append the builtin config so the builtin provider is created
	// and available during rule parsing. Without it, rules using builtin
	// capabilities (filecontent, file, xml, hasTags) are skipped as
	// "unavailable provider". InitConfigs may be empty here (e.g., for
	// binary analysis where locations are Maven URIs or files); the builtin
	// will receive its actual configs from providers via
	// additionalBuiltinConfigs during ProviderStart.
	finalConfigs = append(finalConfigs, provider.Config{
		Name:       "builtin",
		InitConfig: slices.Collect(maps.Values(defaultBuiltinConfigs)),
	})

	return finalConfigs, slices.Collect(maps.Keys(defaultBuiltinConfigs))

}
