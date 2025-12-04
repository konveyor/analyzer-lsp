package konveyor

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/engine/labels"
	v1 "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/parser"
	"github.com/konveyor/analyzer-lsp/progress"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/provider/lib"
	"github.com/swaggest/openapi-go/openapi3"
)

type validationErrors []error

type Analyzer interface {
	GetProviderForLanguage(language string) (Provider, bool)
	GetProviders() []Provider
	GetDependencies(outputFilePath string, tree bool) error
	Engine
	Rules
	Stop() error
}

// TODO: Figure out how to support progress reporting.

type AnalyzerOption func(options *analyzerOptions) error

func NewAnalyzer(options ...AnalyzerOption) (Analyzer, error) {
	opts := analyzerOptions{}
	validationErrors := validationErrors{}
	log := opts.log
	if log.IsZero() {
		log = logr.Discard()
	}
	for _, apply := range options {
		if err := apply(&opts); err != nil {
			log.Error(err, "validation failed")
			validationErrors = append(validationErrors, err)
		}
	}

	if len(validationErrors) > 0 {
		return nil, fmt.Errorf("unable to get new analyzer: %w", errors.Join(validationErrors...))
	}

	providerConfig, err := provider.GetConfig(opts.providerConfigFilePath)
	if err != nil {
		return nil, fmt.Errorf("unable to get provider config: %w", err)
	}

	finalConfigs, locations := setupProviderConfigs(providerConfig)
	log.V(3).Info("loaded provider configs", "locations", locations)

	providerErrors := []error{}
	providers := map[string]provider.InternalProviderClient{}
	for _, config := range finalConfigs {
		prov, err := lib.GetProviderClient(config, log)
		if err != nil {
			providerErrors = append(providerErrors, err)
			continue
		}
		providers[config.Name] = prov
	}

	if len(providerErrors) > 0 {
		return nil, fmt.Errorf("unable to get provider clients: %w", providerErrors)
	}
	// Now we have all the provider clients that have been configured. We can look at the rules to determine which are needed.
	parser := parser.RuleParser{
		ProviderNameToClient: providers,
		Log:                  log.WithName("rule-parser"),
		NoDependencyRules:    opts.dependencyRulesDisabled,
	}
	if opts.depLabelSelector != "" {
		var err error
		parser.DepLabelSelector, err = labels.NewLabelSelector[*v1.Dep](opts.depLabelSelector, nil)
		if err != nil {
			return nil, fmt.Errorf("unable to create dependency label selector: %w", err)
		}
	}

	ruleSets := []engine.RuleSet{}
	neededProviders := map[string]provider.InternalProviderClient{}
	providerConditions := map[string][]provider.ConditionsByCap{}
	parserErrors := []error{}
	for _, f := range opts.rulesFilepaths {
		rs, np, pc, err := parser.LoadRules(f)
		if err != nil {
			parserErrors = append(parserErrors, err)
			continue
		}
		ruleSets = append(ruleSets, rs...)
		maps.Copy(neededProviders, np)
		for k, v := range pc {
			c := providerConditions[k]
			providerConditions[k] = append(c, v...)
		}
	}

	ctx := opts.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancelFunc := context.WithCancel(ctx)

	additionalBuiltinConfigs := []provider.InitConfig{}
	providerInitErrors := []error{}
	// Start the needed providers
	for name, provider := range neededProviders {
		switch name {
		case "builtin":
			continue
		default:
			// TODO: Handle tracing.
			additionalBuiltins, err := provider.ProviderInit(ctx, nil)
			if err != nil {
				providerInitErrors = append(providerInitErrors, err)
				continue
			}
			additionalBuiltins = append(additionalBuiltins, additionalBuiltinConfigs...)
		}
	}

	// Init builtins
	if builtinClient, ok := neededProviders["builtin"]; ok {
		if _, err = builtinClient.ProviderInit(ctx, additionalBuiltinConfigs); err != nil {
			providerInitErrors = append(providerInitErrors, err)
		}
	}

	if len(providerInitErrors) > 0 {
		return nil, fmt.Errorf("unable to initialize providers: %w", providerInitErrors)
	}

	// Call Prepare on all providers.
	for name, conditions := range providerConditions {
		if provider, ok := neededProviders[name]; ok {
			if err := provider.Prepare(ctx, conditions); err != nil {
				// TODO: Handle Wrapping
				return nil, err
			}
		}
	}
	//TODO: Handle DepLabelSelector
	eng := engine.CreateRuleEngine(ctx,
		10,
		log,
		engine.WithIncidentLimit(opts.incidentLimit),
		engine.WithCodeSnipLimit(opts.codeSnipLimit),
		engine.WithContextLines(opts.contextLineLimit),
		engine.WithIncidentSelector(opts.incidentSelector),
		engine.WithLocationPrefixes(locations),
		// TODO: Handle encoding
	)

	// Create new Provider Struct
	return &analyzer{
		ruleSets,
		eng,
		cancelFunc,
		ctx,
		providers,
	}, nil
}

// PROVIDERS
// TODO: ADD DOCS
type Address struct {
	Socket *SocketAddress
	Http   *HttpAddress
}

type SocketAddress struct {
}

type HttpAddress struct {
}

type Provider struct {
	Name          string
	Address       Address
	ConditionSpec openapi3.Schema
	Config
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
	progressReporter progress.ProgressReporter
	selectors        []engine.RuleSelector
}

type EngineOption func(options *engineOptions)
