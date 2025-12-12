package konveyor

import (
	"github.com/spf13/cobra"
)

// AnalyzerConfig holds configuration values for creating an Analyzer.
// This type is designed to work with cobra commands, making it easy
// to bind flags and convert them to AnalyzerOptions.
//
// Example usage with cobra:
//
//	config := &konveyor.AnalyzerConfig{}
//	cmd := &cobra.Command{
//	    Use: "analyze",
//	    Run: func(cmd *cobra.Command, args []string) {
//	        analyzer, err := konveyor.NewAnalyzer(config.ToOptions()...)
//	        if err != nil {
//	            log.Fatal(err)
//	        }
//	        defer analyzer.Stop()
//	        // ... run analysis
//	    },
//	}
//	config.AddFlags(cmd)
type AnalyzerConfig struct {
	// ProviderSettings is the path to the provider configuration file.
	// Required for most analysis workflows.
	ProviderSettings string

	// Rules is a list of paths to rule files or directories.
	// Can be specified multiple times to include multiple rule sources.
	Rules []string

	// LabelSelector filters rules by label expressions.
	// Example: "konveyor.io/target=quarkus"
	LabelSelector string

	// DepLabelSelector filters dependency rules by label expressions.
	DepLabelSelector string

	// IncidentSelector filters incidents by custom variable expressions.
	// Example: "(!package=io.konveyor)"
	IncidentSelector string

	// IncidentLimit sets the maximum number of incidents to report per rule.
	// 0 means unlimited.
	IncidentLimit int

	// CodeSnipLimit sets the maximum number of characters in code snippets.
	// 0 means unlimited.
	CodeSnipLimit int

	// ContextLines sets the number of context lines around code snippets.
	ContextLines int

	// AnalysisMode sets the analysis mode: "full" or "source-only".
	// Empty string uses the default mode.
	AnalysisMode string

	// DisableDependencyRules disables dependency analysis rules.
	DisableDependencyRules bool
}

// AddFlags adds all configuration flags to the given cobra command.
// This allows easy integration with cobra-based CLIs.
//
// Example:
//
//	config := &konveyor.AnalyzerConfig{}
//	rootCmd := &cobra.Command{Use: "analyzer"}
//	config.AddFlags(rootCmd)
func (c *AnalyzerConfig) AddFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&c.ProviderSettings, "provider-settings", "", "Path to provider settings file (required)")
	cmd.Flags().StringSliceVar(&c.Rules, "rules", []string{}, "Paths to rule files or directories (can be specified multiple times)")
	cmd.Flags().StringVar(&c.LabelSelector, "label-selector", "", "Filter rules by label selector (e.g., 'konveyor.io/target=quarkus')")
	cmd.Flags().StringVar(&c.DepLabelSelector, "dep-label-selector", "", "Filter dependency rules by label selector")
	cmd.Flags().StringVar(&c.IncidentSelector, "incident-selector", "", "Filter incidents by selector expression")
	cmd.Flags().IntVar(&c.IncidentLimit, "incident-limit", 0, "Maximum incidents per rule (0 = unlimited)")
	cmd.Flags().IntVar(&c.CodeSnipLimit, "code-snip-limit", 0, "Maximum characters in code snippets (0 = unlimited)")
	cmd.Flags().IntVar(&c.ContextLines, "context-lines", 0, "Number of context lines around code snippets")
	cmd.Flags().StringVar(&c.AnalysisMode, "analysis-mode", "", "Analysis mode: 'full' or 'source-only'")
	cmd.Flags().BoolVar(&c.DisableDependencyRules, "disable-dep-rules", false, "Disable dependency analysis rules")
}

// ToOptions converts the AnalyzerConfig to a slice of AnalyzerOptions.
// All configuration values are applied, and validation is performed
// by the individual option functions when passed to NewAnalyzer().
//
// Example:
//
//	config := &konveyor.AnalyzerConfig{
//	    ProviderSettings: "provider_settings.json",
//	    Rules: []string{"rules/"},
//	    IncidentLimit: 1500,
//	}
//	analyzer, err := konveyor.NewAnalyzer(config.ToOptions()...)
//	if err != nil {
//	    // Handle validation errors from option functions
//	    log.Fatal(err)
//	}
func (c *AnalyzerConfig) ToOptions() []AnalyzerOption {
	options := []AnalyzerOption{
		WithProviderConfigFilePath(c.ProviderSettings),
		WithRuleFilepaths(c.Rules),
		WithLabelSelector(c.LabelSelector),
		WithDepLabelSelector(c.DepLabelSelector),
		WithIncidentSelector(c.IncidentSelector),
		WithIncidentLimit(c.IncidentLimit),
		WithCodeSnipLimit(c.CodeSnipLimit),
		WithContextLinesLimit(c.ContextLines),
		WithAnalysisMode(c.AnalysisMode),
	}

	if c.DisableDependencyRules {
		options = append(options, WithDependencyRulesDisabled())
	}

	return options
}
