package konveyor

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/engine/labels"
	"github.com/konveyor/analyzer-lsp/progress"
	"github.com/konveyor/analyzer-lsp/provider"
)

// ANALYSIS OPTIONS
// TODO: Write validatation logic
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
	log                     logr.Logger
	ctx                     context.Context
}

type selectorError []error

func (a *analyzerOptions) getSelectors() ([]engine.RuleSelector, error) {
	selectors := []engine.RuleSelector{}
	selectorErrors := selectorError{}
	if a.labelSelector != "" {
		selector, err := labels.NewLabelSelector[*engine.RuleMeta](a.labelSelector, nil)
		if err != nil {
			selectorErrors = append(selectorErrors, err)
		}
		selectors = append(selectors, selector)
	}
	return selectors, nil
}

// OPTIONALS FOR ENDUSER
func WithProviderConfigFilePath(providerConfigFilePath string) AnalyzerOption {
	return func(opt *analyzerOptions) (err error) {
		opt.providerConfigFilePath = providerConfigFilePath
		return
	}
}
func WithLabelSelector(labelSelector string) AnalyzerOption {
	return func(opt *analyzerOptions) (err error) {
		opt.labelSelector = labelSelector
		return
	}
}
func WithDepLabelSelector(labelSelector string) AnalyzerOption {
	return func(opt *analyzerOptions) (err error) {
		opt.incidentSelector = labelSelector
		return
	}
}
func WitIncidentSelector(selector string) AnalyzerOption {
	return func(opt *analyzerOptions) (err error) {
		opt.depLabelSelector = selector
		return
	}
}
func WithIncidentLimit(limit int) AnalyzerOption {
	return func(opt *analyzerOptions) (err error) {
		opt.incidentLimit = limit
		return
	}
}
func WithCodeSnipLimit(limit int) AnalyzerOption {
	return func(opt *analyzerOptions) (err error) {
		opt.codeSnipLimit = limit
		return
	}
}
func WithContextLinesLimit(limit int) AnalyzerOption {
	return func(opt *analyzerOptions) (err error) {
		opt.contextLineLimit = limit
		return
	}
}
func WithAnalysisMode(mode provider.AnalysisMode) AnalyzerOption {
	return func(opt *analyzerOptions) (err error) {
		opt.analysisMode = mode
		return
	}
}
func WithDependencyRulesDisabled() AnalyzerOption {
	return func(opt *analyzerOptions) (err error) {
		opt.dependencyRulesDisabled = true
		return
	}
}
func WithLogger(log logr.Logger) AnalyzerOption {
	return func(opt *analyzerOptions) (err error) {
		opt.log = log
		return
	}
}
func WithContext(ctx context.Context) AnalyzerOption {
	return func(opt *analyzerOptions) (err error) {
		opt.ctx = ctx
		return
	}
}

// ENGINE OPTIONS
func WithScope(scope engine.Scope) EngineOption {
	return func(options *engineOptions) {
		options.Scope = scope
	}
}

func WithProgressReporter(reporter progress.ProgressReporter) EngineOption {
	return func(options *engineOptions) {
		options.progressReporter = reporter
	}
}

func WithSelector(selectors ...engine.RuleSelector) EngineOption {
	return func(options *engineOptions) {
		options.selectors = selectors
	}
}
