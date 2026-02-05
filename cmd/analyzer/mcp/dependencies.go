package mcp

import (
	"context"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/cmd/analyzer/lib"
	"gopkg.in/yaml.v2"
)

// DependenciesGetParams defines the parameters for the dependencies_get tool
type DependenciesGetParams struct {
	TargetPath       string `json:"target_path"`
	ProviderSettings string `json:"provider_settings,omitempty"`
	TreeFormat       bool   `json:"tree_format,omitempty"`
	LabelSelector    string `json:"label_selector,omitempty"`
}

// dependenciesGet retrieves dependencies by calling into the shared dependency library
func dependenciesGet(ctx context.Context, log logr.Logger, settingsFile string, params DependenciesGetParams) (string, error) {
	// Validate inputs
	if params.TargetPath == "" {
		return "", fmt.Errorf("target_path is required")
	}

	// Check if target path exists
	if _, err := os.Stat(params.TargetPath); err != nil {
		return "", fmt.Errorf("target path does not exist: %s", params.TargetPath)
	}

	// Use default settings file if not provided
	if params.ProviderSettings == "" {
		params.ProviderSettings = settingsFile
	}

	// Get dependencies using shared library
	deps, err := lib.GetDependencies(ctx, lib.DependencyConfig{
		ProviderSettings: params.ProviderSettings,
		TreeFormat:       params.TreeFormat,
		LabelSelector:    params.LabelSelector,
	}, log)
	if err != nil {
		return "", fmt.Errorf("failed to get dependencies: %w", err)
	}

	// Format as YAML
	output, err := yaml.Marshal(deps)
	if err != nil {
		return "", fmt.Errorf("failed to marshal dependency data: %w", err)
	}

	return string(output), nil
}
