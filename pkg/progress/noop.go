package progress

// NoopReporter is a no-op implementation of ProgressReporter
// This is the default when progress reporting is not configured
type NoopReporter struct{}

// NewNoopReporter creates a new no-op progress reporter
func NewNoopReporter() *NoopReporter {
	return &NoopReporter{}
}

// Report does nothing (no-op implementation)
func (n *NoopReporter) Report(event ProgressEvent) {
	// Intentionally empty
}
