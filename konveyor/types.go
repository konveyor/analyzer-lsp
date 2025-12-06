package konveyor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/engine"
	v1 "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/progress"
	"github.com/konveyor/analyzer-lsp/progress/collector"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/provider/lib"
)

type Analyzer interface {
	ProviderStart() error
	GetProviderForLanguage(language string) (Provider, bool)
	GetProviders(...Filter) []Provider
	GetDependencies(outputFilePath string, tree bool) error
	ParseRules(...string) (Rules, error)
	Engine
	Rules
	Stop() error
}

type Filter func(p Provider) bool

type AnalyzerOption func(options *analyzerOptions) error

// TODO: Handle Tracing
// TODO: Handle encoding
func NewAnalyzer(options ...AnalyzerOption) (Analyzer, error) {
	validationErrors := []error{}
	opts := analyzerOptions{}
	for _, apply := range options {
		if err := apply(&opts); err != nil {
			validationErrors = append(validationErrors, err)
		}
	}
	if len(validationErrors) > 0 {
		return nil, fmt.Errorf("unable to get Analyzer: %w", errors.Join(validationErrors...))
	}
	log := opts.log
	if log.IsZero() {
		log = logr.Discard()
	}

	log.V(7).Info("setting up progress")
	if opts.progress == nil {
		var err error
		opts.progress, err = progress.New(progress.WithReporters(opts.reporters...))
		if err != nil {
			return nil, fmt.Errorf("unable to create progress reporter: %w", err)
		}
	}

	collector := collector.New()
	opts.progress.Subscribe(collector)
	if len(validationErrors) > 0 {
		return nil, fmt.Errorf("unable to get new analyzer: %w", errors.Join(validationErrors...))
	}

	log.V(7).Info("Getting Config")
	providerConfig, err := provider.GetConfig(opts.providerConfigFilePath)
	if err != nil {
		return nil, fmt.Errorf("unable to get provider config: %w", err)
	}
	log.V(7).Info("got Config")

	finalConfigs, locations := setupProviderConfigs(providerConfig)
	log.V(3).Info("loaded provider configs", "locations", locations)

	providerErrors := []error{}
	providers := map[string]provider.InternalProviderClient{}
	ctx := opts.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancelFunc := context.WithCancel(ctx)
	for _, config := range finalConfigs {
		prov, err := lib.GetProviderClient(config, log, opts.progress)
		if err != nil {
			providerErrors = append(providerErrors, err)
			continue
		}
		providers[config.Name] = prov
		if startable, ok := prov.(provider.Startable); ok {
			err = startable.Start(ctx)
			if err != nil {
				providerErrors = append(providerErrors, err)
				continue
			}
			collector.Report(progress.Event{
				Timestamp: time.Now(),
				Stage:     progress.StageProviderStart,
				Message:   fmt.Sprintf("started provider: %s", config.Name),
			})
		}
	}

	if len(providerErrors) > 0 {
		cancelFunc()
		return nil, fmt.Errorf("unable to get provider clients: %w", errors.Join(providerErrors...))
	}
	eng := engine.CreateRuleEngine(ctx,
		10,
		log,
		engine.WithIncidentLimit(opts.incidentLimit),
		engine.WithCodeSnipLimit(opts.codeSnipLimit),
		engine.WithContextLines(opts.contextLineLimit),
		engine.WithIncidentSelector(opts.incidentSelector),
		engine.WithLocationPrefixes(locations),
	)

	// Create new Provider Struct
	return &analyzer{
		parserConfig: parserConfig{
			rulePaths:               opts.rulesFilepaths,
			dependencyRulesDisabled: opts.dependencyRulesDisabled,
			depLabelSelector:        opts.depLabelSelector,
		},
		engine:             eng,
		cancelFunc:         cancelFunc,
		ctx:                ctx,
		allConfigProviders: providers,
		log:                log,
		progress:           opts.progress,
		collector:          collector,
	}, nil
}

type Config interface {
	GetConfigValue(configKey string) (any, bool)
}

// Rules Introspection
// TODO: ADD DOCS
type Rules interface {
	RuleLabels() []string
	RulesetFilepaths() map[string]string
}

// ENGINE OPTIONS
// TODO: ADD DOCS
type Engine interface {
	Run(options ...EngineOption) []v1.RuleSet
}

type engineOptions struct {
	// TODO: doc links
	Scope            engine.Scope
	progressReporter progress.Reporter
	selectors        []engine.RuleSelector
}

type EngineOption func(options *engineOptions)
