package main

import (
	"context"
	"fmt"
	"os"

	"github.com/shawn-hurley/jsonrpc-golang/parser"
	"github.com/shawn-hurley/jsonrpc-golang/provider"
	"github.com/shawn-hurley/jsonrpc-golang/provider/lib"
)

const (
	// This must eventually be a default that makes sense, and overrideable by env var or flag.
	SETTING_FILE_PATH = "/home/shurley/repos/jsonrpc-golang/provider_settings.json"
	RULES_FILE_PATH   = "/home/shurley/repos/jsonrpc-golang/rule-example.json"
)

func main() {
	ctx := context.Background()

	// Get the configs
	configs, err := lib.GetConfig(SETTING_FILE_PATH)
	if err != nil {
		fmt.Printf("\n%v\n", err)
		os.Exit(1)
	}

	providers := map[string]provider.Client{}

	for _, config := range configs {
		provider, err := provider.GetProviderClient(config)
		if err != nil {
			fmt.Printf("\n%v\n", err)
			os.Exit(1)
		}
		providers[config.Name] = provider
	}

	// Now that we have all the providers, we need to start them.
	for _, provider := range providers {
		err := provider.Init(ctx)
		if err != nil {
			fmt.Printf("\n%v\n", err)
			os.Exit(1)
		}
	}

	parser := parser.RuleParser{
		ProviderNameToClient: providers,
	}

	rules, err := parser.LoadRules(RULES_FILE_PATH)
	if err != nil {
		fmt.Printf("\n%v\n", err)
		os.Exit(1)
	}

	fmt.Printf("rules - %#v", rules)

}
