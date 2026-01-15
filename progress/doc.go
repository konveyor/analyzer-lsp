// Package progress provides progress reporting functionality for long-running operations
// in analyzer-lsp providers.
//
// The package includes:
//
//   - ProgressReporter interface for pluggable progress reporting
//   - TextReporter for console/stderr output
//   - ThrottledReporter for intelligent throttling and streaming
//
// # Basic Usage
//
// For simple console output:
//
//	reporter := progress.NewTextReporter(os.Stderr)
//	reporter.Report(progress.ProgressEvent{
//	    Stage: progress.StageProviderPrepare,
//	    Message: "Preparing Java provider",
//	    Current: 50,
//	    Total: 100,
//	})
//
// # Throttled Reporting
//
// To avoid overwhelming consumers with too many updates, use ThrottledReporter:
//
//	baseReporter := progress.NewTextReporter(os.Stderr)
//	throttled := progress.NewThrottledReporter("java", baseReporter)
//
//	// Reports are automatically throttled to 500ms intervals
//	// First and last events are always reported regardless of timing
//	for i := 1; i <= total; i++ {
//	    throttled.Report(progress.ProgressEvent{
//	        Current: i,
//	        Total: total,
//	    })
//	}
//
// # Streaming for GRPC
//
// ThrottledReporter supports dual-mode operation for GRPC streaming:
//
//	throttled := progress.NewThrottledReporter("java", textReporter)
//
//	// Enable streaming to a channel
//	eventChan := make(chan progress.ProgressEvent, 100)
//	throttled.EnableStreaming(eventChan)
//
//	// Events are sent to both the text reporter AND the channel
//	// The channel uses non-blocking sends, so slow consumers don't block progress
//
//	// When done:
//	throttled.DisableStreaming()
//	close(eventChan)
//
// # Thread Safety
//
// All reporters are safe for concurrent use. ThrottledReporter uses fine-grained
// locking to minimize contention and ensure non-blocking operation.
package progress
