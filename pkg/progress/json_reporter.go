package progress

import (
	"encoding/json"
	"io"
	"sync"
	"time"
)

// JSONReporter writes progress events as JSON lines
type JSONReporter struct {
	writer io.Writer
	mu     sync.Mutex
}

// NewJSONReporter creates a new JSON progress reporter
func NewJSONReporter(w io.Writer) *JSONReporter {
	return &JSONReporter{
		writer: w,
	}
}

// Report writes a progress event as a JSON line
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
