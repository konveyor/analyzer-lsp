package progress

// NoopReporter is a no-op implementation of Reporter that discards all events.
//
// This is the default reporter when progress reporting is not explicitly configured,
// ensuring zero overhead when the feature is disabled. All Report calls are no-ops
// and will be optimized away by the compiler in many cases.
//
// NoopReporter is used automatically by Progress when New() is called without
// any reporters specified via WithReporters().
//
// Example:
//
//	// Explicitly create a no-op reporter (usually unnecessary)
//	reporter := progress.NewNoopReporter()
//	reporter.Report(event) // Does nothing
//
//	// More common: Progress uses it automatically
//	prog, _ := progress.New() // Uses NoopReporter by default
type NoopReporter struct{}

// NewNoopReporter creates a new no-op progress reporter.
//
// This is used as the default reporter when --progress-output is not specified,
// ensuring that progress reporting has zero overhead when disabled.
func NewNoopReporter() *NoopReporter {
	return &NoopReporter{}
}

// Report discards the event without any action.
//
// This method is intentionally empty and will be optimized away by the compiler
// in most cases, ensuring zero runtime overhead.
func (n *NoopReporter) Report(event Event) {
	// Intentionally empty - no-op implementation
}
