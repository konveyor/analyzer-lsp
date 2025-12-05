package reporter

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/konveyor/analyzer-lsp/progress"
)

// JSONReporter writes progress events as newline-delimited JSON (NDJSON).
//
// JSONReporter serializes each event to a single JSON line, creating a stream
// of structured data suitable for machine consumption. This format is ideal for:
//   - Log aggregation systems (Elasticsearch, Splunk, etc.)
//   - External monitoring tools
//   - Programmatic analysis of progress data
//   - CI/CD pipelines that need to parse progress
//
// Each line is a complete, valid JSON object that can be parsed independently,
// making the format robust to interruptions and easy to stream.
//
// The reporter is thread-safe and uses a mutex to ensure each JSON line is
// written atomically without interleaving.
//
// Example output:
//
//	{"timestamp":"2024-10-29T17:06:14Z","stage":"provider_init","message":"Initializing nodejs provider"}
//	{"timestamp":"2024-10-29T17:06:17Z","stage":"provider_init","message":"Provider nodejs ready"}
//	{"timestamp":"2024-10-29T17:06:22Z","stage":"rule_parsing","total":10}
//	{"timestamp":"2024-10-29T17:06:22Z","stage":"rule_execution","current":1,"total":10,"percent":10}
//	{"timestamp":"2024-10-29T17:06:26Z","stage":"complete"}
//
// Usage:
//
//	reporter := reporter.NewJSONReporter(os.Stderr)
//	prog, _ := progress.New(
//	    progress.WithReporters(reporter),
//	)
type JSONReporter struct {
	writer io.Writer
	mu     sync.Mutex
}

// NewJSONReporter creates a new JSON progress reporter that writes to w.
//
// The writer can be os.Stdout, os.Stderr, a file, or any io.Writer.
// Each progress event will be written as a single JSON line (NDJSON format).
//
// Example:
//
//	// Stderr output
//	reporter := reporter.NewJSONReporter(os.Stderr)
//
//	// File output for later processing
//	f, _ := os.Create("progress.ndjson")
//	defer f.Close()
//	reporter := reporter.NewJSONReporter(f)
func NewJSONReporter(w io.Writer) *JSONReporter {
	return &JSONReporter{
		writer: w,
	}
}

// Report writes a progress event as a JSON line.
//
// The event is marshaled to JSON and written with a newline, creating NDJSON
// format. All fields of the Event struct are included in the output.
//
// If the event's Timestamp is zero, it will be set to the current time before
// marshaling. The Percent field is auto-calculated if not set.
//
// Errors during JSON marshaling or writing are silently ignored to avoid
// disrupting the analysis. In production use, consider wrapping the writer
// with error handling if you need to detect write failures.
//
// This method is safe for concurrent use.
func (j *JSONReporter) Report(event progress.Event) {
	j.mu.Lock()
	defer j.mu.Unlock()

	// Normalize event (set timestamp, calculate percent)
	normalize(&event)

	// Marshal and write
	data, err := json.Marshal(event)
	if err != nil {
		return // Silently skip errors to avoid disrupting analysis
	}
	fmt.Fprintln(j.writer, string(data))
}
