package reporter

import (
	"fmt"
	"io"
	"sync"

	"github.com/konveyor/analyzer-lsp/progress"
)

// TextReporter writes progress events as human-readable text with timestamps.
//
// TextReporter formats events into timestamped text lines suitable for terminal
// output or log files. Each stage has its own formatting style to provide
// clear, readable progress information.
//
// The reporter is thread-safe and uses a mutex to ensure proper output ordering
// when multiple goroutines report progress concurrently (though Progress's
// architecture typically serializes events through reporter workers).
//
// Example output:
//
//	[17:06:14] Provider: Initializing nodejs provider
//	[17:06:17] Provider: Provider nodejs ready
//	[17:06:22] Loaded 10 rules
//	[17:06:22] Processing rules: 1/10 (10.0%)
//	[17:06:22] Rule: patternfly-v5-to-v6-charts-00000
//	[17:06:26] Analysis complete!
//
// Usage:
//
//	reporter := reporter.NewTextReporter(os.Stderr)
//	prog, _ := progress.New(
//	    progress.WithReporters(reporter),
//	)
type TextReporter struct {
	writer io.Writer
	mu     sync.Mutex
}

// NewTextReporter creates a new text progress reporter that writes to w.
//
// The writer is typically os.Stderr for terminal output, but can be any io.Writer
// including files, buffers, or custom writers.
//
// Example:
//
//	// Terminal output
//	reporter := reporter.NewTextReporter(os.Stderr)
//
//	// File output
//	f, _ := os.Create("progress.log")
//	defer f.Close()
//	reporter := reporter.NewTextReporter(f)
func NewTextReporter(w io.Writer) *TextReporter {
	return &TextReporter{
		writer: w,
	}
}

// Report writes a progress event as human-readable text.
//
// The output format varies by stage:
//   - StageInit: "[HH:MM:SS] Initializing..."
//   - StageProviderInit: "[HH:MM:SS] Provider: <message>"
//   - StageProviderPrepare: "[HH:MM:SS] <message>... X/Y files (Z%)"
//   - StageRuleParsing: "[HH:MM:SS] Loaded X rules"
//   - StageRuleExecution: "[HH:MM:SS] Processing rules: X/Y (Z%)" and/or "[HH:MM:SS] Rule: <rule-id>"
//   - StageDependencyAnalysis: "[HH:MM:SS] Analyzing dependencies..."
//   - StageComplete: "[HH:MM:SS] Analysis complete!"
//
// If the event's Timestamp is zero, it will be set to the current time.
// This method is safe for concurrent use.
func (t *TextReporter) Report(event progress.Event) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Normalize event (set timestamp, calculate percent)
	normalize(&event)

	var output string

	switch event.Stage {
	case progress.StageInit:
		output = fmt.Sprintf("[%s] Initializing...\n", event.Timestamp.Format("15:04:05"))
	case progress.StageProviderInit:
		if event.Message != "" {
			output = fmt.Sprintf("[%s] Provider: %s\n", event.Timestamp.Format("15:04:05"), event.Message)
		}
	case progress.StageProviderPrepare:
		if event.Total > 0 {
			output = fmt.Sprintf("[%s] %s... %d/%d files (%.1f%%)\n",
				event.Timestamp.Format("15:04:05"),
				event.Message,
				event.Current,
				event.Total,
				event.Percent)
		} else if event.Message != "" {
			output = fmt.Sprintf("[%s] %s\n", event.Timestamp.Format("15:04:05"), event.Message)
		}
	case progress.StageRuleParsing:
		if event.Total > 0 {
			output = fmt.Sprintf("[%s] Loaded %d rules\n", event.Timestamp.Format("15:04:05"), event.Total)
		}
	case progress.StageRuleExecution:
		if event.Total > 0 {
			output += fmt.Sprintf("[%s] Processing rules: %d/%d (%.1f%%)\n",
				event.Timestamp.Format("15:04:05"),
				event.Current,
				event.Total,
				event.Percent)
		}
		if event.Message != "" {
			output += fmt.Sprintf("[%s] Rule: %s\n", event.Timestamp.Format("15:04:05"), event.Message)
		}
	case progress.StageDependencyAnalysis:
		output = fmt.Sprintf("[%s] Analyzing dependencies...\n", event.Timestamp.Format("15:04:05"))
	case progress.StageComplete:
		output = fmt.Sprintf("[%s] Analysis complete!\n", event.Timestamp.Format("15:04:05"))
	default:
		if event.Message != "" {
			output = fmt.Sprintf("[%s] %s\n", event.Timestamp.Format("15:04:05"), event.Message)
		}
	}

	if output != "" {
		t.writer.Write([]byte(output))
	}
}
