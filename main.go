package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"

	logrusr "github.com/bombsimon/logrusr/v3"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/parser"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/provider/lib"
	"github.com/konveyor/analyzer-lsp/tracing"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

const (
	EXIT_ON_ERROR_CODE = 3
)

var (
	settingsFile      = flag.String("provider-settings", "provider_settings.json", "path to the provider settings")
	rulesFile         = flag.String("rules", "rule-example.yaml", "filename or directory containing rule files")
	outputViolations  = flag.String("output-file", "output.yaml", "filepath to to store rule violations")
	errorOnViolations = flag.Bool("error-on-violation", false, "exit with 3 if any violation are found will also print violations to console")
	logLevel          = flag.Int("verbose", 9, "level for logging output")
	enableJaeger      = flag.Bool("enable-jaeger", false, "enable tracer exports to jaeger endpoint")
	jaegerEndpoint    = flag.String("jaeger-endpoint", "http://localhost:14268/api/traces", "jaeger endpoint to collect tracing data")
)

func main() {
	flag.Parse()

	err := validateFlags()
	if err != nil {
		fmt.Printf("%v\n", err)
		os.Exit(1)
	}

	logrusLog := logrus.New()
	logrusLog.SetOutput(os.Stdout)
	logrusLog.SetFormatter(&logrus.TextFormatter{})
	// need to do research on mapping in logrusr to level here TODO
	logrusLog.SetLevel(logrus.Level(*logLevel))

	log := logrusr.New(logrusLog)

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	tracerOptions := tracing.Options{
		EnableJaeger:   *enableJaeger,
		JaegerEndpoint: *jaegerEndpoint,
	}
	tp, err := tracing.InitTracerProvider(log, tracerOptions)
	if err != nil {
		log.Error(err, "failed to initialize tracing")
		os.Exit(1)
	}

	defer tracing.Shutdown(ctx, log, tp)

	ctx, span := tracing.StartNewSpan(ctx, "main")
	defer span.End()

	// Get the configs
	configs, err := lib.GetConfig(*settingsFile)
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
		Log:                  log.WithName("parser"),
	}

	ruleSet, needProviders, err := parser.LoadRules(*rulesFile)
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

	rulesets := engine.RunRules(ctx, ruleSet)
	engine.Stop()

	for _, provider := range needProviders {
		provider.Stop()
	}

	sort.SliceStable(rulesets, func(i, j int) bool {
		return rulesets[i].Name < rulesets[j].Name
	})

	// Write results out to CLI
	b, _ := yaml.Marshal(rulesets)
	if *errorOnViolations && len(rulesets) != 0 {
		fmt.Printf("%s", string(b))
		os.Exit(EXIT_ON_ERROR_CODE)
	}

	os.WriteFile(*outputViolations, b, 0644)
}

func validateFlags() error {
	_, err := os.Stat(*settingsFile)
	if err != nil {
		return fmt.Errorf("unable to find provider settings file")
	}

	_, err = os.Stat(*rulesFile)
	if err != nil {
		return fmt.Errorf("unable to find rule path or file")
	}

	return nil
}
