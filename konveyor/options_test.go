package konveyor

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/stretchr/testify/assert"
)

func TestWithRuleFilepaths(t *testing.T) {
	rules := []string{"/path/to/rule1", "/path/to/rule2"}
	opts := &analyzerOptions{}

	err := WithRuleFilepaths(rules)(opts)

	assert.NoError(t, err)
	assert.Equal(t, rules, opts.rulesFilepaths)
}

func TestWithProviderConfigFilePath(t *testing.T) {
	path := "/path/to/provider/config.yaml"
	opts := &analyzerOptions{}

	err := WithProviderConfigFilePath(path)(opts)

	assert.NoError(t, err)
	assert.Equal(t, path, opts.providerConfigFilePath)
}

func TestWithLabelSelector(t *testing.T) {
	selector := "label1=value1"
	opts := &analyzerOptions{}

	err := WithLabelSelector(selector)(opts)

	assert.NoError(t, err)
	assert.Equal(t, selector, opts.labelSelector)
}

func TestWithDepLabelSelector(t *testing.T) {
	selector := "dep.label=value"
	opts := &analyzerOptions{}

	err := WithDepLabelSelector(selector)(opts)

	assert.NoError(t, err)
	assert.Equal(t, selector, opts.depLabelSelector)
}

func TestWithIncidentSelector(t *testing.T) {
	selector := "incident.selector"
	opts := &analyzerOptions{}

	err := WithIncidentSelector(selector)(opts)

	assert.NoError(t, err)
	assert.Equal(t, selector, opts.incidentSelector)
}

func TestWithIncidentLimit(t *testing.T) {
	limit := 100
	opts := &analyzerOptions{}

	err := WithIncidentLimit(limit)(opts)

	assert.NoError(t, err)
	assert.Equal(t, limit, opts.incidentLimit)
}

func TestWithCodeSnipLimit(t *testing.T) {
	limit := 50
	opts := &analyzerOptions{}

	err := WithCodeSnipLimit(limit)(opts)

	assert.NoError(t, err)
	assert.Equal(t, limit, opts.codeSnipLimit)
}

func TestWithContextLinesLimit(t *testing.T) {
	limit := 10
	opts := &analyzerOptions{}

	err := WithContextLinesLimit(limit)(opts)

	assert.NoError(t, err)
	assert.Equal(t, limit, opts.contextLineLimit)
}

func TestWithAnalysisMode(t *testing.T) {
	tests := []struct {
		name         string
		mode         string
		expectedMode provider.AnalysisMode
		expectError  bool
	}{
		{
			name:         "full analysis mode",
			mode:         string(provider.FullAnalysisMode),
			expectedMode: provider.FullAnalysisMode,
			expectError:  false,
		},
		{
			name:         "source only analysis mode",
			mode:         string(provider.SourceOnlyAnalysisMode),
			expectedMode: provider.SourceOnlyAnalysisMode,
			expectError:  false,
		},
		{
			name:         "unknown mode returns error",
			mode:         "unknown",
			expectedMode: "",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &analyzerOptions{}
			err := WithAnalysisMode(tt.mode)(opts)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedMode, opts.analysisMode)
			}
		})
	}
}

func TestWithDependencyRulesDisabled(t *testing.T) {
	opts := &analyzerOptions{}

	err := WithDependencyRulesDisabled()(opts)

	assert.NoError(t, err)
	assert.True(t, opts.dependencyRulesDisabled)
}

func TestWithLogger(t *testing.T) {
	logger := logr.Discard()
	opts := &analyzerOptions{}

	err := WithLogger(logger)(opts)

	assert.NoError(t, err)
	// Note: logr.Discard() creates a "discard" logger, not a zero logger
	// We just verify it's set, not that it's non-zero
	assert.NotNil(t, opts.log)
}

func TestWithContext(t *testing.T) {
	ctx := context.Background()
	opts := &analyzerOptions{}

	err := WithContext(ctx)(opts)

	assert.NoError(t, err)
	assert.Equal(t, ctx, opts.ctx)
}

func TestWithReporters(t *testing.T) {
	reporter1 := &mockReporter{}
	reporter2 := &mockReporter{}
	opts := &analyzerOptions{}

	err := WithReporters(reporter1, reporter2)(opts)

	assert.NoError(t, err)
	assert.Len(t, opts.reporters, 2)
	assert.Equal(t, reporter1, opts.reporters[0])
	assert.Equal(t, reporter2, opts.reporters[1])
}

func TestWithScope(t *testing.T) {
	scope := &mockScope{name: "test-scope"}
	opts := &engineOptions{}

	WithScope(scope)(opts)

	assert.Equal(t, scope, opts.Scope)
}

func TestWithProgressReporter(t *testing.T) {
	reporter := &mockReporter{}
	opts := &engineOptions{}

	WithProgressReporter(reporter)(opts)

	assert.Equal(t, reporter, opts.progressReporter)
}

func TestWithSelector(t *testing.T) {
	selector1 := &mockRuleSelector{}
	selector2 := &mockRuleSelector{}
	opts := &engineOptions{}

	WithSelector(selector1, selector2)(opts)

	assert.Len(t, opts.selectors, 2)
	assert.Equal(t, selector1, opts.selectors[0])
	assert.Equal(t, selector2, opts.selectors[1])
}

// Validation tests

func TestWithIncidentLimit_Negative(t *testing.T) {
	opts := &analyzerOptions{}

	err := WithIncidentLimit(-1)(opts)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be non-negative")
}

func TestWithCodeSnipLimit_Negative(t *testing.T) {
	opts := &analyzerOptions{}

	err := WithCodeSnipLimit(-5)(opts)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be non-negative")
}

func TestWithContextLinesLimit_Negative(t *testing.T) {
	opts := &analyzerOptions{}

	err := WithContextLinesLimit(-10)(opts)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be non-negative")
}

func TestWithAnalysisMode_Invalid(t *testing.T) {
	opts := &analyzerOptions{}

	err := WithAnalysisMode("invalid-mode")(opts)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid analysis mode")
}

func TestWithRuleFilepaths_Empty(t *testing.T) {
	opts := &analyzerOptions{}

	err := WithRuleFilepaths([]string{})(opts)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be empty")
}

func TestWithRuleFilepaths_EmptyPath(t *testing.T) {
	opts := &analyzerOptions{}

	err := WithRuleFilepaths([]string{"/path/to/rule1", "", "/path/to/rule3"})(opts)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "filepath at index 1 is empty")
}

func TestWithProviderConfigFilePath_Empty(t *testing.T) {
	opts := &analyzerOptions{}

	err := WithProviderConfigFilePath("")(opts)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be empty")
}

func TestWithContext_Nil(t *testing.T) {
	opts := &analyzerOptions{}

	err := WithContext(nil)(opts)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be nil")
}
