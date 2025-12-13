# Konveyor Package

The `konveyor` package provides a high-level, programmatic API for running analysis with the Konveyor analyzer. It abstracts the complexity of managing providers, parsing rules, and executing analysis into a clean, fluent interface.

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Core Concepts](#core-concepts)
- [Option Validation](#option-validation)
- [API Reference](#api-reference)
- [Usage Examples](#usage-examples)
- [Advanced Topics](#advanced-topics)

## Overview

The konveyor package simplifies the analyzer workflow into three main phases:

1. **Initialization** - Create an analyzer with configuration
2. **Rule Parsing** - Parse rules and identify needed providers
3. **Provider Setup** - Initialize and prepare providers
4. **Execution** - Run analysis and get results

## Quick Start

Here's a minimal example to get started:

```go
import (
    "context"
    "github.com/konveyor/analyzer-lsp/konveyor"
)

func main() {
    // Create an analyzer
    analyzer, err := konveyor.NewAnalyzer(
        konveyor.WithProviderConfigFilePath("provider_settings.json"),
        konveyor.WithRuleFilepaths([]string{"rules/"}),
        konveyor.WithLogger(log),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer analyzer.Stop()

    // Parse rules
    _, err = analyzer.ParseRules()
    if err != nil {
        log.Fatal(err)
    }

    // Start providers
    err = analyzer.ProviderStart()
    if err != nil {
        log.Fatal(err)
    }

    // Run analysis
    results := analyzer.Run()

    // Process results...
}
```

## Core Concepts

### Analyzer

The `Analyzer` interface is the main entry point for running analysis. It manages the entire lifecycle:

```go
type Analyzer interface {
    ParseRules(...string) (Rules, error)
    ProviderStart() error
    Run(options ...EngineOption) []v1.RuleSet
    GetProviders(...Filter) []Provider
    GetProviderForLanguage(language string) (Provider, bool)
    GetDependencies(outputFilePath string, tree bool) error
    RuleLabels() []string
    RulesetFilepaths() map[string]string
    Stop() error
}
```

### Workflow Phases

#### 1. Initialization

Create an analyzer instance with configuration options:

```go
analyzer, err := konveyor.NewAnalyzer(
    konveyor.WithLogger(log),
    konveyor.WithProviderConfigFilePath("provider_settings.json"),
    konveyor.WithRuleFilepaths([]string{"rules/"}),
    konveyor.WithLabelSelector("konveyor.io/target=quarkus"),
    konveyor.WithIncidentLimit(1500),
    konveyor.WithCodeSnipLimit(20),
    konveyor.WithContextLinesLimit(10),
    konveyor.WithAnalysisMode("full"),
)
```

During initialization:
- Provider configurations are loaded from the settings file
- Provider clients are created and started (if they implement `Startable`)
- A rule engine is created with specified options
- Progress reporting is set up

#### 2. Rule Parsing

Parse rule files to determine which providers are needed:

```go
rules, err := analyzer.ParseRules()
// Or parse specific rule paths:
rules, err := analyzer.ParseRules("custom-rules.yaml")
```

The parser:
- Loads rule files from specified paths
- Determines which providers are required
- Identifies provider conditions that need preparation
- Applies dependency label selectors if configured

#### 3. Provider Setup

Initialize providers needed by the parsed rules:

```go
err = analyzer.ProviderStart()
```

This phase:
- Calls `ProviderInit()` on all providers (except builtin)
- Collects additional builtin configurations from providers
- Initializes the builtin provider with all configurations
- Calls `Prepare()` on providers with their required conditions
- Reports progress through the progress reporting system

#### 4. Execution

Run the analysis:

```go
results := analyzer.Run()
// Or with options:
results := analyzer.Run(
    konveyor.WithSelector(customSelector),
    konveyor.WithProgressReporter(reporter),
)
```

The engine:
- Evaluates rules against the codebase
- Collects violations and incidents
- Returns results as RuleSets

## Option Validation

The konveyor package implements comprehensive validation for analyzer options. Validation errors are caught early during `NewAnalyzer()` initialization, preventing runtime issues and providing clear error messages.

### Validated Options

The following options have validation constraints:

#### File Path Options

**`WithRuleFilepaths(rules []string)`**
- Validates: Non-empty array
- Validates: Each individual path is not empty
- Returns error: "rule filepaths cannot be empty" if array is empty
- Returns error: "rule filepath at index X is empty" if any path is empty

**`WithProviderConfigFilePath(path string)`**
- Validates: Non-empty path
- Returns error: "provider config file path cannot be empty"

#### Numeric Limit Options

**`WithIncidentLimit(limit int)`**
- Validates: Non-negative value (>= 0)
- Returns error: "incident limit must be non-negative, got: X"

**`WithCodeSnipLimit(limit int)`**
- Validates: Non-negative value (>= 0)
- Returns error: "code snippet limit must be non-negative, got: X"

**`WithContextLinesLimit(limit int)`**
- Validates: Non-negative value (>= 0)
- Returns error: "context lines limit must be non-negative, got: X"

#### Mode Options

**`WithAnalysisMode(mode string)`**
- Validates: Must be "full", "source-only", or empty string
- Empty string is allowed (uses default mode)
- Returns error: "invalid analysis mode: X (valid values: full or source-only)"

#### Context Options

**`WithContext(ctx context.Context)`**
- Validates: Context must not be nil
- Returns error: "context cannot be nil"
- Note: If not provided, `context.Background()` is used automatically

#### Selector Options

**`WithLabelSelector(selector string)`**
- Selector syntax is validated during analyzer initialization
- Returns error if selector syntax is invalid
- See [labels.md](labels.md) for selector syntax

### Validation Behavior

All validation errors are collected during `NewAnalyzer()` and returned as a combined error:

```go
analyzer, err := konveyor.NewAnalyzer(
    konveyor.WithIncidentLimit(-1),           // Invalid: negative
    konveyor.WithAnalysisMode("invalid"),      // Invalid: unknown mode
    konveyor.WithRuleFilepaths([]string{}),   // Invalid: empty array
)
if err != nil {
    // err contains all validation errors combined
    log.Fatalf("Validation failed: %v", err)
}
```

### Options Without Validation

The following options accept any value and do not perform validation:

- `WithDepLabelSelector(selector)` - Accepts any string
- `WithIncidentSelector(selector)` - Accepts any string
- `WithDependencyRulesDisabled()` - No parameters
- `WithLogger(log)` - Accepts any logger
- `WithProgress(progress)` - Accepts any progress tracker
- `WithReporters(reporters...)` - Accepts any reporters

## API Reference

### Creating an Analyzer

#### `NewAnalyzer(options ...AnalyzerOption) (Analyzer, error)`

Creates a new analyzer instance.

**Options:**

| Option | Description | Validated | Example |
|--------|-------------|-----------|---------|
| `WithLogger(log)` | Set the logger | No | `konveyor.WithLogger(logr.Logger)` |
| `WithProviderConfigFilePath(path)` | Path to provider settings | Yes (non-empty) | `konveyor.WithProviderConfigFilePath("settings.json")` |
| `WithRuleFilepaths(paths)` | Paths to rule files/directories | Yes (non-empty array, non-empty paths) | `konveyor.WithRuleFilepaths([]string{"rules/"})` |
| `WithLabelSelector(selector)` | Filter rules by labels | Yes (syntax) | `konveyor.WithLabelSelector("konveyor.io/target=quarkus")` |
| `WithDepLabelSelector(selector)` | Filter dependencies by labels | No | `konveyor.WithDepLabelSelector("konveyor.io/dep=critical")` |
| `WithIncidentSelector(selector)` | Filter incidents by custom variables | No | `konveyor.WithIncidentSelector("(!package=io.konveyor)")` |
| `WithIncidentLimit(n)` | Max incidents per rule (0=unlimited) | Yes (>= 0) | `konveyor.WithIncidentLimit(1500)` |
| `WithCodeSnipLimit(n)` | Max code snippets per file (0=unlimited) | Yes (>= 0) | `konveyor.WithCodeSnipLimit(20)` |
| `WithContextLinesLimit(n)` | Lines of context in violations | Yes (>= 0) | `konveyor.WithContextLinesLimit(10)` |
| `WithAnalysisMode(mode)` | "full" or "source-only" | Yes (known modes) | `konveyor.WithAnalysisMode("full")` |
| `WithDependencyRulesDisabled()` | Disable dependency analysis | No | `konveyor.WithDependencyRulesDisabled()` |
| `WithContext(ctx)` | Set context for cancellation | Yes (non-nil) | `konveyor.WithContext(ctx)` |
| `WithProgress(progress)` | Custom progress tracker | No | `konveyor.WithProgress(p)` |
| `WithReporters(reporters...)` | Progress reporters | No | `konveyor.WithReporters(r1, r2)` |

See [Option Validation](#option-validation) for details on validation constraints and error messages.

### Parsing Rules

#### `ParseRules(rulePaths ...string) (Rules, error)`

Parses rule files and determines needed providers.

**Parameters:**
- `rulePaths` (optional): Override rule paths set during initialization

**Returns:**
- `Rules` interface for introspection
- Error if parsing fails

**What it does:**
1. Creates a rule parser with provider clients
2. Loads rules from specified paths
3. Collects needed providers and their conditions
4. Reports progress events

### Starting Providers

#### `ProviderStart() error`

Initializes providers needed for the parsed rules.

**Returns:**
- Error if any provider fails to initialize

**What it does:**
1. Calls `ProviderInit()` on non-builtin providers
2. Collects builtin configurations from all providers
3. Initializes builtin provider last with all configs
4. Calls `Prepare()` on providers with their conditions
5. Reports progress events

**Important Notes:**
- Must be called after `ParseRules()`
- Builtin provider is always initialized last
- Progress events are emitted for each provider

### Running Analysis

#### `Run(options ...EngineOption) []v1.RuleSet`

Executes the analysis and returns results.

**Options:**

| Option | Description |
|--------|-------------|
| `WithSelector(selectors...)` | Filter rules to evaluate |
| `WithProgressReporter(reporter)` | Custom progress reporter |
| `WithScope(scope)` | Set analysis scope |

**Returns:**
- Array of RuleSets containing violations

**What it does:**
1. Validates rules and providers are ready
2. Applies engine options
3. Runs rule engine with configured options
4. Sorts results by ruleset name
5. Returns violations

### Provider Introspection

#### `GetProviders(filters ...Filter) []Provider`

Get providers that match filters.

```go
providers := analyzer.GetProviders(
    func(p konveyor.Provider) bool {
        caps, _ := p.Capabilities()
        return hasCapability(caps, "dependency")
    },
)
```

#### `GetProviderForLanguage(language string) (Provider, bool)`

Get provider for a specific language.

```go
provider, found := analyzer.GetProviderForLanguage("java")
if found {
    // Use provider
}
```

### Rule Introspection

#### `RuleLabels() []string`

Get all unique labels from parsed rules.

```go
labels := analyzer.RuleLabels()
// Returns: ["konveyor.io/target=quarkus", "konveyor.io/source=eap"]
```

#### `RulesetFilepaths() map[string]string`

Get mapping of ruleset names to file paths.

```go
paths := analyzer.RulesetFilepaths()
```

### Cleanup

#### `Stop() error`

Stops the engine and all providers, cleans up resources.

```go
defer analyzer.Stop()
```

**What it does:**
1. Stops the rule engine
2. Stops all provider clients
3. Unsubscribes progress collectors
4. Cancels context

## Usage Examples

### Example 1: Basic Analysis

```go
package main

import (
    "log"
    "os"

    "github.com/go-logr/logr"
    "github.com/konveyor/analyzer-lsp/konveyor"
    "gopkg.in/yaml.v2"
)

func main() {
    logger := logr.Discard()

    analyzer, err := konveyor.NewAnalyzer(
        konveyor.WithLogger(logger),
        konveyor.WithProviderConfigFilePath("provider_settings.json"),
        konveyor.WithRuleFilepaths([]string{"rules/"}),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer analyzer.Stop()

    if _, err = analyzer.ParseRules(); err != nil {
        log.Fatal(err)
    }

    if err = analyzer.ProviderStart(); err != nil {
        log.Fatal(err)
    }

    results := analyzer.Run()

    data, _ := yaml.Marshal(results)
    os.WriteFile("output.yaml", data, 0644)
}
```

### Example 2: Using AnalyzerConfig with Cobra

For cobra-based CLIs, use `AnalyzerConfig` to easily bind command-line flags:

```go
package main

import (
    "log"

    "github.com/konveyor/analyzer-lsp/konveyor"
    "github.com/spf13/cobra"
)

func main() {
    config := &konveyor.AnalyzerConfig{}

    cmd := &cobra.Command{
        Use:   "analyze",
        Short: "Run analysis on the codebase",
        RunE: func(cmd *cobra.Command, args []string) error {
            // Create analyzer from config
            analyzer, err := konveyor.NewAnalyzer(config.ToOptions()...)
            if err != nil {
                return err
            }
            defer analyzer.Stop()

            // Parse rules
            if _, err = analyzer.ParseRules(); err != nil {
                return err
            }

            // Start providers
            if err = analyzer.ProviderStart(); err != nil {
                return err
            }

            // Run analysis
            results := analyzer.Run()
            log.Printf("Analysis complete: %d rulesets", len(results))

            return nil
        },
    }

    // Add all analyzer flags to the command
    config.AddFlags(cmd)

    if err := cmd.Execute(); err != nil {
        log.Fatal(err)
    }
}
```

Now users can run: `analyzer --provider-settings=settings.json --rules=rules/ --incident-limit=1500`

### Example 3: With Label Filtering

```go
analyzer, err := konveyor.NewAnalyzer(
    konveyor.WithLogger(log),
    konveyor.WithProviderConfigFilePath("provider_settings.json"),
    konveyor.WithRuleFilepaths([]string{"rules/"}),
    konveyor.WithLabelSelector("konveyor.io/target=quarkus"),
)

// Only rules with matching labels will be evaluated
results := analyzer.Run()
```

### Example 4: Custom Progress Reporting

```go
import (
    "github.com/konveyor/analyzer-lsp/progress"
    "github.com/konveyor/analyzer-lsp/progress/reporter"
)

// Create custom reporter
progressReporter := reporter.NewTextReporter(os.Stdout)

analyzer, err := konveyor.NewAnalyzer(
    konveyor.WithLogger(log),
    konveyor.WithProviderConfigFilePath("provider_settings.json"),
    konveyor.WithRuleFilepaths([]string{"rules/"}),
    konveyor.WithReporters(progressReporter),
)

// Progress will be reported to stdout
```

### Example 5: Error Handling and Validation

```go
analyzer, err := konveyor.NewAnalyzer(
    konveyor.WithProviderConfigFilePath("settings.json"),
    konveyor.WithRuleFilepaths([]string{"rules/"}),
    konveyor.WithIncidentLimit(1500),
)
if err != nil {
    log.Fatalf("Failed to create analyzer: %v", err)
}
defer analyzer.Stop()

_, err = analyzer.ParseRules()
if err != nil {
    log.Fatalf("Failed to parse rules: %v", err)
}

err = analyzer.ProviderStart()
if err != nil {
    log.Fatalf("Failed to start providers: %v", err)
}

results := analyzer.Run()
if len(results) > 0 {
    log.Printf("Found %d rulesets with violations", len(results))
}
```

### Example 6: Dynamic Rule Paths

```go
analyzer, err := konveyor.NewAnalyzer(
    konveyor.WithProviderConfigFilePath("settings.json"),
    // No rules specified initially
)

// Later, parse specific rules
_, err = analyzer.ParseRules("custom-rules/eap7-to-eap8.yaml")

err = analyzer.ProviderStart()
results := analyzer.Run()
```

### Example 7: Context Cancellation

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
defer cancel()

analyzer, err := konveyor.NewAnalyzer(
    konveyor.WithContext(ctx),
    konveyor.WithProviderConfigFilePath("settings.json"),
    konveyor.WithRuleFilepaths([]string{"rules/"}),
)
defer analyzer.Stop()

// Analysis will be cancelled after 30 minutes
```

### Example 8: Source-Only Analysis

```go
analyzer, err := konveyor.NewAnalyzer(
    konveyor.WithProviderConfigFilePath("settings.json"),
    konveyor.WithRuleFilepaths([]string{"rules/"}),
    konveyor.WithAnalysisMode("source-only"),
    konveyor.WithDependencyRulesDisabled(),
)

// Will only analyze source code, skip dependency analysis
```

## Advanced Topics

### Progress Reporting

The analyzer integrates with the progress reporting system. Progress events are emitted during:

- Provider startup (konveyor/types.go:91-96)
- Rule parsing (konveyor/analyzer.go:72-96)
- Provider initialization (konveyor/analyzer.go:123-175)
- Rule execution (handled by engine)

See [progress-reporting.md](progress-reporting.md) for details.

### Provider Configuration

Provider settings are loaded from a JSON file specified via `WithProviderConfigFilePath`. The analyzer:

1. Loads all provider configurations
2. Creates provider clients using `lib.GetProviderClient()`
3. Starts providers that implement `Startable`
4. Manages builtin provider configuration specially

The builtin provider is always initialized last and receives configurations from all other providers.

See [providers.md](providers.md) for provider configuration details.

### Label Selectors

Label selectors filter rules during parsing and execution:

```go
// Filter by target platform
konveyor.WithLabelSelector("konveyor.io/target=quarkus")

// Filter dependencies
konveyor.WithDepLabelSelector("konveyor.io/dep=critical")
```

See [labels.md](labels.md) for label selector syntax.

### Incident Selectors

Incident selectors filter violations based on custom variables:

```go
konveyor.WithIncidentSelector("(!package=io.konveyor.demo.config-utils)")
```

See [incident_selector.md](incident_selector.md) for syntax details.

### Engine Options

When calling `Run()`, you can customize execution:

```go
import "github.com/konveyor/analyzer-lsp/engine"

results := analyzer.Run(
    konveyor.WithScope(engine.Scope{
        // Define scope here
    }),
    konveyor.WithSelector(customSelector),
)
```

### Error Handling

The package uses Go's standard error handling. Errors can occur at each phase:

**Validation errors (during NewAnalyzer):**
- Empty or invalid file paths
- Negative limit values
- Invalid analysis mode
- Nil context
- Invalid label selector syntax

All validation errors are collected and returned as a combined error. See [Option Validation](#option-validation) for details.

**Initialization errors:**
- Invalid provider settings file
- Failed to create provider clients
- Provider startup failures

**Parsing errors:**
- Invalid rule files
- Missing provider capabilities
- Label selector syntax errors

**Provider start errors:**
- ProviderInit failures
- Prepare failures

**Run errors:**
- Rules not parsed (logged, returns nil)
- Providers not started (logged, returns nil)

### Lifecycle Management

The analyzer manages several resources that need cleanup:

```go
analyzer, err := konveyor.NewAnalyzer(...)
if err != nil {
    log.Fatal(err)
}
defer analyzer.Stop() // Always call Stop()
```

`Stop()` is responsible for:
1. Stopping the rule engine (konveyor/analyzer.go:296)
2. Stopping all provider clients (konveyor/analyzer.go:297-299)
3. Unsubscribing progress collectors (konveyor/analyzer.go:300)
4. Cancelling the context (konveyor/analyzer.go:301)

## Reference Implementation

See `cmd/analyzer/main.go` for a complete reference implementation that demonstrates:

- Command-line flag parsing
- Progress reporter creation
- Full analyzer lifecycle
- Error handling
- Output file generation
- Exit code handling

## See Also

- [Rules Documentation](rules.md) - Rule file format and syntax
- [Provider Documentation](providers.md) - Provider configuration
- [Progress Reporting](progress-reporting.md) - Progress reporting system
- [Labels](labels.md) - Label selector syntax
- [Incident Selectors](incident_selector.md) - Incident filtering
- [Output Format](output.md) - Analysis output format
