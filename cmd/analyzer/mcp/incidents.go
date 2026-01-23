package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"gopkg.in/yaml.v2"
)

// AnalyzeIncidentsParams defines the parameters for the analyze_incidents tool
type AnalyzeIncidentsParams struct {
	ResultFile string `json:"result_file"`
	RuleID     string `json:"rule_id,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

// IncidentInfo represents detailed incident information
type IncidentInfo struct {
	RuleID       string                 `json:"rule_id" yaml:"rule_id"`
	RuleSet      string                 `json:"ruleset" yaml:"ruleset"`
	Violation    konveyor.Violation     `json:"violation" yaml:"violation"`
	Category     string                 `json:"category,omitempty" yaml:"category,omitempty"`
	Effort       int                    `json:"effort,omitempty" yaml:"effort,omitempty"`
	Labels       []string               `json:"labels,omitempty" yaml:"labels,omitempty"`
}

// IncidentsResult represents the results of incident querying
type IncidentsResult struct {
	TotalIncidents int            `json:"total_incidents" yaml:"total_incidents"`
	Incidents      []IncidentInfo `json:"incidents" yaml:"incidents"`
	Filtered       bool           `json:"filtered" yaml:"filtered"`
	FilteredBy     string         `json:"filtered_by,omitempty" yaml:"filtered_by,omitempty"`
}

// analyzeIncidents queries and filters incidents from an analysis result file
func analyzeIncidents(ctx context.Context, log logr.Logger, params AnalyzeIncidentsParams) (string, error) {
	// Validate inputs
	if params.ResultFile == "" {
		return "", fmt.Errorf("result_file is required")
	}

	// Check if result file exists
	if _, err := os.Stat(params.ResultFile); err != nil {
		return "", fmt.Errorf("result file does not exist: %s", params.ResultFile)
	}

	// Read the result file
	content, err := os.ReadFile(params.ResultFile)
	if err != nil {
		return "", fmt.Errorf("failed to read result file: %w", err)
	}

	// Parse the result file (try YAML first, then JSON)
	var ruleSets []konveyor.RuleSet
	err = yaml.Unmarshal(content, &ruleSets)
	if err != nil {
		// Try JSON
		err = json.Unmarshal(content, &ruleSets)
		if err != nil {
			return "", fmt.Errorf("failed to parse result file (tried YAML and JSON): %w", err)
		}
	}

	// Extract incidents
	result := IncidentsResult{
		Incidents:  []IncidentInfo{},
		Filtered:   params.RuleID != "" || params.Limit > 0,
		FilteredBy: "",
	}

	if params.RuleID != "" {
		result.FilteredBy = fmt.Sprintf("rule_id=%s", params.RuleID)
	}
	if params.Limit > 0 {
		if result.FilteredBy != "" {
			result.FilteredBy += fmt.Sprintf(", limit=%d", params.Limit)
		} else {
			result.FilteredBy = fmt.Sprintf("limit=%d", params.Limit)
		}
	}

	incidentCount := 0
	for _, ruleSet := range ruleSets {
		for ruleID, violation := range ruleSet.Violations {
			// Apply rule ID filter if specified
			if params.RuleID != "" && ruleID != params.RuleID {
				continue
			}

			// Count all matching incidents
			result.TotalIncidents += len(violation.Incidents)

			// Add incidents up to limit
			for _, incident := range violation.Incidents {
				// Check if we've hit the limit
				if params.Limit > 0 && incidentCount >= params.Limit {
					goto done
				}

				// Get effort value
				effort := 0
				if violation.Effort != nil {
					effort = *violation.Effort
				}

				incidentInfo := IncidentInfo{
					RuleID:   ruleID,
					RuleSet:  ruleSet.Name,
					Violation: konveyor.Violation{
						Description: violation.Description,
						Category:    violation.Category,
						Labels:      violation.Labels,
						Incidents:   []konveyor.Incident{incident},
						Extras:      violation.Extras,
						Effort:      violation.Effort,
					},
					Category: getCategoryFromViolation(violation),
					Effort:   effort,
					Labels:   violation.Labels,
				}

				result.Incidents = append(result.Incidents, incidentInfo)
				incidentCount++
			}
		}
	}

done:
	// Format output as JSON (better for structured incident data)
	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal incidents: %w", err)
	}

	return string(output), nil
}

// Helper function to extract category from violation
func getCategoryFromViolation(violation konveyor.Violation) string {
	if violation.Category != nil {
		return string(*violation.Category)
	}
	return ""
}
