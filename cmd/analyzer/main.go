package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	logrusr "github.com/bombsimon/logrusr/v3"
	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/konveyor"
	"github.com/konveyor/analyzer-lsp/parser"
	"github.com/konveyor/analyzer-lsp/progress"
	"github.com/konveyor/analyzer-lsp/progress/collector"
	"github.com/konveyor/analyzer-lsp/progress/reporter"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/swaggest/openapi-go/openapi3"
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
	treeOutput        bool
	depOutputFile     string
	progressOutput    string
	progressFormat    string
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
			// Adding 5 here to move logs to info level
			// setting verbose 1 -> V(2) logs show up
			// setting verbose 2 -> V(3) logs show up
			// setting verbose 3 -> .V(4) I believe show up
			// setting verbose 4 -> .V(5) I believe show up
			logrusLog.SetLevel(logrus.Level(logLevel + 5))
			log := logrusr.New(logrusLog)
			// This will globally prevent the yaml library from auto-wrapping lines at 80 characters
			yaml.FutureLineWrap()

			ctx, cancelFunc := context.WithCancel(context.Background())
			defer cancelFunc()

			progressReporter := createProgressReporter()
			analyzerCollector := collector.New()
			analyzerProgress, err := progress.New(
				progress.WithCollectors(analyzerCollector),
				progress.WithReporters(progressReporter),
				progress.WithContext(ctx),
			)

			analyzer, err := konveyor.NewAnalyzer(
				konveyor.WithLogger(log),
				konveyor.WithProviderConfigFilePath(settingsFile),
				konveyor.WithRuleFilepaths(rulesFile),
				konveyor.WithLabelSelector(labelSelector),
				konveyor.WithDepLabelSelector(depLabelSelector),
				konveyor.WithIncidentSelector(incidentSelector),
				konveyor.WithIncidentLimit(limitIncidents),
				konveyor.WithCodeSnipLimit(limitCodeSnips),
				konveyor.WithContextLinesLimit(contextLines),
				konveyor.WithAnalysisMode(analysisMode),
				konveyor.WithProgress(analyzerProgress),
			)

			if err != nil {
				errLog.Error(err, "Unable to create new Analyzer")
				os.Exit(1)
			}

			// TODO: Add back getting this.
			if getOpenAPISpec != "" {
				sc := []byte{}
				b, err := json.Marshal(sc)
				if err != nil {
					errLog.Error(err, "unable to create inital schema")
					os.Exit(1)
				}

				err = os.WriteFile(getOpenAPISpec, b, 0644)
				if err != nil {
					errLog.Error(err, "error writing output file", "file", getOpenAPISpec)
					os.Exit(1) // Treat the error as a fatal error
				}
				os.Exit(0)
			}

			_, err = analyzer.ParseRules()
			if err != nil {
				errLog.Error(err, "unable to parse rules")
			}

			err = analyzer.ProviderStart()
			if err != nil {
				errLog.Error(err, "unable to start providers")
			}

			wg := sync.WaitGroup{}
			if depOutputFile != "" {
				wg.Add(1)
				go func() {
					defer wg.Done()
					analyzer.GetDependencies(depOutputFile, treeOutput)
				}()
			}

			// All the information should already be set on analyzer
			// We don't need to override.
			rulesets := analyzer.Run()

			sort.SliceStable(rulesets, func(i, j int) bool {
				return rulesets[i].Name < rulesets[j].Name
			})

			// Write results out to CLI
			b, _ := yaml.Marshal(rulesets)
			if errorOnViolations && len(rulesets) != 0 {
				fmt.Printf("%s", string(b))
				os.Exit(EXIT_ON_ERROR_CODE)
			}

			log.Info("writing violations to file", "file", outputViolations)
			err = os.WriteFile(outputViolations, b, 0644)
			if err != nil {
				errLog.Error(err, "error writing output file", "file", outputViolations)
				os.Exit(1) // Treat the error as a fatal error
			}
			wg.Wait()
		},
	}

	rootCmd.Flags().StringVar(&settingsFile, "provider-settings", "provider_settings.json", "path to the provider settings")
	rootCmd.Flags().StringArrayVar(&rulesFile, "rules", []string{"rule-example.yaml"}, "filename or directory containing rule files")
	rootCmd.Flags().StringVar(&outputViolations, "output-file", "output.yaml", "filepath to to store rule violations")
	rootCmd.Flags().BoolVar(&errorOnViolations, "error-on-violation", false, "exit with 3 if any violation are found will also print violations to console")
	rootCmd.Flags().StringVar(&labelSelector, "label-selector", "", "an expression to select rules based on labels")
	rootCmd.Flags().StringVar(&depLabelSelector, "dep-label-selector", "", "an expression to select dependencies based on labels. This will filter out the violations from these dependencies as well these dependencies when matching dependency conditions")
	rootCmd.Flags().StringVar(&incidentSelector, "incident-selector", "", "an expression to select incidents based on custom variables. ex: (!package=io.konveyor.demo.config-utils)")
	rootCmd.Flags().IntVar(&logLevel, "verbose", 0, "level for logging output")
	rootCmd.Flags().BoolVar(&enableJaeger, "enable-jaeger", false, "enable tracer exports to jaeger endpoint")
	rootCmd.Flags().StringVar(&jaegerEndpoint, "jaeger-endpoint", "http://localhost:14268/api/traces", "jaeger endpoint to collect tracing data")
	rootCmd.Flags().IntVar(&limitIncidents, "limit-incidents", 1500, "Set this to the limit incidents that a given rule can give, zero means no limit")
	rootCmd.Flags().IntVar(&limitCodeSnips, "limit-code-snips", 20, "limit the number code snippets that are retrieved for a file while evaluating a rule, 0 means no limit")
	rootCmd.Flags().StringVar(&analysisMode, "analysis-mode", "", "select one of full or source-only to tell the providers what to analyize. This can be given on a per provider setting, but this flag will override")
	rootCmd.Flags().BoolVar(&noDependencyRules, "no-dependency-rules", false, "Disable dependency analysis rules")
	rootCmd.Flags().IntVar(&contextLines, "context-lines", 10, "When violation occurs, A part of source code is added to the output, So this flag configures the number of source code lines to be printed to the output.")
	rootCmd.Flags().StringVar(&getOpenAPISpec, "get-openapi-spec", "", "Get the openAPI spec for the rulesets, rules and provider capabilities and put in file passed in.")
	rootCmd.Flags().BoolVar(&treeOutput, "tree", false, "output dependencies as a tree")
	rootCmd.Flags().StringVar(&depOutputFile, "dep-output-file", "", "path to dependency output file")
	rootCmd.Flags().StringVar(&progressOutput, "progress-output", "", "where to write progress events (stderr, stdout, or file path)")
	rootCmd.Flags().StringVar(&progressFormat, "progress-format", "bar", "format for progress output: bar, text, or json")

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

// createProgressReporter creates a progress reporter based on CLI flags
func createProgressReporter() progress.Reporter {
	// If no output specified, return noop reporter
	if progressOutput == "" {
		return progress.NewNoopReporter()
	}

	// Determine output writer
	var writer *os.File
	switch progressOutput {
	case "stderr":
		writer = os.Stderr
	case "stdout":
		writer = os.Stdout
	default:
		// It's a file path
		file, err := os.Create(progressOutput)
		if err != nil {
			// If we can't create the file, fallback to stderr
			fmt.Fprintf(os.Stderr, "Warning: failed to create progress output file %s: %v\n", progressOutput, err)
			writer = os.Stderr
		} else {
			writer = file
		}
	}

	// Create reporter based on format
	switch progressFormat {
	case "json":
		return reporter.NewJSONReporter(writer)
	case "text":
		return reporter.NewTextReporter(writer)
	case "bar":
		return reporter.NewProgressBarReporter(writer)
	default:
		// Default to progress bar
		return reporter.NewProgressBarReporter(writer)
	}
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
			// Only add output schemas for capabilities that have defined them.
			if c.Output.Schema != nil && len(c.Output.Schema.Properties) != 0 {
				spec.MapOfSchemaOrRefValues[fmt.Sprintf("%s.%s-out", provName, c.Name)] = openapi3.SchemaOrRef{
					Schema: c.Output.Schema,
				}
			}
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
