package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/provider/lib"
)

// ProvidersListParams defines the parameters for the providers_list tool
type ProvidersListParams struct {
	SettingsPath string `json:"settings_path,omitempty"`
}

// ProviderInfo represents information about a provider
type ProviderInfo struct {
	Name         string              `json:"name"`
	Capabilities []CapabilityInfo    `json:"capabilities"`
	Locations    []string            `json:"locations"`
	Config       *provider.InitConfig `json:"config,omitempty"`
}

// CapabilityInfo represents a provider capability
type CapabilityInfo struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	HasInput    bool   `json:"has_input"`
	HasOutput   bool   `json:"has_output"`
}

// providersList lists all available analysis providers and their capabilities
func providersList(ctx context.Context, log logr.Logger, settingsFile string, params ProvidersListParams) (string, error) {
	// Use default settings file if not provided
	if params.SettingsPath == "" {
		params.SettingsPath = settingsFile
	}

	// Check if settings file exists
	if _, err := os.Stat(params.SettingsPath); err != nil {
		return "", fmt.Errorf("provider settings file not found: %s", params.SettingsPath)
	}

	// Get provider configs
	configs, err := provider.GetConfig(params.SettingsPath)
	if err != nil {
		return "", fmt.Errorf("unable to get provider configuration: %w", err)
	}

	// Collect provider information
	var providersInfo []ProviderInfo

	for _, config := range configs {
		// Extract locations from init configs
		locations := []string{}
		for _, initConf := range config.InitConfig {
			if initConf.Location != "" {
				locations = append(locations, initConf.Location)
			}
		}

		// Create provider client to get capabilities
		prov, err := lib.GetProviderClient(config, log)
		if err != nil {
			log.V(5).Info("unable to create provider client", "provider", config.Name, "error", err)
			// Still add provider info even if we can't create the client
			providersInfo = append(providersInfo, ProviderInfo{
				Name:         config.Name,
				Capabilities: []CapabilityInfo{},
				Locations:    locations,
			})
			continue
		}

		// Get capabilities
		caps := prov.Capabilities()
		capabilityInfos := make([]CapabilityInfo, len(caps))
		for i, cap := range caps {
			capabilityInfos[i] = CapabilityInfo{
				Name:        cap.Name,
				HasInput:    cap.Input.Schema != nil,
				HasOutput:   cap.Output.Schema != nil,
			}
		}

		// Add provider info
		providerInfo := ProviderInfo{
			Name:         config.Name,
			Capabilities: capabilityInfos,
			Locations:    locations,
		}

		// Add config details if available
		if len(config.InitConfig) > 0 {
			providerInfo.Config = &config.InitConfig[0]
		}

		providersInfo = append(providersInfo, providerInfo)

		// Stop the provider
		prov.Stop()
	}

	// Format output as JSON
	output, err := json.MarshalIndent(providersInfo, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal provider information: %w", err)
	}

	return string(output), nil
}
