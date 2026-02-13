package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/engine/labels"
	"github.com/konveyor/analyzer-lsp/parser"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/provider/lib"
)

// RulesListParams defines the parameters for the rules_list tool
type RulesListParams struct {
	RulesPath   string `json:"rules_path"`
	LabelFilter string `json:"label_filter,omitempty"`
}

// RulesValidateParams defines the parameters for the rules_validate tool
type RulesValidateParams struct {
	RulesPath string `json:"rules_path"`
}

// RuleMetadata represents simplified rule information for listing
type RuleMetadata struct {
	ID          string            `json:"id" yaml:"id"`
	Description string            `json:"description" yaml:"description"`
	Labels      []string          `json:"labels" yaml:"labels"`
	Category    string            `json:"category,omitempty" yaml:"category,omitempty"`
	Effort      int               `json:"effort,omitempty" yaml:"effort,omitempty"`
	Message     string            `json:"message,omitempty" yaml:"message,omitempty"`
	RuleSet     string            `json:"ruleset" yaml:"ruleset"`
}

// ValidationResult represents the result of rules validation
type ValidationResult struct {
	Valid    bool     `json:"valid" yaml:"valid"`
	Errors   []string `json:"errors,omitempty" yaml:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty" yaml:"warnings,omitempty"`
	RulesCount int    `json:"rules_count" yaml:"rules_count"`
}

// rulesList lists all available rules from a rules file or directory
func rulesList(ctx context.Context, log logr.Logger, settingsFile string, params RulesListParams) (string, error) {
	// Validate inputs
	if params.RulesPath == "" {
		return "", fmt.Errorf("rules_path is required")
	}

	// Check if rules path exists
	if _, err := os.Stat(params.RulesPath); err != nil {
		return "", fmt.Errorf("rules path does not exist: %s", params.RulesPath)
	}

	// Get minimal provider configs for parsing
	configs, err := provider.GetConfig(settingsFile)
	if err != nil {
		return "", fmt.Errorf("unable to get provider configuration: %w", err)
	}

	// Create minimal provider clients for rule parsing
	providers := map[string]provider.InternalProviderClient{}
	for _, config := range configs {
		prov, err := lib.GetProviderClient(config, log)
		if err != nil {
			// Continue even if some providers fail
			log.V(5).Info("unable to create provider client", "provider", config.Name, "error", err)
			continue
		}
		providers[config.Name] = prov
	}

	// Cleanup providers on exit
	defer func() {
		for _, prov := range providers {
			if prov != nil {
				prov.Stop()
			}
		}
	}()

	// Parse rules
	ruleParser := parser.RuleParser{
		ProviderNameToClient: providers,
		Log:                  log.WithName("parser"),
		NoDependencyRules:    false,
	}

	ruleSets, _, _, err := ruleParser.LoadRules(params.RulesPath)
	if err != nil {
		return "", fmt.Errorf("unable to parse rules: %w", err)
	}

	// Extract rule metadata
	var allRules []RuleMetadata
	for _, ruleSet := range ruleSets {
		for _, rule := range ruleSet.Rules {
			// Get effort value
			effort := 0
			if rule.Effort != nil {
				effort = *rule.Effort
			}

			// Get message text
			message := ""
			if rule.Perform.Message.Text != nil {
				message = *rule.Perform.Message.Text
			}

			metadata := RuleMetadata{
				ID:          rule.RuleID,
				Description: rule.Description,
				Labels:      convertLabelsToStrings(rule.Labels),
				Category:    getCategoryFromLabels(rule.Labels),
				Effort:      effort,
				Message:     message,
				RuleSet:     ruleSet.Name,
			}

			// Apply label filter if specified
			if params.LabelFilter != "" {
				selector, err := labels.NewLabelSelector[*engine.RuleMeta](params.LabelFilter, nil)
				if err != nil {
					return "", fmt.Errorf("invalid label filter: %w", err)
				}
				matches, err := selector.Matches(&rule.RuleMeta)
				if err != nil {
					return "", fmt.Errorf("error matching label filter: %w", err)
				}
				if !matches {
					continue
				}
			}

			allRules = append(allRules, metadata)
		}
	}

	// Format output as JSON (better for structured data)
	output, err := json.MarshalIndent(allRules, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal results: %w", err)
	}

	return string(output), nil
}

// rulesValidate validates rules syntax and structure
func rulesValidate(ctx context.Context, log logr.Logger, settingsFile string, params RulesValidateParams) (string, error) {
	// Validate inputs
	if params.RulesPath == "" {
		return "", fmt.Errorf("rules_path is required")
	}

	// Check if rules path exists
	if _, err := os.Stat(params.RulesPath); err != nil {
		return "", fmt.Errorf("rules path does not exist: %s", params.RulesPath)
	}

	result := ValidationResult{
		Valid:    true,
		Errors:   []string{},
		Warnings: []string{},
	}

	// Get provider configs
	configs, err := provider.GetConfig(settingsFile)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("unable to get provider configuration: %v", err))
		output, _ := json.MarshalIndent(result, "", "  ")
		return string(output), nil
	}

	// Create provider clients
	providers := map[string]provider.InternalProviderClient{}
	for _, config := range configs {
		prov, err := lib.GetProviderClient(config, log)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("unable to create provider client %s: %v", config.Name, err))
			continue
		}
		providers[config.Name] = prov
	}

	// Cleanup providers on exit
	defer func() {
		for _, prov := range providers {
			if prov != nil {
				prov.Stop()
			}
		}
	}()

	// Parse and validate rules
	ruleParser := parser.RuleParser{
		ProviderNameToClient: providers,
		Log:                  log.WithName("parser"),
		NoDependencyRules:    false,
	}

	ruleSets, _, _, err := ruleParser.LoadRules(params.RulesPath)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("rule parsing failed: %v", err))
	} else {
		// Count valid rules
		for _, ruleSet := range ruleSets {
			result.RulesCount += len(ruleSet.Rules)
		}

		// Perform additional validation checks
		for _, ruleSet := range ruleSets {
			for _, rule := range ruleSet.Rules {
				// Check for required fields
				if rule.RuleID == "" {
					result.Valid = false
					result.Errors = append(result.Errors, "rule missing ID")
				}

				// Check for recommended fields
				if rule.Description == "" {
					result.Warnings = append(result.Warnings, fmt.Sprintf("rule %s missing description", rule.RuleID))
				}
				if len(rule.Labels) == 0 {
					result.Warnings = append(result.Warnings, fmt.Sprintf("rule %s has no labels", rule.RuleID))
				}
			}
		}
	}

	// Format output
	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal validation results: %w", err)
	}

	return string(output), nil
}

// Helper functions

func convertLabelsToStrings(labels []string) []string {
	if labels == nil {
		return []string{}
	}
	return labels
}

func getCategoryFromLabels(labels []string) string {
	// Try to extract category from labels
	for _, label := range labels {
		if len(label) > 9 && label[:9] == "category=" {
			return label[9:]
		}
	}
	return ""
}
