// Package konveyor provides an analyzer interface for static code analysis.
//
// # Option Validation
//
// This package implements comprehensive validation for all analyzer options.
// Validation errors are returned immediately when invalid options are provided
// to prevent runtime issues.
//
// Options with validation constraints:
//   - WithRuleFilepaths: Validates non-empty array and non-empty paths
//   - WithProviderConfigFilePath: Validates non-empty path
//   - WithIncidentLimit: Validates non-negative values (>= 0)
//   - WithCodeSnipLimit: Validates non-negative values (>= 0)
//   - WithContextLinesLimit: Validates non-negative values (>= 0)
//   - WithAnalysisMode: Validates against known modes ("full" or "source-only")
//   - WithContext: Validates non-nil context
//   - WithLabelSelector: Validates selector syntax during initialization
//
// All validation errors are collected and returned by NewAnalyzer if any
// option fails validation.
package konveyor

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/engine/labels"
	"github.com/konveyor/analyzer-lsp/progress"
	"github.com/konveyor/analyzer-lsp/provider"
)

// ANALYSIS OPTIONS
type analyzerOptions struct {
	rulesFilepaths          []string
	providerConfigFilePath  string
	labelSelector           string
	depLabelSelector        string
	incidentSelector        string
	incidentLimit           int
	codeSnipLimit           int
	contextLineLimit        int
	analysisMode            provider.AnalysisMode
	dependencyRulesDisabled bool
	reporters               []progress.Reporter
	progress                *progress.Progress
	log                     logr.Logger
	ctx                     context.Context
}

type selectorError []error

func (s selectorError) Error() string {
	if len(s) == 0 {
		return ""
	}
	if len(s) == 1 {
		return s[0].Error()
	}
	errorMsg := "multiple selector errors:\n"
	for i, err := range s {
		errorMsg += fmt.Sprintf("  %d: %s\n", i+1, err.Error())
	}
	return errorMsg
}

func (a *analyzerOptions) getSelectors() ([]engine.RuleSelector, error) {
	selectors := []engine.RuleSelector{}
	selectorErrors := selectorError{}
	if a.labelSelector != "" {
		selector, err := labels.NewLabelSelector[*engine.RuleMeta](a.labelSelector, nil)
		if err != nil {
			selectorErrors = append(selectorErrors, err)
		} else {
			selectors = append(selectors, selector)
		}
	}
	if len(selectorErrors) > 0 {
		return nil, selectorErrors
	}
	return selectors, nil
}

// OPTIONALS FOR ENDUSER

// WithRuleFilepaths sets the file paths to rule definitions for the analyzer.
//
// Validation:
//   - The rules slice must not be empty
//   - Each individual filepath must not be an empty string
//
// Returns an error if validation fails.
func WithRuleFilepaths(rules []string) AnalyzerOption {
	return func(options *analyzerOptions) error {
		if len(rules) == 0 {
			return fmt.Errorf("rule filepaths cannot be empty")
		}
		for i, rule := range rules {
			if rule == "" {
				return fmt.Errorf("rule filepath at index %d is empty", i)
			}
		}
		options.rulesFilepaths = rules
		return nil
	}
}

// WithProviderConfigFilePath sets the path to the provider configuration file.
//
// Validation:
//   - The file path must not be an empty string
//
// Returns an error if validation fails.
func WithProviderConfigFilePath(providerConfigFilePath string) AnalyzerOption {
	return func(opt *analyzerOptions) (err error) {
		if providerConfigFilePath == "" {
			return fmt.Errorf("provider config file path cannot be empty")
		}
		opt.providerConfigFilePath = providerConfigFilePath
		return
	}
}

// WithLabelSelector sets the label selector for filtering rules.
// The selector syntax is validated during analyzer initialization.
func WithLabelSelector(labelSelector string) AnalyzerOption {
	return func(opt *analyzerOptions) (err error) {
		opt.labelSelector = labelSelector
		return
	}
}

// WithDepLabelSelector sets the label selector for filtering dependency rules.
func WithDepLabelSelector(selector string) AnalyzerOption {
	return func(opt *analyzerOptions) (err error) {
		opt.depLabelSelector = selector
		return
	}
}

// WithIncidentSelector sets the selector for filtering incidents in the analysis results.
func WithIncidentSelector(selector string) AnalyzerOption {
	return func(opt *analyzerOptions) (err error) {
		opt.incidentSelector = selector
		return
	}
}

// WithIncidentLimit sets the maximum number of incidents to report per rule.
//
// Validation:
//   - The limit must be non-negative (>= 0)
//
// Returns an error if validation fails.
func WithIncidentLimit(limit int) AnalyzerOption {
	return func(opt *analyzerOptions) (err error) {
		if limit < 0 {
			return fmt.Errorf("incident limit must be non-negative, got: %d", limit)
		}
		opt.incidentLimit = limit
		return
	}
}

// WithCodeSnipLimit sets the maximum number of characters to include in code snippets.
//
// Validation:
//   - The limit must be non-negative (>= 0)
//
// Returns an error if validation fails.
func WithCodeSnipLimit(limit int) AnalyzerOption {
	return func(opt *analyzerOptions) (err error) {
		if limit < 0 {
			return fmt.Errorf("code snippet limit must be non-negative, got: %d", limit)
		}
		opt.codeSnipLimit = limit
		return
	}
}

// WithContextLinesLimit sets the number of context lines to include around code snippets.
//
// Validation:
//   - The limit must be non-negative (>= 0)
//
// Returns an error if validation fails.
func WithContextLinesLimit(limit int) AnalyzerOption {
	return func(opt *analyzerOptions) (err error) {
		if limit < 0 {
			return fmt.Errorf("context lines limit must be non-negative, got: %d", limit)
		}
		opt.contextLineLimit = limit
		return
	}
}

// WithAnalysisMode sets the analysis mode for the analyzer.
//
// Validation:
//   - Must be one of: "full" (FullAnalysisMode) or "source-only" (SourceOnlyAnalysisMode)
//   - Empty string is allowed and will use the default mode
//
// Returns an error if an invalid mode is provided.
func WithAnalysisMode(mode string) AnalyzerOption {
	return func(opt *analyzerOptions) (err error) {
		switch mode {
		case string(provider.FullAnalysisMode):
			opt.analysisMode = provider.FullAnalysisMode
		case string(provider.SourceOnlyAnalysisMode):
			opt.analysisMode = provider.SourceOnlyAnalysisMode
		case "":
			return
		default:
			return fmt.Errorf("invalid analysis mode: %s (valid values: %v or %v)", mode, provider.FullAnalysisMode, provider.SourceOnlyAnalysisMode)
		}
		return
	}
}

// WithDependencyRulesDisabled disables dependency analysis rules.
func WithDependencyRulesDisabled() AnalyzerOption {
	return func(opt *analyzerOptions) (err error) {
		opt.dependencyRulesDisabled = true
		return
	}
}

// WithLogger sets a custom logger for the analyzer.
// If not provided, a discard logger will be used.
func WithLogger(log logr.Logger) AnalyzerOption {
	return func(opt *analyzerOptions) (err error) {
		opt.log = log
		return
	}
}

// WithContext sets a custom context for the analyzer.
//
// Validation:
//   - The context must not be nil
//
// Returns an error if validation fails.
// If not provided, context.Background() will be used.
func WithContext(ctx context.Context) AnalyzerOption {
	return func(opt *analyzerOptions) (err error) {
		if ctx == nil {
			return fmt.Errorf("context cannot be nil")
		}
		opt.ctx = ctx
		return
	}
}

// WithProgress sets a custom progress tracker for the analyzer.
// If not provided, a new progress tracker will be created automatically.
func WithProgress(progress *progress.Progress) AnalyzerOption {
	return func(opt *analyzerOptions) (err error) {
		opt.progress = progress
		return
	}
}

// WithReporters sets custom progress reporters for the analyzer.
// Multiple reporters can be provided to receive progress updates.
func WithReporters(reporters ...progress.Reporter) AnalyzerOption {
	return func(options *analyzerOptions) error {
		options.reporters = reporters
		return nil
	}
}

// ENGINE OPTIONS

// WithScope sets the scope for the engine execution.
func WithScope(scope engine.Scope) EngineOption {
	return func(options *engineOptions) {
		options.Scope = scope
	}
}

// WithProgressReporter sets a progress reporter for engine execution.
// This should be a collector.
func WithProgressReporter(reporter progress.Reporter) EngineOption {
	return func(options *engineOptions) {
		options.progressReporter = reporter
	}
}

// WithSelector sets rule selectors for filtering which rules to execute.
// Multiple selectors can be provided.
func WithSelector(selectors ...engine.RuleSelector) EngineOption {
	return func(options *engineOptions) {
		options.selectors = selectors
	}
}
