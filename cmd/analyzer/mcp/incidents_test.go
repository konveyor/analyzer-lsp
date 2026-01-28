package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
)

func TestAnalyzeIncidents_Validation(t *testing.T) {
	log := getTestLogger()

	tests := []struct {
		name    string
		params  AnalyzeIncidentsParams
		wantErr bool
		errMsg  string
	}{
		{
			name:    "missing result_file",
			params:  AnalyzeIncidentsParams{},
			wantErr: true,
			errMsg:  "result_file is required",
		},
		{
			name: "non-existent result file",
			params: AnalyzeIncidentsParams{
				ResultFile: "nonexistent.yaml",
			},
			wantErr: true,
			errMsg:  "result file does not exist",
		},
		{
			name: "valid result file",
			params: AnalyzeIncidentsParams{
				ResultFile: "testdata/results/sample_output.yaml",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := analyzeIncidents(context.Background(), log, tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("analyzeIncidents() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("analyzeIncidents() error = %v, expected to contain %q", err, tt.errMsg)
				}
			}
			if !tt.wantErr && result == "" {
				t.Error("analyzeIncidents() returned empty result without error")
			}
		})
	}
}

func TestAnalyzeIncidents_OutputFormat(t *testing.T) {
	log := getTestLogger()

	params := AnalyzeIncidentsParams{
		ResultFile: "testdata/results/sample_output.yaml",
	}

	result, err := analyzeIncidents(context.Background(), log, params)
	if err != nil {
		t.Fatalf("analyzeIncidents() unexpected error: %v", err)
	}

	// Verify result is valid JSON
	var incidentsResult IncidentsResult
	err = json.Unmarshal([]byte(result), &incidentsResult)
	if err != nil {
		t.Errorf("analyzeIncidents() result is not valid JSON: %v", err)
	}

	// Verify we got some incidents
	if len(incidentsResult.Incidents) == 0 {
		t.Error("analyzeIncidents() returned no incidents")
	}

	// Verify total incidents count
	if incidentsResult.TotalIncidents == 0 {
		t.Error("analyzeIncidents() reported 0 total incidents")
	}
}

func TestAnalyzeIncidents_WithRuleIDFilter(t *testing.T) {
	log := getTestLogger()

	params := AnalyzeIncidentsParams{
		ResultFile: "testdata/results/sample_output.yaml",
		RuleID:     "test-001",
	}

	result, err := analyzeIncidents(context.Background(), log, params)
	if err != nil {
		t.Fatalf("analyzeIncidents() unexpected error: %v", err)
	}

	var incidentsResult IncidentsResult
	err = json.Unmarshal([]byte(result), &incidentsResult)
	if err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	// Verify filtering was applied
	if !incidentsResult.Filtered {
		t.Error("analyzeIncidents() did not mark result as filtered")
	}
	if !contains(incidentsResult.FilteredBy, "rule_id=test-001") {
		t.Errorf("analyzeIncidents() FilteredBy = %q, expected to contain 'rule_id=test-001'", incidentsResult.FilteredBy)
	}

	// Verify all returned incidents match the filter
	for _, incident := range incidentsResult.Incidents {
		if incident.RuleID != "test-001" {
			t.Errorf("analyzeIncidents() returned incident with RuleID %q, expected 'test-001'", incident.RuleID)
		}
	}
}

func TestAnalyzeIncidents_WithLimit(t *testing.T) {
	log := getTestLogger()

	params := AnalyzeIncidentsParams{
		ResultFile: "testdata/results/sample_output.yaml",
		Limit:      1,
	}

	result, err := analyzeIncidents(context.Background(), log, params)
	if err != nil {
		t.Fatalf("analyzeIncidents() unexpected error: %v", err)
	}

	var incidentsResult IncidentsResult
	err = json.Unmarshal([]byte(result), &incidentsResult)
	if err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	// Verify filtering was applied
	if !incidentsResult.Filtered {
		t.Error("analyzeIncidents() did not mark result as filtered")
	}
	if !contains(incidentsResult.FilteredBy, "limit=1") {
		t.Errorf("analyzeIncidents() FilteredBy = %q, expected to contain 'limit=1'", incidentsResult.FilteredBy)
	}

	// Verify limit was respected
	if len(incidentsResult.Incidents) > 1 {
		t.Errorf("analyzeIncidents() returned %d incidents, expected at most 1", len(incidentsResult.Incidents))
	}
}

func TestAnalyzeIncidents_WithRuleIDAndLimit(t *testing.T) {
	log := getTestLogger()

	params := AnalyzeIncidentsParams{
		ResultFile: "testdata/results/sample_output.yaml",
		RuleID:     "test-001",
		Limit:      1,
	}

	result, err := analyzeIncidents(context.Background(), log, params)
	if err != nil {
		t.Fatalf("analyzeIncidents() unexpected error: %v", err)
	}

	var incidentsResult IncidentsResult
	err = json.Unmarshal([]byte(result), &incidentsResult)
	if err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	// Verify both filters were applied
	if !incidentsResult.Filtered {
		t.Error("analyzeIncidents() did not mark result as filtered")
	}
	if !contains(incidentsResult.FilteredBy, "rule_id=test-001") {
		t.Errorf("analyzeIncidents() FilteredBy missing 'rule_id=test-001': %q", incidentsResult.FilteredBy)
	}
	if !contains(incidentsResult.FilteredBy, "limit=1") {
		t.Errorf("analyzeIncidents() FilteredBy missing 'limit=1': %q", incidentsResult.FilteredBy)
	}
}

func TestAnalyzeIncidents_IncidentStructure(t *testing.T) {
	log := getTestLogger()

	params := AnalyzeIncidentsParams{
		ResultFile: "testdata/results/sample_output.yaml",
	}

	result, err := analyzeIncidents(context.Background(), log, params)
	if err != nil {
		t.Fatalf("analyzeIncidents() unexpected error: %v", err)
	}

	var incidentsResult IncidentsResult
	err = json.Unmarshal([]byte(result), &incidentsResult)
	if err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	// Verify incident structure
	for i, incident := range incidentsResult.Incidents {
		if incident.RuleID == "" {
			t.Errorf("Incident %d has empty RuleID", i)
		}
		if incident.RuleSet == "" {
			t.Errorf("Incident %d has empty RuleSet", i)
		}
		if len(incident.Violation.Incidents) == 0 {
			t.Errorf("Incident %d has no violation incidents", i)
		}
		// Each IncidentInfo should contain exactly one incident from the violation
		if len(incident.Violation.Incidents) != 1 {
			t.Errorf("Incident %d has %d violation incidents, expected 1", i, len(incident.Violation.Incidents))
		}
	}
}

func TestAnalyzeIncidents_JSONResultFile(t *testing.T) {
	log := getTestLogger()

	// Create a temporary JSON result file
	tmpDir := t.TempDir()
	jsonFile := filepath.Join(tmpDir, "result.json")

	// Create sample data
	effort := 1
	category := konveyor.Category("mandatory")
	rulesets := []konveyor.RuleSet{
		{
			Name: "test-ruleset",
			Violations: map[string]konveyor.Violation{
				"test-001": {
					Description: "Test violation",
					Category:    &category,
					Effort:      &effort,
					Incidents: []konveyor.Incident{
						{
							URI:     "file:///test.java",
							Message: "Test message",
						},
					},
				},
			},
		},
	}

	data, err := json.MarshalIndent(rulesets, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal test data: %v", err)
	}

	err = os.WriteFile(jsonFile, data, 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	params := AnalyzeIncidentsParams{
		ResultFile: jsonFile,
	}

	result, err := analyzeIncidents(context.Background(), log, params)
	if err != nil {
		t.Fatalf("analyzeIncidents() unexpected error: %v", err)
	}

	var incidentsResult IncidentsResult
	err = json.Unmarshal([]byte(result), &incidentsResult)
	if err != nil {
		t.Errorf("analyzeIncidents() result is not valid JSON: %v", err)
	}

	if len(incidentsResult.Incidents) == 0 {
		t.Error("analyzeIncidents() returned no incidents for JSON file")
	}
}

func TestAnalyzeIncidents_InvalidResultFile(t *testing.T) {
	log := getTestLogger()

	// Create a temporary invalid file
	tmpDir := t.TempDir()
	invalidFile := filepath.Join(tmpDir, "invalid.txt")
	err := os.WriteFile(invalidFile, []byte("not yaml or json"), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	params := AnalyzeIncidentsParams{
		ResultFile: invalidFile,
	}

	_, err = analyzeIncidents(context.Background(), log, params)
	if err == nil {
		t.Error("analyzeIncidents() should return error for invalid file format")
	}
	if !contains(err.Error(), "failed to parse") {
		t.Errorf("analyzeIncidents() error = %v, expected to contain 'failed to parse'", err)
	}
}

func TestGetCategoryFromViolation(t *testing.T) {
	tests := []struct {
		name      string
		violation konveyor.Violation
		want      string
	}{
		{
			name:      "nil category",
			violation: konveyor.Violation{},
			want:      "",
		},
		{
			name: "with category",
			violation: konveyor.Violation{
				Category: func() *konveyor.Category {
					c := konveyor.Category("mandatory")
					return &c
				}(),
			},
			want: "mandatory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getCategoryFromViolation(tt.violation)
			if result != tt.want {
				t.Errorf("getCategoryFromViolation() = %q, want %q", result, tt.want)
			}
		})
	}
}
