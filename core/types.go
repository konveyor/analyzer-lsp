package core

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

	// Mutual exclusivity validation
	if len(opts.providerConfigs) > 0 && opts.providerConfigFilePath != "" {
		validationErrors = append(validationErrors,
			fmt.Errorf("cannot specify both provider configs and provider config file path"))
	}
	if len(opts.providerConfigs) == 0 && opts.providerConfigFilePath == "" {
		validationErrors = append(validationErrors,
			fmt.Errorf("must specify either provider configs or provider config file path"))
	}
	if len(opts.pathMappings) > 0 && opts.ignoreAdditionalBuiltinConfigs {
		validationErrors = append(validationErrors,
			fmt.Errorf("cannot specify path mappings when additional builtin configs are ignored; path mappings have no effect"))
	}

	if len(validationErrors) > 0 {
		return nil, fmt.Errorf("unable to get Analyzer: %w", errors.Join(validationErrors...))
	}
	log := opts.log
	if log.IsZero() {
		log = logr.Discard()
	}

	log.V(5).Info("setting up progress")
	if opts.progress == nil {
		var err error
		opts.progress, err = progress.New(progress.WithReporters(opts.reporters...))
		if err != nil {
			return nil, fmt.Errorf("unable to create progress reporter: %w", err)
		}
	}

	collector := collector.New()
	opts.progress.Subscribe(collector)

	// Load provider configs from file or use programmatic configs
	var providerConfig []provider.Config
	if len(opts.providerConfigs) > 0 {
		log.V(5).Info("Using programmatic provider configs")
		providerConfig = opts.providerConfigs
		// Apply the same proxy defaulting and validation that GetConfig does
		if err := provider.ValidateAndDefaultConfigs(providerConfig); err != nil {
			return nil, fmt.Errorf("unable to validate provider configs: %w", err)
		}
	} else {
		log.V(5).Info("Getting Config from file")
		var err error
		providerConfig, err = provider.GetConfig(opts.providerConfigFilePath)
		if err != nil {
			return nil, fmt.Errorf("unable to get provider config: %w", err)
		}
	}
	log.V(5).Info("got Config")

	finalConfigs, locations := setupProviderConfigs(
		providerConfig, opts.ignoreAdditionalBuiltinConfigs, opts.pathMappings)
	log.V(3).Info("loaded provider configs", "locations", locations)

	log.Info("handling analyis mode from opts", "mode", opts.analysisMode)

	providerErrors := []error{}
	providers := map[string]provider.InternalProviderClient{}
	ctx := opts.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancelFunc := context.WithCancel(ctx)
	for _, config := range finalConfigs {
		if opts.analysisMode != "" {
			for i := range config.InitConfig {
				config.InitConfig[i].AnalysisMode = opts.analysisMode
			}
		}

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

	// Set default worker count if not specified
	workerCount := opts.workerCount
	if workerCount == 0 {
		workerCount = 10
	}

	eng := engine.CreateRuleEngine(ctx,
		workerCount,
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
		engine:                         eng,
		cancelFunc:                     cancelFunc,
		ctx:                            ctx,
		allConfigProviders:             providers,
		log:                            log,
		progress:                       opts.progress,
		collector:                      collector,
		labelSelector:                  opts.labelSelector,
		pathMappings:                   opts.pathMappings,
		ignoreAdditionalBuiltinConfigs: opts.ignoreAdditionalBuiltinConfigs,
	}, nil
}

type Config interface {
	GetConfigValue(configKey string) (any, bool)
}

// Rules Introspection
// TODO: ADD DOCS
type Rules interface {
	RuleLabels() []string
	// TODO: (shawn-hurley) add ability to get the filepaths to loaded rulesets.
	//RulesetFilepaths() map[string]string
	RuleSets() []engine.RuleSet
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
