package konveyor

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/engine/labels"
	v1 "github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/parser"
	"github.com/konveyor/analyzer-lsp/progress"
	"github.com/konveyor/analyzer-lsp/progress/collector"
	"github.com/konveyor/analyzer-lsp/provider"
)

type parserConfig struct {
	rulePaths               []string
	dependencyRulesDisabled bool
	depLabelSelector        string
}

// TODO: Docs
type analyzer struct {
	parserConfig
	ruleset            []engine.RuleSet
	engine             engine.RuleEngine
	cancelFunc         context.CancelFunc
	ctx                context.Context
	providers          []Provider
	allConfigProviders map[string]provider.InternalProviderClient
	providerConditions map[string][]provider.ConditionsByCap
	log                logr.Logger
	progress           *progress.Progress
	collector          progress.Collector
}

var _ Analyzer = &analyzer{}

// ParseRules
// Either use the the rules that are set during anlaysis creation time, or use the rules that you pass here.
func (a *analyzer) ParseRules(rulePaths ...string) (Rules, error) {
	a.log.Info("Parsing rules")
	// Now we have all the provider clients that have been configured. We can look at the rules to determine which are needed.
	collector := collector.New()
	a.progress.Subscribe(collector)
	parser := parser.RuleParser{
		ProviderNameToClient: a.allConfigProviders,
		Log:                  a.log.WithName("rule-parser"),
		NoDependencyRules:    a.dependencyRulesDisabled,
	}
	if a.depLabelSelector != "" {
		var err error
		parser.DepLabelSelector, err = labels.NewLabelSelector[*v1.Dep](a.depLabelSelector, nil)
		if err != nil {
			return nil, fmt.Errorf("unable to create dependency label selector: %w", err)
		}
	}

	if len(rulePaths) == 0 {
		rulePaths = a.rulePaths
	}

	ruleSets := []engine.RuleSet{}
	neededProviders := map[string]provider.InternalProviderClient{}
	providerConditions := map[string][]provider.ConditionsByCap{}
	parserErrors := []error{}
	collector.Report(progress.Event{
		Timestamp: time.Now(),
		Stage:     progress.StageRuleParsing,
		Message:   "starting to parse rules",
		Current:   0,
		Total:     len(a.rulePaths),
	})
	for i, f := range a.rulePaths {
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
		collector.Report(progress.Event{
			Timestamp: time.Time{},
			Stage:     progress.StageRuleParsing,
			Message:   fmt.Sprintf("finished parsing rules for: %s", f),
			Current:   i + 1,
		})

	}
	if len(parserErrors) > 0 {
		return nil, fmt.Errorf("unable to parse rules: %w", errors.Join(parserErrors...))
	}

	a.ruleset = ruleSets
	a.providerConditions = providerConditions
	providers := []Provider{}
	for name, pv := range neededProviders {
		providers = append(providers, Provider{
			Name:     name,
			provider: pv,
		})
	}
	a.providers = providers
	return a, nil
}

func (a *analyzer) ProviderStart() error {
	a.log.Info("Starting providers")
	if len(a.allConfigProviders) == 0 || len(a.providers) == 0 {
		return fmt.Errorf("no providers to start")
	}
	additionalBuiltinConfigs := []provider.InitConfig{}
	providerInitErrors := []error{}
	var builtinProvider *Provider

	a.collector.Report(progress.Event{
		Stage:   progress.StageProviderInit,
		Message: "Staring provider init",
		Total:   len(a.providers),
	})
	abConfigChan := make(chan []provider.InitConfig)
	providerInitCtx, cancelFunc := context.WithCancel(a.ctx)
	waitGroup := sync.WaitGroup{}
	go func() {
		for {
			select {
			case config := <-abConfigChan:
				waitGroup.Done()
				additionalBuiltinConfigs = append(additionalBuiltinConfigs, config...)
			case <-providerInitCtx.Done():
				return
			}
		}
	}()

	for i, pv := range a.providers {
		switch pv.Name {
		case "builtin":
			builtinProvider = &pv
			continue
		default:
			// TODO: Handle tracing.
			waitGroup.Add(1)
			go func() {
				a.log.Info("provider init", "provider", pv.Name)
				additionalBuiltins, err := pv.provider.ProviderInit(a.ctx, nil)
				if err != nil {
					a.log.Error(err, "unable to init provider")
					providerInitErrors = append(providerInitErrors, err)
				}
				a.collector.Report(progress.Event{
					Stage:   progress.StageProviderInit,
					Message: fmt.Sprintf("started provider: %s", pv.Name),
					Current: i + 1,
					Total:   len(a.providers),
				})
				abConfigChan <- additionalBuiltins
			}()
		}
	}

	c := make(chan struct{})
	go func() {
		waitGroup.Wait()
		close(c)
	}()

	select {
	case <-c:
		a.log.V(3).Info("started all non builtin providers")
	case <-time.After(4 * time.Minute):
		cancelFunc()
		return fmt.Errorf("timed out starting providers")
	}
	cancelFunc()

	// Init builtins
	if builtinProvider != nil {
		if _, err := builtinProvider.provider.ProviderInit(a.ctx, additionalBuiltinConfigs); err != nil {
			providerInitErrors = append(providerInitErrors, err)
		}
		a.collector.Report(progress.Event{
			Stage:   progress.StageProviderInit,
			Message: fmt.Sprintf("started provider: %s", "builtin"),
			// This might not be true, should come back and fix
			Current: len(a.providers),
			Total:   len(a.providers),
		})
	}

	if len(providerInitErrors) > 0 {
		return fmt.Errorf("unable to initialize providers: %w", errors.Join(providerInitErrors...))
	}

	a.collector.Report(progress.Event{
		Stage:   progress.StageProviderInit,
		Message: "all providers have been initialized",
	})

	// Call Prepare on all providers.
	prepareError := []error{}
	for name, conditions := range a.providerConditions {
		for _, provider := range a.providers {
			if name == provider.Name {
				if err := provider.provider.Prepare(a.ctx, conditions); err != nil {
					// TODO: Handle Wrapping
					prepareError = append(prepareError, err)
				}
			}
		}
	}
	if len(prepareError) > 0 {
		return fmt.Errorf("unable to prepare providers: %w", errors.Join(prepareError...))
	}

	return nil
}

func (a *analyzer) Run(options ...EngineOption) []v1.RuleSet {
	a.log.Info("Running analysis")
	if len(a.ruleset) == 0 {
		a.log.Error(fmt.Errorf("rules must be parsed before running rules"), "unable to run analysis")
		return nil
	}
	if len(a.providers) == 0 {
		a.log.Error(fmt.Errorf("providers must be started before running rules"), "unable to run analysis")
		return nil
	}

	engineOptions := engineOptions{}
	for _, opt := range options {
		opt(&engineOptions)
	}
	// TODO: Handle ProgressReporter
	if engineOptions.progressReporter == nil {
		collector := collector.New()
		a.progress.Subscribe(collector)
		engineOptions.progressReporter = collector
	}
	// TODO: Handle Scopes
	ruleset := a.engine.RunRulesWithOptions(a.ctx, a.ruleset, []engine.RunOption{
		engine.WithProgressReporter(engineOptions.progressReporter),
	}, engineOptions.selectors...)

	sort.SliceStable(ruleset, func(i, j int) bool {
		return ruleset[i].Name < ruleset[j].Name
	})
	a.log.Info("finished running analysis")
	return ruleset

}

func (a *analyzer) RuleLabels() []string {
	if len(a.ruleset) == 0 {
		a.log.Info("no ruleset's to get get labels from")
		return []string{}
	}
	labels := map[string]any{}
	for _, rs := range a.ruleset {
		for _, label := range rs.Labels {
			labels[label] = nil
		}
	}
	return slices.Collect(maps.Keys(labels))
}

func (a *analyzer) RulesetFilepaths() map[string]string {
	if len(a.ruleset) == 0 {
		a.log.Info("no ruleset's to get get labels from")
		return map[string]string{}
	}

	filePaths := map[string]string{}
	// TODO: Need to save more info on analyzer when looping through files.
	return filePaths
}

// note this is going to use name, as a proxy for language
func (a *analyzer) GetProviderForLanguage(language string) (Provider, bool) {
	if len(a.allConfigProviders) == 0 {
		a.log.Info("no providers to get look for")
		return Provider{}, false
	}

	for name, pv := range a.allConfigProviders {
		if name == language {
			return Provider{
				Name:     name,
				provider: pv,
			}, true
		}
	}
	return Provider{}, false
}

// This will only loop over the needed providers after parsing rules
func (a *analyzer) GetProviders(filters ...Filter) []Provider {
	if len(a.providers) == 0 {
		a.log.Info("no providers to get look for")
		return nil
	}

	r := map[string]Provider{}
	for _, p := range a.providers {
		for _, filter := range filters {
			if filter(p) {
				r[p.Name] = p
			}
		}
	}

	return slices.Collect(maps.Values(r))
}

// TODO: Need to add
func (a *analyzer) GetDependencies(outputFilePath string, tree bool) error {
	return nil
}

func (a *analyzer) Stop() error {
	a.engine.Stop()
	for _, pv := range a.allConfigProviders {
		pv.Stop()
	}
	a.progress.Unsubscribe(a.collector)
	a.cancelFunc()
	return nil
}
