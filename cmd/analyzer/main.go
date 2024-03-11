package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	logrusr "github.com/bombsimon/logrusr/v3"
	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/engine"
	"github.com/konveyor/analyzer-lsp/engine/labels"
	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/parser"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/analyzer-lsp/provider/lib"
	"github.com/konveyor/analyzer-lsp/tracing"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/swaggest/openapi-go/openapi3"
	"go.opentelemetry.io/otel/attribute"
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
	getOpenAPISpec    string
)

func AnalysisCmd() *cobra.Command {
	var errLog logr.Logger

	rootCmd := &cobra.Command{
		Use:   "konveyor-analyzer",
		Short: "Tool for working with konveyor-analyzer",
		PreRunE: func(c *cobra.Command, args []string) error {
			logrusErrLog := logrus.New()
			logrusErrLog.SetOutput(os.Stderr)
			errLog = logrusr.New(logrusErrLog)
			err := validateFlags()
			if err != nil {
				errLog.Error(err, "failed to validate flags")

				return err
			}

			return nil
		},
		Run: func(c *cobra.Command, args []string) {

			logrusLog := logrus.New()
			logrusLog.SetOutput(os.Stdout)
			logrusLog.SetFormatter(&logrus.TextFormatter{})
			// need to do research on mapping in logrusr to level here TODO
			logrusLog.SetLevel(logrus.Level(logLevel))
			log := logrusr.New(logrusLog)

			// This will globally prevent the yaml library from auto-wrapping lines at 80 characters
			yaml.FutureLineWrap()

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
			var err error
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

			ctx, mainSpan := tracing.StartNewSpan(ctx, "main")
			defer mainSpan.End()

			// Get the configs
			configs, err := provider.GetConfig(settingsFile)
			if err != nil {
				errLog.Error(err, "unable to get configuration")
				os.Exit(1)
			}

			engineCtx, engineSpan := tracing.StartNewSpan(ctx, "rule-engine")
			//start up the rule eng
			eng := engine.CreateRuleEngine(engineCtx,
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
			if getOpenAPISpec != "" {
				sc := createOpenAPISchema(providers, log)
				b, err := json.Marshal(sc)
				if err != nil {
					log.Error(err, "unable to create inital schema")
					os.Exit(1)
				}

				err = os.WriteFile(getOpenAPISpec, b, 0644)
				if err != nil {
					log.Error(err, "error writing output file", "file", getOpenAPISpec)
					os.Exit(1) // Treat the error as a fatal error
				}
				os.Exit(0)
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
				initCtx, initSpan := tracing.StartNewSpan(ctx, "init",
					attribute.Key("provider").String(name))
				err := provider.ProviderInit(initCtx)
				if err != nil {
					errLog.Error(err, "unable to init the providers", "provider", name)
					os.Exit(1)
				}
				initSpan.End()
			}

			rulesets := eng.RunRules(ctx, ruleSets, selectors...)
			engineSpan.End()
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
		},
	}

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
	rootCmd.Flags().StringVar(&getOpenAPISpec, "get-openapi-spec", "", "Get the openAPI spec for the rulesets, rules and provider capabilities and put in file passed in.")

	return rootCmd
}

func main() {
	if err := AnalysisCmd().Execute(); err != nil {
		os.Exit(1)
	} else if AnalysisCmd().Flags().Changed("help") {
		return
	}
}

func validateFlags() error {
	_, err := os.Stat(settingsFile)
	if err != nil {
		return fmt.Errorf("unable to find provider settings file")
	}

	if getOpenAPISpec == "" {
		for _, f := range rulesFile {
			_, err = os.Stat(f)
			if err != nil {
				return fmt.Errorf("unable to find rule path or file")
			}
		}
	}
	m := provider.AnalysisMode(strings.ToLower(analysisMode))
	if analysisMode != "" && !(m == provider.FullAnalysisMode || m == provider.SourceOnlyAnalysisMode) {
		return fmt.Errorf("must select one of %s or %s for analysis mode", provider.FullAnalysisMode, provider.SourceOnlyAnalysisMode)
	}

	return nil
}

func createOpenAPISchema(providers map[string]provider.InternalProviderClient, log logr.Logger) openapi3.Spec {

	// in the future loop and build the openapi spec here:
	spec, err := parser.CreateSchema()
	if err != nil {
		log.Error(err, "unable to create inital schema")
		os.Exit(1)
	}

	AndOrRefRuleRef := []openapi3.SchemaOrRef{}
	for provName, prov := range providers {
		cap := prov.Capabilities()
		for _, c := range cap {
			spec.MapOfSchemaOrRefValues[fmt.Sprintf("%s.%s", provName, c.Name)] = openapi3.SchemaOrRef{
				Schema: &openapi3.Schema{
					Type: &provider.SchemaTypeObject,
					Properties: map[string]openapi3.SchemaOrRef{
						fmt.Sprintf("%s.%s", provName, c.Name): {
							Schema: c.Input.Schema,
						},
						"from": {
							Schema: &openapi3.Schema{
								Type: &provider.SchemaTypeString,
							},
						},
						"as": {
							Schema: &openapi3.Schema{
								Type: &provider.SchemaTypeString,
							},
						},
						"ignore": {
							Schema: &openapi3.Schema{
								Type: &provider.SchemaTypeBool,
							},
						},
						"not": {
							Schema: &openapi3.Schema{
								Type: &provider.SchemaTypeBool,
							},
						},
					},
				},
			}
			AndOrRefRuleRef = append(AndOrRefRuleRef, openapi3.SchemaOrRef{
				SchemaReference: &openapi3.SchemaReference{
					Ref: fmt.Sprintf("#/components/schemas/%s.%s", provName, c.Name),
				},
			})
		}
	}

	AndOrRefRuleRef = append(AndOrRefRuleRef, openapi3.SchemaOrRef{
		SchemaReference: &openapi3.SchemaReference{
			Ref: "#/components/schemas/and",
		},
	})
	AndOrRefRuleRef = append(AndOrRefRuleRef, openapi3.SchemaOrRef{
		SchemaReference: &openapi3.SchemaReference{
			Ref: "#/components/schemas/or",
		},
	})
	spec.MapOfSchemaOrRefValues["and"] = openapi3.SchemaOrRef{
		Schema: &openapi3.Schema{
			Type: &provider.SchemaTypeObject,
			Properties: map[string]openapi3.SchemaOrRef{
				"and": {
					Schema: &openapi3.Schema{
						Type: &provider.SchemaTypeArray,
						Items: &openapi3.SchemaOrRef{
							Schema: &openapi3.Schema{
								Type:  &provider.SchemaTypeObject,
								OneOf: AndOrRefRuleRef,
							},
						},
					},
				},
			},
		},
	}
	spec.MapOfSchemaOrRefValues["or"] = openapi3.SchemaOrRef{
		Schema: &openapi3.Schema{
			Type: &provider.SchemaTypeObject,
			Properties: map[string]openapi3.SchemaOrRef{
				"or": {
					Schema: &openapi3.Schema{
						Type: &provider.SchemaTypeArray,
						Items: &openapi3.SchemaOrRef{
							Schema: &openapi3.Schema{
								Type:  &provider.SchemaTypeObject,
								OneOf: AndOrRefRuleRef,
							},
						},
					},
				},
			},
		},
	}

	spec.MapOfSchemaOrRefValues["rule"].Schema.Properties["when"] = openapi3.SchemaOrRef{
		Schema: &openapi3.Schema{
			Type:  &provider.SchemaTypeObject,
			OneOf: AndOrRefRuleRef,
		},
	}
	sc := openapi3.Spec{
		Components: &openapi3.Components{
			Schemas: &spec,
		},
		Openapi: "3.0.0",
		Info: openapi3.Info{
			Title:   "Konveyor API",
			Version: "1.0.0",
		},
	}

	return sc
}
