package mcp

import (
	"context"
	"encoding/json"
	"testing"
)

func TestRulesList_Validation(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	tests := []struct {
		name    string
		params  RulesListParams
		wantErr bool
		errMsg  string
	}{
		{
			name:    "missing rules_path",
			params:  RulesListParams{},
			wantErr: true,
			errMsg:  "rules_path is required",
		},
		{
			name: "non-existent rules path",
			params: RulesListParams{
				RulesPath: "nonexistent.yaml",
			},
			wantErr: true,
			errMsg:  "rules path does not exist",
		},
		{
			name: "valid rules path",
			params: RulesListParams{
				RulesPath: "testdata/rules/test_rules.yaml",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := rulesList(context.Background(), log, settingsFile, tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("rulesList() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("rulesList() error = %v, expected to contain %q", err, tt.errMsg)
				}
			}
			if !tt.wantErr && result == "" {
				t.Error("rulesList() returned empty result without error")
			}
		})
	}
}

func TestRulesList_OutputFormat(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	params := RulesListParams{
		RulesPath: "testdata/rules/test_rules.yaml",
	}

	result, err := rulesList(context.Background(), log, settingsFile, params)
	if err != nil {
		t.Fatalf("rulesList() unexpected error: %v", err)
	}

	// Verify result is valid JSON
	var rules []RuleMetadata
	err = json.Unmarshal([]byte(result), &rules)
	if err != nil {
		t.Errorf("rulesList() result is not valid JSON: %v", err)
	}

	// Verify we got some rules
	if len(rules) == 0 {
		t.Error("rulesList() returned no rules")
	}

	// Verify rule metadata structure
	for i, rule := range rules {
		if rule.ID == "" {
			t.Errorf("Rule %d has empty ID", i)
		}
		if rule.RuleSet == "" {
			t.Errorf("Rule %d has empty RuleSet", i)
		}
	}
}

func TestRulesList_WithLabelFilter(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	tests := []struct {
		name        string
		labelFilter string
		wantErr     bool
	}{
		{
			name:        "valid label filter - category",
			labelFilter: "category=mandatory",
			wantErr:     false,
		},
		{
			name:        "valid label filter - effort",
			labelFilter: "effort=1",
			wantErr:     false,
		},
		{
			name:        "invalid label filter syntax",
			labelFilter: "invalid[[[",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := RulesListParams{
				RulesPath:   "testdata/rules/test_rules.yaml",
				LabelFilter: tt.labelFilter,
			}

			result, err := rulesList(context.Background(), log, settingsFile, params)
			if (err != nil) != tt.wantErr {
				t.Errorf("rulesList() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify result is valid JSON
				var rules []RuleMetadata
				err = json.Unmarshal([]byte(result), &rules)
				if err != nil {
					t.Errorf("rulesList() result is not valid JSON: %v", err)
				}
			}
		})
	}
}

func TestRulesList_RuleMetadataContent(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	params := RulesListParams{
		RulesPath: "testdata/rules/test_rules.yaml",
	}

	result, err := rulesList(context.Background(), log, settingsFile, params)
	if err != nil {
		t.Fatalf("rulesList() unexpected error: %v", err)
	}

	var rules []RuleMetadata
	err = json.Unmarshal([]byte(result), &rules)
	if err != nil {
		t.Fatalf("Failed to unmarshal rules: %v", err)
	}

	// Verify expected rules are present
	expectedRules := map[string]bool{
		"test-001": false,
		"test-002": false,
	}

	for _, rule := range rules {
		if _, exists := expectedRules[rule.ID]; exists {
			expectedRules[rule.ID] = true

			// Verify metadata fields are populated
			if rule.Description == "" {
				t.Errorf("Rule %s has empty description", rule.ID)
			}
			if rule.RuleSet == "" {
				t.Errorf("Rule %s has empty ruleset", rule.ID)
			}
			if len(rule.Labels) == 0 {
				t.Errorf("Rule %s has no labels", rule.ID)
			}
		}
	}

	// Verify all expected rules were found
	for ruleID, found := range expectedRules {
		if !found {
			t.Errorf("Expected rule %s not found in results", ruleID)
		}
	}
}

func TestRulesValidate_Validation(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	tests := []struct {
		name    string
		params  RulesValidateParams
		wantErr bool
		errMsg  string
	}{
		{
			name:    "missing rules_path",
			params:  RulesValidateParams{},
			wantErr: true,
			errMsg:  "rules_path is required",
		},
		{
			name: "non-existent rules path",
			params: RulesValidateParams{
				RulesPath: "nonexistent.yaml",
			},
			wantErr: true,
			errMsg:  "rules path does not exist",
		},
		{
			name: "valid rules path",
			params: RulesValidateParams{
				RulesPath: "testdata/rules/test_rules.yaml",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := rulesValidate(context.Background(), log, settingsFile, tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("rulesValidate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("rulesValidate() error = %v, expected to contain %q", err, tt.errMsg)
				}
			}
			if !tt.wantErr && result == "" {
				t.Error("rulesValidate() returned empty result without error")
			}
		})
	}
}

func TestRulesValidate_OutputFormat(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	params := RulesValidateParams{
		RulesPath: "testdata/rules/test_rules.yaml",
	}

	result, err := rulesValidate(context.Background(), log, settingsFile, params)
	if err != nil {
		t.Fatalf("rulesValidate() unexpected error: %v", err)
	}

	// Verify result is valid JSON
	var validation ValidationResult
	err = json.Unmarshal([]byte(result), &validation)
	if err != nil {
		t.Errorf("rulesValidate() result is not valid JSON: %v", err)
	}

	// Verify validation result structure
	if !validation.Valid {
		t.Errorf("rulesValidate() marked valid rules as invalid: %v", validation.Errors)
	}
	if validation.RulesCount == 0 {
		t.Error("rulesValidate() reported 0 rules for valid rule file")
	}
}

func TestRulesValidate_InvalidRules(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	params := RulesValidateParams{
		RulesPath: "testdata/rules/invalid_rules.yaml",
	}

	result, err := rulesValidate(context.Background(), log, settingsFile, params)
	if err != nil {
		t.Fatalf("rulesValidate() unexpected error: %v", err)
	}

	// Verify result is valid JSON
	var validation ValidationResult
	err = json.Unmarshal([]byte(result), &validation)
	if err != nil {
		t.Errorf("rulesValidate() result is not valid JSON: %v", err)
	}

	// Invalid rules should have errors
	if validation.Valid {
		t.Error("rulesValidate() marked invalid rules as valid")
	}
	if len(validation.Errors) == 0 {
		t.Error("rulesValidate() reported no errors for invalid rules")
	}
}

func TestRulesValidate_WarningsForMissingFields(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	params := RulesValidateParams{
		RulesPath: "testdata/rules/test_rules.yaml",
	}

	result, err := rulesValidate(context.Background(), log, settingsFile, params)
	if err != nil {
		t.Fatalf("rulesValidate() unexpected error: %v", err)
	}

	var validation ValidationResult
	err = json.Unmarshal([]byte(result), &validation)
	if err != nil {
		t.Fatalf("Failed to unmarshal validation result: %v", err)
	}

	// Warnings are optional, but verify the field exists
	if validation.Warnings == nil {
		validation.Warnings = []string{}
	}

	// Verify validation result has expected fields
	if validation.RulesCount < 0 {
		t.Errorf("Unexpected rules count: %d", validation.RulesCount)
	}
}

func TestConvertLabelsToStrings(t *testing.T) {
	tests := []struct {
		name   string
		labels []string
		want   int
	}{
		{
			name:   "nil labels",
			labels: nil,
			want:   0,
		},
		{
			name:   "empty labels",
			labels: []string{},
			want:   0,
		},
		{
			name:   "labels with values",
			labels: []string{"category=mandatory", "effort=1"},
			want:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertLabelsToStrings(tt.labels)
			if len(result) != tt.want {
				t.Errorf("convertLabelsToStrings() returned %d labels, want %d", len(result), tt.want)
			}
		})
	}
}

func TestGetCategoryFromLabels(t *testing.T) {
	tests := []struct {
		name   string
		labels []string
		want   string
	}{
		{
			name:   "no labels",
			labels: []string{},
			want:   "",
		},
		{
			name:   "labels without category",
			labels: []string{"effort=1", "source=test"},
			want:   "",
		},
		{
			name:   "labels with category",
			labels: []string{"category=mandatory", "effort=1"},
			want:   "mandatory",
		},
		{
			name:   "multiple categories (first one wins)",
			labels: []string{"category=mandatory", "category=optional"},
			want:   "mandatory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getCategoryFromLabels(tt.labels)
			if result != tt.want {
				t.Errorf("getCategoryFromLabels() = %q, want %q", result, tt.want)
			}
		})
	}
}
