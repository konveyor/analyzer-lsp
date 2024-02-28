package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	logrusr "github.com/bombsimon/logrusr/v3"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/engine/labels"
	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/parser"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/provider/lib"
	"github.com/konveyor/analyzer-lsp/tracing"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

const (
	EXIT_ON_ERROR_CODE = 3
)

var (
	settingsFile      string
	rulesFile         []string
	outputViolations  string
	errorOnViolations bool
	labelSelector     string
	depLabelSelector  string
	incidentSelector  string
	logLevel          int
	enableJaeger      bool
	jaegerEndpoint    string
	limitIncidents    int
	limitCodeSnips    int
	analysisMode      string
	noDependencyRules bool
	contextLines      int

	rootCmd = &cobra.Command{
		Use:   "analyze",
		Short: "Tool for working with analyzer-lsp",
		Run:   func(c *cobra.Command, args []string) {},
	}
)

func init() {
	rootCmd.Flags().StringVar(&settingsFile, "provider-settings", "provider_settings.json", "path to the provider settings")
	rootCmd.Flags().StringArrayVar(&rulesFile, "rules", []string{"rule-example.yaml"}, "filename or directory containing rule files")
	rootCmd.Flags().StringVar(&outputViolations, "output-file", "output.yaml", "filepath to to store rule violations")
	rootCmd.Flags().BoolVar(&errorOnViolations, "error-on-violation", false, "exit with 3 if any violation are found will also print violations to console")
	rootCmd.Flags().StringVar(&labelSelector, "label-selector", "", "an expression to select rules based on labels")
	rootCmd.Flags().StringVar(&depLabelSelector, "dep-label-selector", "", "an expression to select dependencies based on labels. This will filter out the violations from these dependencies as well these dependencies when matching dependency conditions")
	rootCmd.Flags().StringVar(&incidentSelector, "incident-selector", "", "an expression to select incidents based on custom variables. ex: (!package=io.konveyor.demo.config-utils)")
	rootCmd.Flags().IntVar(&logLevel, "verbose", 9, "level for logging output")
	rootCmd.Flags().BoolVar(&enableJaeger, "enable-jaeger", false, "enable tracer exports to jaeger endpoint")
	rootCmd.Flags().StringVar(&jaegerEndpoint, "jaeger-endpoint", "http://localhost:14268/api/traces", "jaeger endpoint to collect tracing data")
	rootCmd.Flags().IntVar(&limitIncidents, "limit-incidents", 1500, "Set this to the limit incidents that a given rule can give, zero means no limit")
	rootCmd.Flags().IntVar(&limitCodeSnips, "limit-code-snips", 20, "limit the number code snippets that are retrieved for a file while evaluating a rule, 0 means no limit")
	rootCmd.Flags().StringVar(&analysisMode, "analysis-mode", "", "select one of full or source-only to tell the providers what to analyize. This can be given on a per provider setting, but this flag will override")
	rootCmd.Flags().BoolVar(&noDependencyRules, "no-dependency-rules", false, "Disable dependency analysis rules")
	rootCmd.Flags().IntVar(&contextLines, "context-lines", 10, "When violation occurs, A part of source code is added to the output, So this flag configures the number of source code lines to be printed to the output.")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		println(err.Error())
	} else if rootCmd.Flags().Changed("help") {
		return
	}

	// This will globally prevent the yaml library from auto-wrapping lines at 80 characters
	yaml.FutureLineWrap()

	logrusLog := logrus.New()
	logrusLog.SetOutput(os.Stdout)
	logrusLog.SetFormatter(&logrus.TextFormatter{})
	// need to do research on mapping in logrusr to level here TODO
	logrusLog.SetLevel(logrus.Level(logLevel))
	log := logrusr.New(logrusLog)

	logrusErrLog := logrus.New()
	logrusErrLog.SetOutput(os.Stderr)
	errLog := logrusr.New(logrusErrLog)

	err := validateFlags()
	if err != nil {
		errLog.Error(err, "failed to validate flags")
		os.Exit(1)
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	selectors := []engine.RuleSelector{}
	if labelSelector != "" {
		selector, err := labels.NewLabelSelector[*engine.RuleMeta](labelSelector, nil)
		if err != nil {
			errLog.Error(err, "failed to create label selector from expression", "selector", labelSelector)
			os.Exit(1)
		}
		selectors = append(selectors, selector)
	}

	var dependencyLabelSelector *labels.LabelSelector[*konveyor.Dep]
	if depLabelSelector != "" {
		dependencyLabelSelector, err = labels.NewLabelSelector[*konveyor.Dep](depLabelSelector, nil)
		if err != nil {
			errLog.Error(err, "failed to create label selector from expression", "selector", labelSelector)
			os.Exit(1)
		}
	}

	tracerOptions := tracing.Options{
		EnableJaeger:   enableJaeger,
		JaegerEndpoint: jaegerEndpoint,
	}
	tp, err := tracing.InitTracerProvider(log, tracerOptions)
	if err != nil {
		errLog.Error(err, "failed to initialize tracing")
		os.Exit(1)
	}

	defer tracing.Shutdown(ctx, log, tp)

	ctx, span := tracing.StartNewSpan(ctx, "main")
	defer span.End()

	// Get the configs
	configs, err := provider.GetConfig(settingsFile)
	if err != nil {
		errLog.Error(err, "unable to get configuration")
		os.Exit(1)
	}

	//start up the rule eng
	eng := engine.CreateRuleEngine(ctx,
		10,
		log,
		engine.WithIncidentLimit(limitIncidents),
		engine.WithCodeSnipLimit(limitCodeSnips),
		engine.WithContextLines(contextLines),
		engine.WithIncidentSelector(incidentSelector),
	)

	providers := map[string]provider.InternalProviderClient{}

	for _, config := range configs {
		config.ContextLines = contextLines
		// IF analsyis mode is set from the CLI, then we will override this for each init config
		if analysisMode != "" {
			inits := []provider.InitConfig{}
			for _, i := range config.InitConfig {
				i.AnalysisMode = provider.AnalysisMode(analysisMode)
				inits = append(inits, i)
			}
			config.InitConfig = inits
		}
		prov, err := lib.GetProviderClient(config, log)
		if err != nil {
			errLog.Error(err, "unable to create provider client")
			os.Exit(1)
		}
		providers[config.Name] = prov
		if s, ok := prov.(provider.Startable); ok {
			if err := s.Start(ctx); err != nil {
				errLog.Error(err, "unable to create provider client")
				os.Exit(1)
			}
		}
	}

	parser := parser.RuleParser{
		ProviderNameToClient: providers,
		Log:                  log.WithName("parser"),
		NoDependencyRules:    noDependencyRules,
		DepLabelSelector:     dependencyLabelSelector,
	}
	ruleSets := []engine.RuleSet{}
	needProviders := map[string]provider.InternalProviderClient{}
	for _, f := range rulesFile {
		internRuleSet, internNeedProviders, err := parser.LoadRules(f)
		if err != nil {
			log.WithValues("fileName", f).Error(err, "unable to parse all the rules for ruleset")
		}
		ruleSets = append(ruleSets, internRuleSet...)
		for k, v := range internNeedProviders {
			needProviders[k] = v
		}
	}
	// Now that we have all the providers, we need to start them.
	for name, provider := range needProviders {
		err := provider.ProviderInit(ctx)
		if err != nil {
			errLog.Error(err, "unable to init the providers", "provider", name)
			os.Exit(1)
		}
	}

	rulesets := eng.RunRules(ctx, ruleSets, selectors...)
	eng.Stop()

	for _, provider := range needProviders {
		provider.Stop()
	}

	sort.SliceStable(rulesets, func(i, j int) bool {
		return rulesets[i].Name < rulesets[j].Name
	})

	// Write results out to CLI
	b, _ := yaml.Marshal(rulesets)
	if errorOnViolations && len(rulesets) != 0 {
		fmt.Printf("%s", string(b))
		os.Exit(EXIT_ON_ERROR_CODE)
	}

	err = os.WriteFile(outputViolations, b, 0644)
	if err != nil {
		errLog.Error(err, "error writing output file", "file", outputViolations)
		os.Exit(1) // Treat the error as a fatal error
	}
}

func validateFlags() error {
	_, err := os.Stat(settingsFile)
	if err != nil {
		return fmt.Errorf("unable to find provider settings file")
	}

	for _, f := range rulesFile {
		_, err = os.Stat(f)
		if err != nil {
			return fmt.Errorf("unable to find rule path or file")
		}
	}
	m := provider.AnalysisMode(strings.ToLower(analysisMode))
	if analysisMode != "" && !(m == provider.FullAnalysisMode || m == provider.SourceOnlyAnalysisMode) {
		return fmt.Errorf("must select one of %s or %s for analysis mode", provider.FullAnalysisMode, provider.SourceOnlyAnalysisMode)
	}

	return nil
}
