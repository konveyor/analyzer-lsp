package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/cmd/analyzer/lib"
	"gopkg.in/yaml.v2"
)

// AnalyzeRunParams defines the parameters for the analyze_run tool
type AnalyzeRunParams struct {
	RulesPath      string `json:"rules_path"`
	TargetPath     string `json:"target_path"`
	LabelSelector  string `json:"label_selector,omitempty"`
	OutputFormat   string `json:"output_format,omitempty"`
	IncidentLimit  int    `json:"incident_limit,omitempty"`
}

// analyzeRun runs analysis by calling into the shared analysis library
func analyzeRun(ctx context.Context, log logr.Logger, settingsFile string, params AnalyzeRunParams) (string, error) {
	// Validate inputs
	if params.RulesPath == "" {
		return "", fmt.Errorf("rules_path is required")
	}
	if params.TargetPath == "" {
		return "", fmt.Errorf("target_path is required")
	}

	// Check if paths exist
	if _, err := os.Stat(params.RulesPath); err != nil {
		return "", fmt.Errorf("rules path does not exist: %s", params.RulesPath)
	}
	if _, err := os.Stat(params.TargetPath); err != nil {
		return "", fmt.Errorf("target path does not exist: %s", params.TargetPath)
	}

	// Set defaults
	if params.OutputFormat == "" {
		params.OutputFormat = "yaml"
	}
	if params.IncidentLimit == 0 {
		params.IncidentLimit = 1500
	}

	// Validate output format
	if params.OutputFormat != "json" && params.OutputFormat != "yaml" {
		return "", fmt.Errorf("output_format must be 'json' or 'yaml', got: %s", params.OutputFormat)
	}

	// Run analysis using shared library
	rulesets, err := lib.RunAnalysis(ctx, lib.AnalysisConfig{
		ProviderSettings:  settingsFile,
		RulesFiles:        []string{params.RulesPath},
		LabelSelector:     params.LabelSelector,
		IncidentLimit:     params.IncidentLimit,
		CodeSnipLimit:     20,
		ContextLines:      10,
		NoDependencyRules: false,
	}, log)
	if err != nil {
		return "", fmt.Errorf("analysis failed: %w", err)
	}

	// Format output
	var output []byte
	if params.OutputFormat == "json" {
		output, err = json.MarshalIndent(rulesets, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to marshal results as JSON: %w", err)
		}
	} else {
		output, err = yaml.Marshal(rulesets)
		if err != nil {
			return "", fmt.Errorf("failed to marshal results as YAML: %w", err)
		}
	}

	return string(output), nil
}
