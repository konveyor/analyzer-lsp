package reporter

import (
	"time"

	"github.com/konveyor/analyzer-lsp/progress"
)

// normalize updates the event with calculated values.
// - Sets Timestamp to now if zero
// - Calculates Percent from Current/Total if Percent is zero and Total > 0
func normalize(e *progress.Event) {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}

	// Auto-calculate percent if not set and we have total
	if e.Percent == 0.0 && e.Total > 0 {
		e.Percent = float64(e.Current) / float64(e.Total) * 100.0
	}
}
