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
	reporters               []progress.Reporter
	progress                *progress.Progress
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
func WithRuleFilepaths(rules []string) AnalyzerOption {
	return func(options *analyzerOptions) error {
		options.rulesFilepaths = rules
		return nil
	}
}
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
func WithAnalysisMode(mode string) AnalyzerOption {
	return func(opt *analyzerOptions) (err error) {
		switch mode {
		case string(provider.FullAnalysisMode):
			opt.analysisMode = provider.FullAnalysisMode
		case string(provider.SourceOnlyAnalysisMode):
			opt.analysisMode = provider.SourceOnlyAnalysisMode
		default:
		}
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
		log.Info("here")
		opt.log = log
		log.Info("here", "zero?", opt.log.IsZero())
		return
	}
}
func WithContext(ctx context.Context) AnalyzerOption {
	return func(opt *analyzerOptions) (err error) {
		opt.ctx = ctx
		return
	}
}
func WithProgress(progress *progress.Progress) AnalyzerOption {
	return func(opt *analyzerOptions) (err error) {
		opt.progress = progress
		return
	}
}
func WithReporters(reporters ...progress.Reporter) AnalyzerOption {
	return func(options *analyzerOptions) error {
		options.reporters = reporters
		return nil
	}
}

// ENGINE OPTIONS
func WithScope(scope engine.Scope) EngineOption {
	return func(options *engineOptions) {
		options.Scope = scope
	}
}

// This should be a collector
func WithProgressReporter(reporter progress.Reporter) EngineOption {
	return func(options *engineOptions) {
		options.progressReporter = reporter
	}
}

func WithSelector(selectors ...engine.RuleSelector) EngineOption {
	return func(options *engineOptions) {
		options.selectors = selectors
	}
}
