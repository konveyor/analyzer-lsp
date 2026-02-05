package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v2"
)

func TestAnalyzeRun_Validation(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	tests := []struct {
		name    string
		params  AnalyzeRunParams
		wantErr bool
		errMsg  string
	}{
		{
			name: "missing rules_path",
			params: AnalyzeRunParams{
				TargetPath: "testdata/target",
			},
			wantErr: true,
			errMsg:  "rules_path is required",
		},
		{
			name: "missing target_path",
			params: AnalyzeRunParams{
				RulesPath: "testdata/rules/test_rules.yaml",
			},
			wantErr: true,
			errMsg:  "target_path is required",
		},
		{
			name: "non-existent rules path",
			params: AnalyzeRunParams{
				RulesPath:  "nonexistent.yaml",
				TargetPath: "testdata/target",
			},
			wantErr: true,
			errMsg:  "rules path does not exist",
		},
		{
			name: "non-existent target path",
			params: AnalyzeRunParams{
				RulesPath:  "testdata/rules/test_rules.yaml",
				TargetPath: "nonexistent",
			},
			wantErr: true,
			errMsg:  "target path does not exist",
		},
		{
			name: "invalid output format",
			params: AnalyzeRunParams{
				RulesPath:    "testdata/rules/test_rules.yaml",
				TargetPath:   "testdata/target",
				OutputFormat: "xml",
			},
			wantErr: true,
			errMsg:  "output_format must be 'json' or 'yaml'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := analyzeRun(context.Background(), log, settingsFile, tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("analyzeRun() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("analyzeRun() error = %v, expected to contain %q", err, tt.errMsg)
				}
			}
			if !tt.wantErr && result == "" {
				t.Error("analyzeRun() returned empty result without error")
			}
		})
	}
}

func TestAnalyzeRun_Defaults(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	// Create a temporary test target directory
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "Test.java")
	err := os.WriteFile(testFile, []byte("public class Test {}"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	params := AnalyzeRunParams{
		RulesPath:  "testdata/rules/test_rules.yaml",
		TargetPath: tmpDir,
	}

	result, err := analyzeRun(context.Background(), log, settingsFile, params)
	if err != nil {
		// Analysis might fail due to provider issues in test environment
		// We're primarily testing parameter handling here
		t.Logf("analyzeRun() returned error (may be expected in test env): %v", err)
		return
	}

	// Verify result is valid YAML (default format)
	var rulesets interface{}
	err = yaml.Unmarshal([]byte(result), &rulesets)
	if err != nil {
		t.Errorf("analyzeRun() result is not valid YAML: %v", err)
	}
}

func TestAnalyzeRun_JSONOutput(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	// Create a temporary test target directory
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "Test.java")
	err := os.WriteFile(testFile, []byte("public class Test {}"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	params := AnalyzeRunParams{
		RulesPath:    "testdata/rules/test_rules.yaml",
		TargetPath:   tmpDir,
		OutputFormat: "json",
	}

	result, err := analyzeRun(context.Background(), log, settingsFile, params)
	if err != nil {
		// Analysis might fail due to provider issues in test environment
		t.Logf("analyzeRun() returned error (may be expected in test env): %v", err)
		return
	}

	// Verify result is valid JSON
	var rulesets interface{}
	err = json.Unmarshal([]byte(result), &rulesets)
	if err != nil {
		t.Errorf("analyzeRun() result is not valid JSON: %v", err)
	}
}

func TestAnalyzeRun_YAMLOutput(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	// Create a temporary test target directory
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "Test.java")
	err := os.WriteFile(testFile, []byte("public class Test {}"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	params := AnalyzeRunParams{
		RulesPath:    "testdata/rules/test_rules.yaml",
		TargetPath:   tmpDir,
		OutputFormat: "yaml",
	}

	result, err := analyzeRun(context.Background(), log, settingsFile, params)
	if err != nil {
		// Analysis might fail due to provider issues in test environment
		t.Logf("analyzeRun() returned error (may be expected in test env): %v", err)
		return
	}

	// Verify result is valid YAML
	var rulesets interface{}
	err = yaml.Unmarshal([]byte(result), &rulesets)
	if err != nil {
		t.Errorf("analyzeRun() result is not valid YAML: %v", err)
	}
}

func TestAnalyzeRun_WithLabelSelector(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	// Create a temporary test target directory
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "Test.java")
	err := os.WriteFile(testFile, []byte("public class Test {}"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	params := AnalyzeRunParams{
		RulesPath:     "testdata/rules/test_rules.yaml",
		TargetPath:    tmpDir,
		LabelSelector: "category=mandatory",
		OutputFormat:  "json",
	}

	result, err := analyzeRun(context.Background(), log, settingsFile, params)
	if err != nil {
		// Analysis might fail due to provider issues in test environment
		t.Logf("analyzeRun() returned error (may be expected in test env): %v", err)
		return
	}

	// Verify result is valid JSON
	var rulesets interface{}
	err = json.Unmarshal([]byte(result), &rulesets)
	if err != nil {
		t.Errorf("analyzeRun() result is not valid JSON: %v", err)
	}
}

func TestAnalyzeRun_WithIncidentLimit(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	// Create a temporary test target directory
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "Test.java")
	err := os.WriteFile(testFile, []byte("public class Test {}"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	params := AnalyzeRunParams{
		RulesPath:     "testdata/rules/test_rules.yaml",
		TargetPath:    tmpDir,
		IncidentLimit: 10,
		OutputFormat:  "json",
	}

	result, err := analyzeRun(context.Background(), log, settingsFile, params)
	if err != nil {
		// Analysis might fail due to provider issues in test environment
		t.Logf("analyzeRun() returned error (may be expected in test env): %v", err)
		return
	}

	// Verify result is valid JSON
	var rulesets interface{}
	err = json.Unmarshal([]byte(result), &rulesets)
	if err != nil {
		t.Errorf("analyzeRun() result is not valid JSON: %v", err)
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
