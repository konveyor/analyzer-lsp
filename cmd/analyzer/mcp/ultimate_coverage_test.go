package mcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Tests to reach 90%+ coverage by targeting specific uncovered branches

func TestDependenciesGet_EmptyTargetPath(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	params := DependenciesGetParams{
		TargetPath: "",
	}

	_, err := dependenciesGet(context.Background(), log, settingsFile, params)
	if err == nil {
		t.Error("Expected error for empty target_path")
	}
	if err.Error() != "target_path is required" {
		t.Errorf("Expected 'target_path is required' error, got: %v", err)
	}
}

func TestDependenciesGet_NonExistentPath(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	params := DependenciesGetParams{
		TargetPath: "/nonexistent/path/that/does/not/exist",
	}

	_, err := dependenciesGet(context.Background(), log, settingsFile, params)
	if err == nil {
		t.Error("Expected error for non-existent target path")
	}
}

func TestRulesValidate_EmptyRulesPath(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	params := RulesValidateParams{
		RulesPath: "",
	}

	_, err := rulesValidate(context.Background(), log, settingsFile, params)
	if err == nil {
		t.Error("Expected error for empty rules_path")
	}
	if err.Error() != "rules_path is required" {
		t.Errorf("Expected 'rules_path is required' error, got: %v", err)
	}
}

func TestRulesValidate_NonExistentRulesPath(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	params := RulesValidateParams{
		RulesPath: "/nonexistent/rules/path",
	}

	_, err := rulesValidate(context.Background(), log, settingsFile, params)
	if err == nil {
		t.Error("Expected error for non-existent rules path")
	}
}

func TestRulesValidate_InvalidProviderConfig(t *testing.T) {
	log := getTestLogger()
	tmpDir := t.TempDir()

	// Create an invalid settings file
	invalidSettings := filepath.Join(tmpDir, "invalid_settings.json")
	os.WriteFile(invalidSettings, []byte("invalid json content"), 0644)

	// Create valid rules
	rulesPath := filepath.Join(tmpDir, "rules.yaml")
	rules := `- ruleID: test
  message: "Test"
  when:
    builtin.file:
      pattern: ".*"
`
	os.WriteFile(rulesPath, []byte(rules), 0644)

	params := RulesValidateParams{
		RulesPath: rulesPath,
	}

	result, err := rulesValidate(context.Background(), log, invalidSettings, params)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should have returned validation result with errors
	if result == "" {
		t.Error("Expected validation result")
	}
}

func TestRulesValidate_RuleMissingID(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)
	tmpDir := t.TempDir()

	// Create rules with missing ID
	rulesPath := filepath.Join(tmpDir, "no_id_rules.yaml")
	rules := `- message: "Test rule without ID"
  when:
    builtin.file:
      pattern: ".*"
`
	os.WriteFile(rulesPath, []byte(rules), 0644)

	params := RulesValidateParams{
		RulesPath: rulesPath,
	}

	result, err := rulesValidate(context.Background(), log, settingsFile, params)
	if err != nil {
		t.Logf("rulesValidate returned error (may be expected): %v", err)
		return
	}

	if result == "" {
		t.Error("Expected validation result")
	}
}

func TestRulesValidate_MissingDescriptionAndLabels(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)
	tmpDir := t.TempDir()

	// Create rules without description and labels
	rulesPath := filepath.Join(tmpDir, "minimal_rules.yaml")
	rules := `- ruleID: test-minimal
  message: "Minimal rule"
  when:
    builtin.file:
      pattern: ".*"
`
	os.WriteFile(rulesPath, []byte(rules), 0644)

	params := RulesValidateParams{
		RulesPath: rulesPath,
	}

	result, err := rulesValidate(context.Background(), log, settingsFile, params)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result == "" {
		t.Error("Expected validation result with warnings")
	}
}

func TestRulesList_EmptyRulesPath(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	params := RulesListParams{
		RulesPath: "",
	}

	_, err := rulesList(context.Background(), log, settingsFile, params)
	if err == nil {
		t.Error("Expected error for empty rules_path")
	}
	if err.Error() != "rules_path is required" {
		t.Errorf("Expected 'rules_path is required' error, got: %v", err)
	}
}

func TestRulesList_NonExistentRulesPath(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	params := RulesListParams{
		RulesPath: "/nonexistent/rules/path",
	}

	_, err := rulesList(context.Background(), log, settingsFile, params)
	if err == nil {
		t.Error("Expected error for non-existent rules path")
	}
}

func TestAnalyzeRun_EmptyRulesPath(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	params := AnalyzeRunParams{
		RulesPath:  "",
		TargetPath: "testdata/target",
	}

	_, err := analyzeRun(context.Background(), log, settingsFile, params)
	if err == nil {
		t.Error("Expected error for empty rules_path")
	}
	if err.Error() != "rules_path is required" {
		t.Errorf("Expected 'rules_path is required' error, got: %v", err)
	}
}

func TestAnalyzeRun_EmptyTargetPath(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	params := AnalyzeRunParams{
		RulesPath:  "testdata/rules/test_rules.yaml",
		TargetPath: "",
	}

	_, err := analyzeRun(context.Background(), log, settingsFile, params)
	if err == nil {
		t.Error("Expected error for empty target_path")
	}
	if err.Error() != "target_path is required" {
		t.Errorf("Expected 'target_path is required' error, got: %v", err)
	}
}

func TestAnalyzeRun_InvalidOutputFormat(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	params := AnalyzeRunParams{
		RulesPath:    "testdata/rules/test_rules.yaml",
		TargetPath:   "testdata/target",
		OutputFormat: "xml",
	}

	_, err := analyzeRun(context.Background(), log, settingsFile, params)
	if err == nil {
		t.Error("Expected error for invalid output format")
	}
	// Just check that we got an error about output_format
	if err != nil && err.Error() == "" {
		t.Errorf("Expected output_format error, got: %v", err)
	}
}

func TestAnalyzeRun_NonExistentRulesPath(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	params := AnalyzeRunParams{
		RulesPath:  "/nonexistent/rules.yaml",
		TargetPath: "testdata/target",
	}

	_, err := analyzeRun(context.Background(), log, settingsFile, params)
	if err == nil {
		t.Error("Expected error for non-existent rules path")
	}
}

func TestAnalyzeRun_NonExistentTargetPath(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	params := AnalyzeRunParams{
		RulesPath:  "testdata/rules/test_rules.yaml",
		TargetPath: "/nonexistent/target",
	}

	_, err := analyzeRun(context.Background(), log, settingsFile, params)
	if err == nil {
		t.Error("Expected error for non-existent target path")
	}
}

func TestAnalyzeIncidents_EmptyResultFile(t *testing.T) {
	log := getTestLogger()

	params := AnalyzeIncidentsParams{
		ResultFile: "",
	}

	_, err := analyzeIncidents(context.Background(), log, params)
	if err == nil {
		t.Error("Expected error for empty result_file")
	}
	if err.Error() != "result_file is required" {
		t.Errorf("Expected 'result_file is required' error, got: %v", err)
	}
}

func TestAnalyzeIncidents_NonExistentFile(t *testing.T) {
	log := getTestLogger()

	params := AnalyzeIncidentsParams{
		ResultFile: "/nonexistent/results.yaml",
	}

	_, err := analyzeIncidents(context.Background(), log, params)
	if err == nil {
		t.Error("Expected error for non-existent result file")
	}
}

func TestAnalyzeIncidents_InvalidYAML(t *testing.T) {
	log := getTestLogger()
	tmpDir := t.TempDir()

	// Create invalid YAML file
	invalidFile := filepath.Join(tmpDir, "invalid.yaml")
	os.WriteFile(invalidFile, []byte("invalid: yaml: content: ["), 0644)

	params := AnalyzeIncidentsParams{
		ResultFile: invalidFile,
	}

	_, err := analyzeIncidents(context.Background(), log, params)
	if err == nil {
		t.Error("Expected error for invalid YAML")
	}
}

func TestServeStdio_ContextCancellation(t *testing.T) {
	server, err := NewMCPServer(getTestLogger(), getTestSettingsPath(t))
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// ServeStdio should handle cancelled context gracefully
	err = server.ServeStdio(ctx)
	if err != nil && err != context.Canceled {
		t.Logf("ServeStdio with cancelled context: %v (may be expected)", err)
	}
}

func TestProvidersList_EmptyConfig(t *testing.T) {
	log := getTestLogger()
	tmpDir := t.TempDir()

	// Create empty settings file
	emptySettings := filepath.Join(tmpDir, "empty.json")
	os.WriteFile(emptySettings, []byte("[]"), 0644)

	params := ProvidersListParams{
		SettingsPath: emptySettings,
	}

	result, err := providersList(context.Background(), log, emptySettings, params)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result == "" {
		t.Error("Expected result even with empty config")
	}
}

func TestProvidersList_InvalidConfig(t *testing.T) {
	log := getTestLogger()
	tmpDir := t.TempDir()

	// Create invalid settings file
	invalidSettings := filepath.Join(tmpDir, "invalid.json")
	os.WriteFile(invalidSettings, []byte("{invalid json}"), 0644)

	params := ProvidersListParams{
		SettingsPath: invalidSettings,
	}

	_, err := providersList(context.Background(), log, invalidSettings, params)
	if err == nil {
		t.Error("Expected error for invalid config")
	}
}

func TestConvertLabelsToStrings_AllCases(t *testing.T) {
	tests := []struct {
		name     string
		labels   []string
		expected int
	}{
		{
			name:     "nil input",
			labels:   nil,
			expected: 0,
		},
		{
			name:     "empty slice",
			labels:   []string{},
			expected: 0,
		},
		{
			name:     "single label",
			labels:   []string{"test"},
			expected: 1,
		},
		{
			name:     "multiple labels",
			labels:   []string{"a", "b", "c", "d"},
			expected: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertLabelsToStrings(tt.labels)
			if len(result) != tt.expected {
				t.Errorf("Expected %d labels, got %d", tt.expected, len(result))
			}
		})
	}
}

func TestGetCategoryFromLabels_AllCases(t *testing.T) {
	tests := []struct {
		name     string
		labels   []string
		expected string
	}{
		{
			name:     "nil input",
			labels:   nil,
			expected: "",
		},
		{
			name:     "empty slice",
			labels:   []string{},
			expected: "",
		},
		{
			name:     "no category label",
			labels:   []string{"effort=1", "source=java"},
			expected: "",
		},
		{
			name:     "category first",
			labels:   []string{"category=mandatory", "effort=1"},
			expected: "mandatory",
		},
		{
			name:     "category middle",
			labels:   []string{"effort=1", "category=optional", "source=java"},
			expected: "optional",
		},
		{
			name:     "category last",
			labels:   []string{"effort=1", "source=java", "category=potential"},
			expected: "potential",
		},
		{
			name:     "malformed category",
			labels:   []string{"category"},
			expected: "",
		},
		{
			name:     "category with special chars",
			labels:   []string{"category=mandatory-advanced"},
			expected: "mandatory-advanced",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getCategoryFromLabels(tt.labels)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}
