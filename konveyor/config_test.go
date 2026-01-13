package konveyor

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestAnalyzerConfig_AddFlags(t *testing.T) {
	config := &AnalyzerConfig{}
	cmd := &cobra.Command{Use: "test"}

	config.AddFlags(cmd)

	// Verify all expected flags were added
	assert.NotNil(t, cmd.Flags().Lookup("provider-settings"))
	assert.NotNil(t, cmd.Flags().Lookup("rules"))
	assert.NotNil(t, cmd.Flags().Lookup("label-selector"))
	assert.NotNil(t, cmd.Flags().Lookup("dep-label-selector"))
	assert.NotNil(t, cmd.Flags().Lookup("incident-selector"))
	assert.NotNil(t, cmd.Flags().Lookup("limit-incidents"))
	assert.NotNil(t, cmd.Flags().Lookup("limit-code-snips"))
	assert.NotNil(t, cmd.Flags().Lookup("context-lines"))
	assert.NotNil(t, cmd.Flags().Lookup("analysis-mode"))
	assert.NotNil(t, cmd.Flags().Lookup("no-dependency-rules"))
}

func TestAnalyzerConfig_AddFlags_Binding(t *testing.T) {
	config := &AnalyzerConfig{}
	cmd := &cobra.Command{Use: "test"}

	config.AddFlags(cmd)

	// Verify values were bound to config struct
	assert.Equal(t, "provider_settings.json", config.ProviderSettings)
	assert.Equal(t, []string{"rule-example.yaml"}, config.Rules)
	assert.Equal(t, "", config.LabelSelector)
	assert.Equal(t, 1500, config.IncidentLimit)
	assert.Equal(t, "", config.AnalysisMode)
	assert.False(t, config.DisableDependencyRules)
}

func TestAnalyzerConfig_ToOptions_AllFields(t *testing.T) {
	config := &AnalyzerConfig{
		ProviderSettings:       "provider_settings.json",
		Rules:                  []string{"rules/", "custom-rules/"},
		LabelSelector:          "konveyor.io/target=quarkus",
		DepLabelSelector:       "konveyor.io/dep=critical",
		IncidentSelector:       "(!package=io.konveyor)",
		IncidentLimit:          1500,
		CodeSnipLimit:          20,
		ContextLines:           10,
		AnalysisMode:           "full",
		DisableDependencyRules: true,
	}

	options := config.ToOptions()

	// Should have all options
	assert.Len(t, options, 10)

	// Verify options can be applied to analyzerOptions
	opts := &analyzerOptions{}
	for _, apply := range options {
		err := apply(opts)
		// Note: Some options may fail validation (e.g., if files don't exist)
		// but we're testing that ToOptions() creates the right option functions
		_ = err
	}

	assert.Equal(t, "provider_settings.json", opts.providerConfigFilePath)
	assert.Equal(t, []string{"rules/", "custom-rules/"}, opts.rulesFilepaths)
	assert.Equal(t, "konveyor.io/target=quarkus", opts.labelSelector)
	assert.Equal(t, "konveyor.io/dep=critical", opts.depLabelSelector)
	assert.Equal(t, "(!package=io.konveyor)", opts.incidentSelector)
	assert.Equal(t, 1500, opts.incidentLimit)
	assert.Equal(t, 20, opts.codeSnipLimit)
	assert.Equal(t, 10, opts.contextLineLimit)
	assert.Equal(t, "full", string(opts.analysisMode))
	assert.True(t, opts.dependencyRulesDisabled)
}

func TestAnalyzerConfig_ToOptions_ZeroValues(t *testing.T) {
	config := &AnalyzerConfig{
		ProviderSettings: "settings.json",
		Rules:            []string{"rules/"},
	}

	options := config.ToOptions()

	// Should still create options for zero values (validation happens in option functions)
	assert.GreaterOrEqual(t, len(options), 2)

	opts := &analyzerOptions{}
	for _, apply := range options {
		apply(opts)
	}

	// Zero values are still applied
	assert.Equal(t, 0, opts.incidentLimit)
	assert.Equal(t, 0, opts.codeSnipLimit)
	assert.Equal(t, 0, opts.contextLineLimit)
	assert.False(t, opts.dependencyRulesDisabled)
}

func TestAnalyzerConfig_ToOptions_DisableDependencyRules(t *testing.T) {
	tests := []struct {
		name     string
		disabled bool
		expected bool
	}{
		{
			name:     "dependency rules enabled",
			disabled: false,
			expected: false,
		},
		{
			name:     "dependency rules disabled",
			disabled: true,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &AnalyzerConfig{
				ProviderSettings:       "settings.json",
				Rules:                  []string{"rules/"},
				DisableDependencyRules: tt.disabled,
			}

			options := config.ToOptions()
			opts := &analyzerOptions{}
			for _, apply := range options {
				apply(opts)
			}

			assert.Equal(t, tt.expected, opts.dependencyRulesDisabled)
		})
	}
}

func TestAnalyzerConfig_ToOptions_EmptyConfig(t *testing.T) {
	config := &AnalyzerConfig{}

	options := config.ToOptions()

	// Should still create options even with empty config
	// Validation will happen when passed to NewAnalyzer()
	assert.NotEmpty(t, options)
}
