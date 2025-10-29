package progress

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// TextReporter writes progress events as human-readable text
type TextReporter struct {
	writer io.Writer
	mu     sync.Mutex
}

// NewTextReporter creates a new text progress reporter
func NewTextReporter(w io.Writer) *TextReporter {
	return &TextReporter{
		writer: w,
	}
}

// Report writes a progress event as human-readable text
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
