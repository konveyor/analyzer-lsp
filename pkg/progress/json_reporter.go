package progress

import (
	"encoding/json"
	"io"
	"sync"
	"time"
)

// JSONReporter writes progress events as newline-delimited JSON (NDJSON).
//
// Each event is marshaled to JSON and written as a single line, making the
// output suitable for streaming consumption by other tools. This reporter is
// thread-safe and suitable for concurrent use.
//
// Example output:
//
//	{"timestamp":"2024-10-29T17:06:14Z","stage":"provider_init","message":"Initializing nodejs provider"}
//	{"timestamp":"2024-10-29T17:06:17Z","stage":"provider_init","message":"Provider nodejs ready"}
//	{"timestamp":"2024-10-29T17:06:22Z","stage":"rule_execution","current":1,"total":10,"percent":10}
type JSONReporter struct {
	writer io.Writer
	mu     sync.Mutex
}

// NewJSONReporter creates a new JSON progress reporter that writes to w.
//
// The writer can be os.Stdout, os.Stderr, a file, or any io.Writer.
// Each progress event will be written as a single JSON line.
//
// Example:
//
//	reporter := progress.NewJSONReporter(os.Stderr)
//	reporter.Report(progress.ProgressEvent{
//	    Stage: progress.StageRuleExecution,
//	    Current: 5,
//	    Total: 10,
//	    Percent: 50.0,
//	})
func NewJSONReporter(w io.Writer) *JSONReporter {
	return &JSONReporter{
		writer: w,
	}
}

// Report writes a progress event as a JSON line.
//
// If the event's Timestamp is zero, it will be set to the current time.
// Errors during JSON marshaling or writing are silently ignored to avoid
// disrupting the analysis.
//
// This method is safe for concurrent use.
func (j *JSONReporter) Report(event ProgressEvent) {
	j.mu.Lock()
	defer j.mu.Unlock()

	// Set timestamp if not already set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Marshal and write
	data, err := json.Marshal(event)
	if err != nil {
		return // Silently skip errors to avoid disrupting analysis
	}

	j.writer.Write(data)
	j.writer.Write([]byte("\n"))
}
