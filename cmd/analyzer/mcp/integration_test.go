package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v2"
)

// Integration tests that exercise full code paths with realistic data

func TestAnalyzeRun_FullIntegration(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	// Create a temporary directory with test files
	tmpDir := t.TempDir()

	// Create a Java file
	javaFile := filepath.Join(tmpDir, "TestClass.java")
	err := os.WriteFile(javaFile, []byte(`
public class TestClass {
    public static void main(String[] args) {
        System.out.println("Hello World");
    }
}
`), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create an XML file
	xmlFile := filepath.Join(tmpDir, "config.xml")
	err = os.WriteFile(xmlFile, []byte(`<?xml version="1.0"?>
<configuration>
    <setting>value</setting>
</configuration>
`), 0644)
	if err != nil {
		t.Fatalf("Failed to create XML file: %v", err)
	}

	tests := []struct {
		name         string
		params       AnalyzeRunParams
		expectError  bool
		validateFunc func(*testing.T, string)
	}{
		{
			name: "analyze with JSON output",
			params: AnalyzeRunParams{
				RulesPath:    "testdata/rules/test_rules.yaml",
				TargetPath:   tmpDir,
				OutputFormat: "json",
			},
			expectError: false,
			validateFunc: func(t *testing.T, result string) {
				var rulesets interface{}
				if err := json.Unmarshal([]byte(result), &rulesets); err != nil {
					t.Errorf("Result is not valid JSON: %v", err)
				}
			},
		},
		{
			name: "analyze with YAML output",
			params: AnalyzeRunParams{
				RulesPath:    "testdata/rules/test_rules.yaml",
				TargetPath:   tmpDir,
				OutputFormat: "yaml",
			},
			expectError: false,
			validateFunc: func(t *testing.T, result string) {
				var rulesets interface{}
				if err := yaml.Unmarshal([]byte(result), &rulesets); err != nil {
					t.Errorf("Result is not valid YAML: %v", err)
				}
			},
		},
		{
			name: "analyze with label selector",
			params: AnalyzeRunParams{
				RulesPath:     "testdata/rules/test_rules.yaml",
				TargetPath:    tmpDir,
				LabelSelector: "category=mandatory",
				OutputFormat:  "json",
			},
			expectError: false,
			validateFunc: func(t *testing.T, result string) {
				var rulesets interface{}
				if err := json.Unmarshal([]byte(result), &rulesets); err != nil {
					t.Errorf("Result is not valid JSON: %v", err)
				}
			},
		},
		{
			name: "analyze with custom incident limit",
			params: AnalyzeRunParams{
				RulesPath:     "testdata/rules/test_rules.yaml",
				TargetPath:    tmpDir,
				IncidentLimit: 5,
				OutputFormat:  "json",
			},
			expectError: false,
			validateFunc: func(t *testing.T, result string) {
				var rulesets interface{}
				if err := json.Unmarshal([]byte(result), &rulesets); err != nil {
					t.Errorf("Result is not valid JSON: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := analyzeRun(context.Background(), log, settingsFile, tt.params)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
				return
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if !tt.expectError && tt.validateFunc != nil {
				tt.validateFunc(t, result)
			}
		})
	}
}

func TestRulesList_FullIntegration(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	tests := []struct {
		name         string
		params       RulesListParams
		expectError  bool
		validateFunc func(*testing.T, string)
	}{
		{
			name: "list all rules",
			params: RulesListParams{
				RulesPath: "testdata/rules/test_rules.yaml",
			},
			expectError: false,
			validateFunc: func(t *testing.T, result string) {
				var rules []RuleMetadata
				if err := json.Unmarshal([]byte(result), &rules); err != nil {
					t.Fatalf("Failed to parse result: %v", err)
				}
				if len(rules) < 2 {
					t.Errorf("Expected at least 2 rules, got %d", len(rules))
				}
				// Verify both test rules are present
				foundTest001 := false
				foundTest002 := false
				for _, rule := range rules {
					if rule.ID == "test-001" {
						foundTest001 = true
					}
					if rule.ID == "test-002" {
						foundTest002 = true
					}
				}
				if !foundTest001 {
					t.Error("Rule test-001 not found")
				}
				if !foundTest002 {
					t.Error("Rule test-002 not found")
				}
			},
		},
		{
			name: "list with label filter",
			params: RulesListParams{
				RulesPath:   "testdata/rules/test_rules.yaml",
				LabelFilter: "category=mandatory",
			},
			expectError: false,
			validateFunc: func(t *testing.T, result string) {
				var rules []RuleMetadata
				if err := json.Unmarshal([]byte(result), &rules); err != nil {
					t.Fatalf("Failed to parse result: %v", err)
				}
				// Should only have mandatory rules
				for _, rule := range rules {
					if rule.Category != "mandatory" {
						t.Errorf("Rule %s has category %s, expected mandatory", rule.ID, rule.Category)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := rulesList(context.Background(), log, settingsFile, tt.params)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
				return
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if !tt.expectError && tt.validateFunc != nil {
				tt.validateFunc(t, result)
			}
		})
	}
}

func TestRulesValidate_FullIntegration(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	tests := []struct {
		name         string
		params       RulesValidateParams
		expectError  bool
		validateFunc func(*testing.T, string)
	}{
		{
			name: "validate valid rules",
			params: RulesValidateParams{
				RulesPath: "testdata/rules/test_rules.yaml",
			},
			expectError: false,
			validateFunc: func(t *testing.T, result string) {
				var validation ValidationResult
				if err := json.Unmarshal([]byte(result), &validation); err != nil {
					t.Fatalf("Failed to parse result: %v", err)
				}
				if !validation.Valid {
					t.Errorf("Validation failed for valid rules: %v", validation.Errors)
				}
				if validation.RulesCount < 2 {
					t.Errorf("Expected at least 2 rules, got %d", validation.RulesCount)
				}
			},
		},
		{
			name: "validate invalid rules",
			params: RulesValidateParams{
				RulesPath: "testdata/rules/invalid_rules.yaml",
			},
			expectError: false,
			validateFunc: func(t *testing.T, result string) {
				var validation ValidationResult
				if err := json.Unmarshal([]byte(result), &validation); err != nil {
					t.Fatalf("Failed to parse result: %v", err)
				}
				if validation.Valid {
					t.Error("Validation succeeded for invalid rules")
				}
				if len(validation.Errors) == 0 {
					t.Error("Expected validation errors for invalid rules")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := rulesValidate(context.Background(), log, settingsFile, tt.params)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
				return
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if !tt.expectError && tt.validateFunc != nil {
				tt.validateFunc(t, result)
			}
		})
	}
}

func TestProvidersList_FullIntegration(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	params := ProvidersListParams{
		SettingsPath: settingsFile,
	}

	result, err := providersList(context.Background(), log, settingsFile, params)
	if err != nil {
		t.Fatalf("providersList() error = %v", err)
	}

	var providers []ProviderInfo
	if err := json.Unmarshal([]byte(result), &providers); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}

	if len(providers) == 0 {
		t.Error("Expected at least one provider")
	}

	// Verify builtin provider exists
	foundBuiltin := false
	for _, provider := range providers {
		if provider.Name == "builtin" {
			foundBuiltin = true
			if provider.Capabilities == nil {
				t.Error("Builtin provider has nil capabilities")
			}
			if len(provider.Capabilities) == 0 {
				t.Error("Builtin provider has no capabilities")
			}
		}
	}

	if !foundBuiltin {
		t.Error("Builtin provider not found")
	}
}

func TestAnalyzeIncidents_FullIntegration(t *testing.T) {
	log := getTestLogger()

	tests := []struct {
		name         string
		params       AnalyzeIncidentsParams
		expectError  bool
		validateFunc func(*testing.T, string)
	}{
		{
			name: "query all incidents",
			params: AnalyzeIncidentsParams{
				ResultFile: "testdata/results/sample_output.yaml",
			},
			expectError: false,
			validateFunc: func(t *testing.T, result string) {
				var incidentsResult IncidentsResult
				if err := json.Unmarshal([]byte(result), &incidentsResult); err != nil {
					t.Fatalf("Failed to parse result: %v", err)
				}
				if incidentsResult.TotalIncidents == 0 {
					t.Error("Expected some incidents")
				}
				if len(incidentsResult.Incidents) == 0 {
					t.Error("Expected incident list to not be empty")
				}
			},
		},
		{
			name: "query with rule filter",
			params: AnalyzeIncidentsParams{
				ResultFile: "testdata/results/sample_output.yaml",
				RuleID:     "test-001",
			},
			expectError: false,
			validateFunc: func(t *testing.T, result string) {
				var incidentsResult IncidentsResult
				if err := json.Unmarshal([]byte(result), &incidentsResult); err != nil {
					t.Fatalf("Failed to parse result: %v", err)
				}
				if !incidentsResult.Filtered {
					t.Error("Result should be marked as filtered")
				}
				for _, incident := range incidentsResult.Incidents {
					if incident.RuleID != "test-001" {
						t.Errorf("Expected only test-001 incidents, got %s", incident.RuleID)
					}
				}
			},
		},
		{
			name: "query with limit",
			params: AnalyzeIncidentsParams{
				ResultFile: "testdata/results/sample_output.yaml",
				Limit:      1,
			},
			expectError: false,
			validateFunc: func(t *testing.T, result string) {
				var incidentsResult IncidentsResult
				if err := json.Unmarshal([]byte(result), &incidentsResult); err != nil {
					t.Fatalf("Failed to parse result: %v", err)
				}
				if len(incidentsResult.Incidents) > 1 {
					t.Errorf("Expected at most 1 incident, got %d", len(incidentsResult.Incidents))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := analyzeIncidents(context.Background(), log, tt.params)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
				return
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if !tt.expectError && tt.validateFunc != nil {
				tt.validateFunc(t, result)
			}
		})
	}
}
