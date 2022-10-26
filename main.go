package main

import (
	"context"
	"os"
	"sort"

	logrusr "github.com/bombsimon/logrusr/v3"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/parser"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/provider/lib"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

const (
	// This must eventually be a default that makes sense, and overrideable by env var or flag.
	SETTING_FILE_PATH = "./provider_settings.json"
	RULES_FILE_PATH   = "./rule-example.yaml"
	OUTPUT_VIOLATIONS = "./output.yaml"
)

func main() {
	ctx, cancelFunc := context.WithCancel(context.Background())

	logrusLog := logrus.New()
	logrusLog.SetOutput(os.Stdout)
	logrusLog.SetFormatter(&logrus.TextFormatter{})
	// need to do research on mapping in logrusr to level here TODO
	logrusLog.SetLevel(9)

	log := logrusr.New(logrusLog)

	// Get the configs
	configs, err := lib.GetConfig(SETTING_FILE_PATH)
	if err != nil {
		log.Error(err, "unable to get configuration")
		os.Exit(1)
	}

	//start up the rule engine
	engine := engine.CreateRuleEngine(ctx, 10, log)

	providers := map[string]provider.Client{}

	for _, config := range configs {
		provider, err := provider.GetProviderClient(config)
		if err != nil {
			log.Error(err, "unable to create provider client")
			os.Exit(1)
		}
		providers[config.Name] = provider
	}

	parser := parser.RuleParser{
		ProviderNameToClient: providers,
	}

	rules, needProviders, err := parser.LoadRules(RULES_FILE_PATH)
	if err != nil {
		log.Error(err, "unable to parse all the rules")
		os.Exit(1)
	}

	// Now that we have all the providers, we need to start them.
	for _, provider := range needProviders {
		err := provider.Init(ctx, log)
		if err != nil {
			log.Error(err, "unable to init the providers")
			os.Exit(1)
		}
	}

	violations := engine.RunRules(ctx, rules)
	engine.Stop()

	for _, provider := range needProviders {
		provider.Stop()
	}

	sort.SliceStable(violations, func(i, j int) bool {
		return violations[i].RuleID < violations[j].RuleID
	})

	//Before we exit we need to clean up
	cancelFunc()

	// Write results out to CLI
	b, _ := yaml.Marshal(violations)

	os.WriteFile("violation_output.yaml", b, 0644)
}
