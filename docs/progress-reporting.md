# Progress Reporting

The analyzer supports real-time progress reporting to provide visibility into analysis execution. This feature enables downstream consumers to display progress bars, ETAs, and status updates during long-running analyses.

## Table of Contents

- [Overview](#overview)
- [CLI Usage](#cli-usage)
- [Programmatic Integration](#programmatic-integration)
- [Output Formats](#output-formats)
- [Progress Events](#progress-events)
- [Examples](#examples)
- [Best Practices](#best-practices)

## Overview

Progress reporting is:
- **Opt-in**: No behavior change unless explicitly enabled
- **Zero overhead**: Negligible performance impact when disabled
- **Flexible**: Multiple output formats for different use cases
- **Real-time**: Updates emitted as rules are processed

### Key Benefits

- Display progress bars in UIs
- Calculate ETAs for long-running analyses
- Identify slow or problematic rules
- Improve user experience in CI/CD pipelines
- Enable better monitoring and observability

## CLI Usage

### Basic Usage

Enable progress reporting with the `--progress-output` and `--progress-format` flags:

```bash
analyzer \
  --rules rules.yaml \
  --provider-settings settings.json \
  --progress-output=stderr \
  --progress-format=text
```

### CLI Flags

#### `--progress-output`

Where to write progress events. Options:
- `stderr` - Write to standard error (default for interactive use)
- `stdout` - Write to standard output
- `<file-path>` - Write to a file

**Default**: None (progress reporting disabled)

#### `--progress-format`

Format for progress output. Options:
- `text` - Human-readable text format
- `json` - JSON line-delimited format (for programmatic consumption)

**Default**: `text`

### Example Commands

**Human-readable progress to stderr:**
```bash
analyzer --rules rules.yaml --progress-output=stderr --progress-format=text
```

**JSON progress to file:**
```bash
analyzer --rules rules.yaml --progress-output=progress.json --progress-format=json
```

**JSON to stdout, results to file:**
```bash
analyzer --rules rules.yaml \
  --progress-output=stdout \
  --progress-format=json \
  --output-file=results.yaml
```

## Programmatic Integration

### Using the Channel Reporter

For Go programs that want to consume progress events programmatically:

```go
package main

import (
    "context"
    "fmt"

    "github.com/konveyor/analyzer-lsp/engine"
    "github.com/konveyor/analyzer-lsp/progress"
)

func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Create a channel-based reporter
    // The reporter automatically closes when ctx is cancelled
    // Optionally pass a logger to log dropped events
    reporter := progress.NewChannelReporter(ctx, progress.WithLogger(log))

    // Create engine
    eng := engine.CreateRuleEngine(ctx, 10, log)

    // Process events in a goroutine
    go func() {
        for event := range reporter.Events() {
            handleProgressEvent(event)
        }
    }()

    // Run analysis with progress reporter
    results := eng.RunRulesWithOptions(ctx, ruleSets, []engine.RunOption{
        engine.WithProgressReporter(reporter),
    })
}

func handleProgressEvent(event progress.Event) {
    switch event.Stage {
    case progress.StageRuleExecution:
        fmt.Printf("Progress: %d%% (%d/%d)\n",
            int(event.Percent), event.Current, event.Total)
    case progress.StageComplete:
        fmt.Println("Analysis complete!")
    }
}
```

### Visual Progress Bar Example

For a more polished terminal UI with a progress bar:

```go
package main

import (
    "fmt"
    "strings"

    "github.com/konveyor/analyzer-lsp/progress"
)

func displayProgress(reporter *progress.ChannelReporter) {
    for event := range reporter.Events() {
        switch event.Stage {
        case progress.StageInit:
            fmt.Printf("⏳ %s\n", event.Message)

        case progress.StageProviderInit:
            fmt.Printf("🔌 Provider: %s\n", event.Message)

        case progress.StageRuleParsing:
            fmt.Printf("📋 %s\n", event.Message)

        case progress.StageRuleExecution:
            if event.Total > 0 {
                // Draw progress bar
                bar := drawProgressBar(event.Percent, 40)
                fmt.Printf("\r🔍 Processing: %s %3.0f%% (%d/%d) - %s",
                    bar,
                    event.Percent,
                    event.Current,
                    event.Total,
                    event.Message)
            }

        case progress.StageComplete:
            fmt.Printf("\n✅ %s\n", event.Message)
        }
    }
}

func drawProgressBar(percent float64, width int) string {
    filled := int(percent / 100.0 * float64(width))
    if filled > width {
        filled = width
    }

    bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
    return fmt.Sprintf("[%s]", bar)
}
```

### Custom Reporter Implementation

Implement the `ProgressReporter` interface for custom behavior:

```go
type MyCustomReporter struct {
    // Your fields
}

func (r *MyCustomReporter) Report(event progress.Event) {
    // Your custom logic
    // - Send to webhook
    // - Update database
    // - Emit metrics
    // - Update UI
}

// Use with engine
eng := engine.CreateRuleEngine(ctx, 10, log,
    engine.WithProgressReporter(&MyCustomReporter{}),
)
```

### Multiple Reporters

Use multiple reporters simultaneously (planned for future enhancement):

```go
jsonFile, _ := os.Create("progress.json")
jsonReporter := progress.NewJSONReporter(jsonFile)
textReporter := progress.NewTextReporter(os.Stderr)

// Future: MultiReporter
// multiReporter := progress.NewMultiReporter(jsonReporter, textReporter)
```

## Output Formats

### Text Format

Human-readable output suitable for terminal display:

```
[15:04:05] Initializing...
[15:04:05] Provider: java-external-provider
[15:04:06] Loaded 45 rules
[15:04:07] Processing rules: 15/45 (33.3%)
[15:04:07] Rule: java-spring-boot-001
[15:04:08] Processing rules: 30/45 (66.7%)
[15:04:09] Analysis complete!
```

### JSON Format

Machine-readable JSON line-delimited format:

```json
{"timestamp":"2024-10-29T15:04:05Z","stage":"init","message":"Initializing..."}
{"timestamp":"2024-10-29T15:04:05Z","stage":"provider_init","message":"java-external-provider"}
{"timestamp":"2024-10-29T15:04:06Z","stage":"rule_parsing","total":45}
{"timestamp":"2024-10-29T15:04:07Z","stage":"rule_execution","current":15,"total":45,"percent":33.3,"message":"java-spring-boot-001"}
{"timestamp":"2024-10-29T15:04:09Z","stage":"complete","current":45,"total":45,"percent":100.0,"message":"Analysis complete"}
```

Each line is a complete JSON object that can be parsed independently.

## Progress Events

### Event Structure

```go
type ProgressEvent struct {
    Timestamp time.Time              `json:"timestamp"`
    Stage     Stage                  `json:"stage"`
    Message   string                 `json:"message,omitempty"`
    Current   int                    `json:"current,omitempty"`
    Total     int                    `json:"total,omitempty"`
    Percent   float64                `json:"percent,omitempty"`
    Metadata  map[string]interface{} `json:"metadata,omitempty"`
}
```

### Stages

| Stage | Description | Typical Events |
|-------|-------------|----------------|
| `init` | Analysis initialization | Once at start |
| `provider_init` | Provider initialization | Per provider |
| `rule_parsing` | Rule loading and parsing | Once with total count |
| `rule_execution` | Rule processing | Per rule completion |
| `dependency_analysis` | Dependency analysis | Future |
| `complete` | Analysis finished | Once at end |

### Event Flow

Typical sequence of events during analysis:

1. **Initialization** (`init`)
   - Analysis starting

2. **Provider Initialization** (`provider_init`) ✨
   - "Initializing {provider} provider" - Provider starting
   - "Provider {provider} ready" - Provider successfully initialized
   - One pair of events per provider (nodejs, builtin, java, etc.)

3. **Rule Parsing** (`rule_parsing`)
   - Total number of rules discovered

4. **Rule Execution** (`rule_execution`)
   - Initial event: `current=0, total=N, percent=0.0`
   - Per-rule events: `current=X, total=N, percent=X/N*100, message=ruleID`
   - Final event: `current=N, total=N, percent=100.0`

5. **Completion** (`complete`)
   - Analysis finished

### Example Progress Timeline

```
provider_init | Initializing nodejs provider
provider_init | Provider nodejs ready
provider_init | Initializing builtin provider
provider_init | Provider builtin ready
rule_execution | Starting rule execution: 10 rules to process
rule_execution | patternfly-5-to-patternfly-6-charts-00000
rule_execution | patternfly-5-to-patternfly-6-css-variables-00000
...
complete | Analysis complete
```

## Examples

### Example 1: Progress Bar in Shell Script

```bash
#!/bin/bash

# Run analyzer with JSON progress
analyzer --rules rules.yaml \
    --progress-output=stdout \
    --progress-format=json \
    --output-file=results.yaml 2>/dev/null | \
while IFS= read -r line; do
    # Parse JSON and extract percent
    percent=$(echo "$line" | jq -r '.percent // 0')
    current=$(echo "$line" | jq -r '.current // 0')
    total=$(echo "$line" | jq -r '.total // 0')

    if [ "$total" -gt 0 ]; then
        printf "\rProgress: %3.0f%% (%d/%d)" "$percent" "$current" "$total"
    fi
done
echo ""
echo "Analysis complete!"
```

### Example 2: Web Dashboard Integration

```go
package main

import (
    "encoding/json"
    "net/http"

    "github.com/konveyor/analyzer-lsp/progress"
)

// Server-Sent Events endpoint
func progressHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    // Create context that cancels when request is done
    ctx := r.Context()
    reporter := progress.NewChannelReporter(ctx)

    // Start analysis in background
    go runAnalysis(ctx, reporter)

    // Stream events to client
    for event := range reporter.Events() {
        data, _ := json.Marshal(event)
        fmt.Fprintf(w, "data: %s\n\n", data)
        w.(http.Flusher).Flush()
    }
}
```

### Example 3: Prometheus Metrics

```go
package main

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/konveyor/analyzer-lsp/progress"
)

type PrometheusReporter struct {
    rulesProcessed prometheus.Counter
    rulesTotal     prometheus.Gauge
    percentGauge   prometheus.Gauge
}

func (r *PrometheusReporter) Report(event progress.Event) {
    switch event.Stage {
    case progress.StageRuleExecution:
        r.rulesProcessed.Add(1)
        r.rulesTotal.Set(float64(event.Total))
        r.percentGauge.Set(event.Percent)
    }
}
```

### Example 4: CI/CD Integration (GitHub Actions)

```yaml
name: Analysis
on: [push]

jobs:
  analyze:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2

      - name: Run Analysis with Progress
        run: |
          analyzer \
            --rules rules.yaml \
            --progress-output=progress.json \
            --progress-format=json \
            --output-file=results.yaml

          # Extract summary from progress
          jq -r 'select(.stage=="complete") | "✅ Completed: \(.current)/\(.total) rules"' progress.json

      - name: Upload Results
        uses: actions/upload-artifact@v2
        with:
          name: analysis-results
          path: |
            results.yaml
            progress.json
```

## Best Practices

### For CLI Users

1. **Use `stderr` for progress, `stdout` for data**
   ```bash
   analyzer --progress-output=stderr --output-file=results.yaml
   ```

2. **Use JSON format for scripting**
   ```bash
   analyzer --progress-output=stdout --progress-format=json | process_events.sh
   ```

3. **Save progress to file for debugging**
   ```bash
   analyzer --progress-output=progress.json --progress-format=json
   ```

### For Programmatic Integration

1. **Pass a context to ChannelReporter**
   ```go
   ctx, cancel := context.WithCancel(context.Background())
   defer cancel()
   reporter := progress.NewChannelReporter(ctx)
   // Reporter automatically closes when ctx is cancelled

   // Optional: Add a logger to track dropped events
   reporter := progress.NewChannelReporter(ctx, progress.WithLogger(log))
   ```

2. **Handle events in separate goroutine**
   ```go
   go func() {
       for event := range reporter.Events() {
           handleEvent(event)
       }
   }()
   ```

3. **Check for nil reporters**
   ```go
   if r.progressReporter != nil {
       r.progressReporter.Report(event)
   }
   ```

4. **Don't block on event handling**
   - Progress events are sent on a buffered channel
   - Channel reporter drops events if buffer is full
   - Keep event handling lightweight

### Performance Considerations

1. **Minimal overhead when disabled**
   - Zero cost when `--progress-output` is not set
   - Only a nil check in critical path

2. **Efficient when enabled**
   - Buffered channels prevent blocking
   - JSON encoding happens in reporter, not engine
   - Text formatting is minimal

3. **Reasonable update frequency**
   - Updates sent per rule completion
   - For fast rules, consider throttling display updates
   - Current implementation: ~1% granularity is typical

## API Reference

### Package `pkg/progress`

#### Types

```go
type ProgressReporter interface {
    Report(event ProgressEvent)
}

type ProgressEvent struct {
    Timestamp time.Time
    Stage     Stage
    Message   string
    Current   int
    Total     int
    Percent   float64
    Metadata  map[string]interface{}
}

type Stage string
```

#### Constants

```go
const (
    StageInit               Stage = "init"
    StageProviderInit       Stage = "provider_init"
    StageRuleParsing        Stage = "rule_parsing"
    StageRuleExecution      Stage = "rule_execution"
    StageDependencyAnalysis Stage = "dependency_analysis"
    StageComplete           Stage = "complete"
)
```

#### Functions

```go
// Create a no-op reporter (default)
func NewNoopReporter() *NoopReporter

// Create a JSON line-delimited reporter
func NewJSONReporter(w io.Writer) *JSONReporter

// Create a human-readable text reporter
func NewTextReporter(w io.Writer) *TextReporter

// Create a channel-based reporter for programmatic use
// Automatically closes when the context is cancelled
func NewChannelReporter(ctx context.Context, opts ...ChannelReporterOption) *ChannelReporter

// WithLogger adds logging for dropped events
func WithLogger(log logr.Logger) ChannelReporterOption

// Get the events channel (ChannelReporter only)
func (c *ChannelReporter) Events() <-chan ProgressEvent

// Close the reporter (ChannelReporter only)
func (c *ChannelReporter) Close()
```

### Package `engine`

#### Engine Options

```go
// Add progress reporter to engine
func WithProgressReporter(reporter progress.ProgressReporter) Option
```

## Troubleshooting

### No progress output

**Problem**: Progress flags are set but no output appears

**Solutions**:
- Verify `--progress-output` is set (defaults to disabled)
- Check file permissions if writing to a file
- Ensure output isn't being buffered (use `stderr` for immediate output)
- Check that rules are actually being executed (not all skipped)

### Progress events are delayed

**Problem**: Progress bar updates lag behind actual progress

**Solutions**:
- This is expected for very fast rules (events are per-rule)
- Use `stderr` instead of file output for lower latency
- Consider that some rules take much longer than others

### JSON parsing errors

**Problem**: JSON events are malformed or incomplete

**Solutions**:
- Ensure you're reading complete lines (JSON line-delimited format)
- Don't mix progress output with other output streams
- Use `--output-file` to separate results from progress

### Performance degradation

**Problem**: Analysis is slower with progress reporting enabled

**Solutions**:
- Verify overhead is actually from progress (profile first)
- Consider that JSON encoding adds minor overhead
- Ensure event handling isn't blocking (use goroutines)
- Typical overhead should be <1%

## Future Enhancements

Planned improvements to progress reporting:

- **Provider-level progress**: Track initialization and dependency analysis
- **Multi-reporter**: Combine multiple reporters
- **Configurable throttling**: Limit update frequency for very fast analyses
- **Rich metadata**: Include timing, memory usage, rule performance
- **Structured logging integration**: Integrate with existing log system

## Feedback

This is a new feature. If you encounter issues or have suggestions:
- Open an issue on GitHub
- Describe your use case
- Share integration patterns that would be helpful

---

**Version**: Initial release
**Last Updated**: 2024-10-29
