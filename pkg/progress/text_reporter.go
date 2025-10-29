package progress

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// TextReporter writes progress events as human-readable text with timestamps.
//
// Each event is formatted with a timestamp prefix and stage-appropriate message.
// This reporter is ideal for terminal output and log files where human readability
// is important. It is thread-safe and suitable for concurrent use.
//
// Example output:
//
//	[17:06:14] Provider: Initializing nodejs provider
//	[17:06:17] Provider: Provider nodejs ready
//	[17:06:22] Rule: Starting rule execution: 10 rules to process
//	[17:06:22] Rule: patternfly-v5-to-v6-charts-00000
//	[17:06:26] Analysis complete!
type TextReporter struct {
	writer io.Writer
	mu     sync.Mutex
}

// NewTextReporter creates a new text progress reporter that writes to w.
//
// The writer is typically os.Stderr for terminal output, but can be any io.Writer
// including files or custom writers.
//
// Example:
//
//	reporter := progress.NewTextReporter(os.Stderr)
//	reporter.Report(progress.ProgressEvent{
//	    Stage: progress.StageRuleExecution,
//	    Message: "Processing rule-001",
//	})
func NewTextReporter(w io.Writer) *TextReporter {
	return &TextReporter{
		writer: w,
	}
}

// Report writes a progress event as human-readable text.
//
// The output format varies by stage:
//   - Provider init: "[HH:MM:SS] Provider: <message>"
//   - Rule execution: "[HH:MM:SS] Rule: <rule-id>" or "[HH:MM:SS] Processing rules: X/Y (Z%)"
//   - Complete: "[HH:MM:SS] Analysis complete!"
//
// If the event's Timestamp is zero, it will be set to the current time.
// This method is safe for concurrent use.
func (t *TextReporter) Report(event ProgressEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Set timestamp if not already set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	var output string

	switch event.Stage {
	case StageInit:
		output = fmt.Sprintf("[%s] Initializing...\n", event.Timestamp.Format("15:04:05"))
	case StageProviderInit:
		if event.Message != "" {
			output = fmt.Sprintf("[%s] Provider: %s\n", event.Timestamp.Format("15:04:05"), event.Message)
		}
	case StageRuleParsing:
		if event.Total > 0 {
			output = fmt.Sprintf("[%s] Loaded %d rules\n", event.Timestamp.Format("15:04:05"), event.Total)
		}
	case StageRuleExecution:
		if event.Total > 0 {
			output = fmt.Sprintf("[%s] Processing rules: %d/%d (%.1f%%)\n",
				event.Timestamp.Format("15:04:05"),
				event.Current,
				event.Total,
				event.Percent)
		}
		if event.Message != "" {
			output = fmt.Sprintf("[%s] Rule: %s\n", event.Timestamp.Format("15:04:05"), event.Message)
		}
	case StageDependencyAnalysis:
		output = fmt.Sprintf("[%s] Analyzing dependencies...\n", event.Timestamp.Format("15:04:05"))
	case StageComplete:
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
