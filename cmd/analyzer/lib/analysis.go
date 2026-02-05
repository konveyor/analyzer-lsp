package lib

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/engine/labels"
	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/parser"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/provider/lib"
)

// AnalysisConfig holds configuration for running analysis
type AnalysisConfig struct {
	ProviderSettings  string
	RulesFiles        []string
	LabelSelector     string
	DepLabelSelector  string
	IncidentSelector  string
	IncidentLimit     int
	CodeSnipLimit     int
	ContextLines      int
	AnalysisMode      string
	NoDependencyRules bool
}

// RunAnalysis performs analysis and returns the results
// This is extracted from cmd/analyzer/main.go to allow reuse
func RunAnalysis(ctx context.Context, config AnalysisConfig, log logr.Logger) ([]konveyor.RuleSet, error) {
	// Get provider configs
	configs, err := provider.GetConfig(config.ProviderSettings)
	if err != nil {
		return nil, fmt.Errorf("unable to get configuration: %w", err)
	}

	// Setup builtin configs
	defaultBuiltinConfigs := []provider.InitConfig{}
	seenBuiltinConfigs := map[string]bool{}
	finalConfigs := []provider.Config{}
	for _, provConfig := range configs {
		if provConfig.Name != "builtin" {
			finalConfigs = append(finalConfigs, provConfig)
		}
		for _, initConf := range provConfig.InitConfig {
			if _, ok := seenBuiltinConfigs[initConf.Location]; !ok {
				if initConf.Location != "" {
					if stat, err := os.Stat(initConf.Location); err == nil && stat.IsDir() {
						builtinLocation, err := filepath.Abs(initConf.Location)
						if err != nil {
							builtinLocation = initConf.Location
						}
						seenBuiltinConfigs[builtinLocation] = true
						builtinConf := provider.InitConfig{Location: builtinLocation}
						if provConfig.Name == "builtin" {
							builtinConf.ProviderSpecificConfig = initConf.ProviderSpecificConfig
						}
						defaultBuiltinConfigs = append(defaultBuiltinConfigs, builtinConf)
					}
				}
			}
		}
	}
	finalConfigs = append(finalConfigs, provider.Config{
		Name:       "builtin",
		InitConfig: defaultBuiltinConfigs,
	})

	// Create provider clients
	providers := map[string]provider.InternalProviderClient{}
	providerLocations := []string{}
	for _, provConfig := range finalConfigs {
		provConfig.ContextLines = config.ContextLines
		for _, ind := range provConfig.InitConfig {
			providerLocations = append(providerLocations, ind.Location)
		}
		// Override analysis mode if set
		if config.AnalysisMode != "" {
			inits := []provider.InitConfig{}
			for _, i := range provConfig.InitConfig {
				i.AnalysisMode = provider.AnalysisMode(config.AnalysisMode)
				inits = append(inits, i)
			}
			provConfig.InitConfig = inits
		}
		prov, err := lib.GetProviderClient(provConfig, log)
		if err != nil {
			return nil, fmt.Errorf("unable to create provider client: %w", err)
		}
		providers[provConfig.Name] = prov
		if s, ok := prov.(provider.Startable); ok {
			if err := s.Start(ctx); err != nil {
				return nil, fmt.Errorf("unable to start provider: %w", err)
			}
		}
	}

	// Cleanup providers on exit
	defer func() {
		for _, prov := range providers {
			prov.Stop()
		}
	}()

	// Create label selectors
	selectors := []engine.RuleSelector{}
	if config.LabelSelector != "" {
		selector, err := labels.NewLabelSelector[*engine.RuleMeta](config.LabelSelector, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create label selector: %w", err)
		}
		selectors = append(selectors, selector)
	}

	var dependencyLabelSelector *labels.LabelSelector[*konveyor.Dep]
	if config.DepLabelSelector != "" {
		dependencyLabelSelector, err = labels.NewLabelSelector[*konveyor.Dep](config.DepLabelSelector, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create dependency label selector: %w", err)
		}
	}

	// Create rule engine
	eng := engine.CreateRuleEngine(ctx,
		10,
		log,
		engine.WithIncidentLimit(config.IncidentLimit),
		engine.WithCodeSnipLimit(config.CodeSnipLimit),
		engine.WithContextLines(config.ContextLines),
		engine.WithIncidentSelector(config.IncidentSelector),
		engine.WithLocationPrefixes(providerLocations),
	)
	defer eng.Stop()

	// Parse rules
	ruleParser := parser.RuleParser{
		ProviderNameToClient: providers,
		Log:                  log.WithName("parser"),
		NoDependencyRules:    config.NoDependencyRules,
		DepLabelSelector:     dependencyLabelSelector,
	}

	ruleSets := []engine.RuleSet{}
	needProviders := map[string]provider.InternalProviderClient{}
	for _, f := range config.RulesFiles {
		internRuleSet, internNeedProviders, _, err := ruleParser.LoadRules(f)
		if err != nil {
			return nil, fmt.Errorf("unable to parse rules from %s: %w", f, err)
		}
		ruleSets = append(ruleSets, internRuleSet...)
		for k, v := range internNeedProviders {
			needProviders[k] = v
		}
	}

	// Initialize needed providers
	additionalBuiltinConfigs := []provider.InitConfig{}
	for name, provider := range needProviders {
		switch name {
		case "builtin":
			continue
		default:
			additionalBuiltinConfs, err := provider.ProviderInit(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("unable to init provider %s: %w", name, err)
			}
			if additionalBuiltinConfs != nil {
				additionalBuiltinConfigs = append(additionalBuiltinConfigs, additionalBuiltinConfs...)
			}
		}
	}

	if builtinClient, ok := needProviders["builtin"]; ok {
		if _, err = builtinClient.ProviderInit(ctx, additionalBuiltinConfigs); err != nil {
			return nil, fmt.Errorf("unable to init builtin provider: %w", err)
		}
	}

	// Run analysis
	rulesets := eng.RunRules(ctx, ruleSets, selectors...)

	// Sort results
	sort.SliceStable(rulesets, func(i, j int) bool {
		return rulesets[i].Name < rulesets[j].Name
	})

	return rulesets, nil
}
