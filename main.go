package main

import (
	"context"
	"fmt"
	"os"

	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/parser"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/provider/lib"
)

const (
	// This must eventually be a default that makes sense, and overrideable by env var or flag.
	SETTING_FILE_PATH = "./provider_settings.json"
	RULES_FILE_PATH   = "./rule-example.json"
)

func main() {
	ctx := context.Background()

	// Get the configs
	configs, err := lib.GetConfig(SETTING_FILE_PATH)
	if err != nil {
		fmt.Printf("\n%v\n", err)
		os.Exit(1)
	}

	//start up the rule engine
	engine := engine.CreateRuleEngine(ctx, 10)

	providers := map[string]provider.Client{}

	for _, config := range configs {
		provider, err := provider.GetProviderClient(config)
		if err != nil {
			fmt.Printf("\n%v\n", err)
			os.Exit(1)
		}
		providers[config.Name] = provider
	}

	parser := parser.RuleParser{
		ProviderNameToClient: providers,
	}

	rules, needProviders, err := parser.LoadRules(RULES_FILE_PATH)
	if err != nil {
		fmt.Printf("\n%v\n", err)
		os.Exit(1)
	}

	// Now that we have all the providers, we need to start them.
	for _, provider := range needProviders {
		err := provider.Init(ctx)
		if err != nil {
			fmt.Printf("\n%v\n", err)
			os.Exit(1)
		}
	}

	engine.RunRules(ctx, rules)

}
