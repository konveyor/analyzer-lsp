package progress

import (
	"sync"
	"sync/atomic"
	"time"
)

// ThrottledReporter wraps a ProgressReporter with intelligent throttling and optional streaming.
//
// It ensures progress updates don't overwhelm the reporting system by:
//   - Throttling updates to a configured interval (default 500ms)
//   - Always reporting first and last events regardless of timing
//   - Optionally streaming events to a channel for real-time consumers
//
// This reporter is designed to be reused across different providers and contexts.
// It's safe for concurrent use.
//
// Example usage:
//
//	baseReporter := progress.NewTextReporter(os.Stderr)
//	throttled := progress.NewThrottledReporter("java", baseReporter)
//
//	// Optional: enable streaming for gRPC or other consumers
//	eventChan := make(chan ProgressEvent, 100)
//	throttled.EnableStreaming(eventChan)
//
//	// Report progress - automatically throttled
//	for i := 0; i < total; i++ {
//	    throttled.Report(ProgressEvent{
//	        Stage: StageProviderPrepare,
//	        Current: i,
//	        Total: total,
//	    })
//	}
type ThrottledReporter struct {
	// stageName is the default stage name for events (e.g., "provider_prepare")
	stageName string

	// reporter is the underlying reporter that receives throttled events
	reporter ProgressReporter

	// Throttling configuration and state
	throttleInterval time.Duration
	lastReportTime   time.Time
	lastReported     int // Last current value we reported
	reportMutex      sync.Mutex

	// Optional streaming
	streamEnabled atomic.Bool
	streamChan    chan<- ProgressEvent
	streamMutex   sync.RWMutex
}

// NewThrottledReporter creates a new throttled reporter with default 500ms interval.
//
// Parameters:
//   - stageName: Default stage name for events (e.g., "provider_prepare")
//   - reporter: Underlying reporter to receive throttled events (can be nil for stream-only mode)
//
// The reporter will automatically:
//   - Report the first event (current == 1)
//   - Report the last event (current == total)
//   - Throttle intermediate events to once per 500ms
func NewThrottledReporter(stageName string, reporter ProgressReporter) *ThrottledReporter {
	return &ThrottledReporter{
		stageName:        stageName,
		reporter:         reporter,
		throttleInterval: 500 * time.Millisecond,
	}
}

// NewThrottledReporterWithInterval creates a throttled reporter with custom throttle interval.
func NewThrottledReporterWithInterval(stageName string, reporter ProgressReporter, interval time.Duration) *ThrottledReporter {
	return &ThrottledReporter{
		stageName:        stageName,
		reporter:         reporter,
		throttleInterval: interval,
	}
}

// Report sends a progress event through the throttled reporter.
//
// The event will be:
//   - Sent immediately if it's the first event (Current == 1)
//   - Sent immediately if it's the last event (Current == Total)
//   - Sent immediately if throttleInterval has elapsed since last report
//   - Dropped otherwise (to avoid overwhelming the reporter)
//
// If streaming is enabled, the event will also be sent to the stream channel
// using a non-blocking send (dropped if channel is full).
//
// The event will be normalized (timestamp set, percent calculated) before delivery.
func (t *ThrottledReporter) Report(event ProgressEvent) {
	// Normalize the event (set timestamp, calculate percent)
	event.normalize()

	// If stage is not set, use the default
	if event.Stage == "" {
		event.Stage = Stage(t.stageName)
	}

	t.reportMutex.Lock()
	now := time.Now()
	timeSinceLastReport := now.Sub(t.lastReportTime)
	current := event.Current
	total := event.Total

	// Determine if we should report based on throttling rules
	isFirstEvent := current == 1 || t.lastReported == 0
	isLastEvent := current == total && total > 0
	intervalElapsed := timeSinceLastReport >= t.throttleInterval

	shouldReport := isFirstEvent || isLastEvent || intervalElapsed

	if shouldReport {
		t.lastReportTime = now
		t.lastReported = current
		t.reportMutex.Unlock()

		// Send to underlying reporter if configured
		if t.reporter != nil {
			t.reporter.Report(event)
		}

		// Send to stream if enabled
		if t.streamEnabled.Load() {
			t.sendToStream(event)
		}
	} else {
		t.reportMutex.Unlock()
	}
}

// EnableStreaming enables event streaming to the provided channel.
//
// Events will be sent using non-blocking sends, so a full or closed channel
// will not block progress reporting. Configure the channel with adequate
// buffering for your use case.
//
// Example:
//
//	eventChan := make(chan ProgressEvent, 100)
//	reporter.EnableStreaming(eventChan)
//
//	go func() {
//	    for event := range eventChan {
//	        // Process event
//	    }
//	}()
func (t *ThrottledReporter) EnableStreaming(ch chan<- ProgressEvent) {
	t.streamMutex.Lock()
	t.streamChan = ch
	t.streamEnabled.Store(true)
	t.streamMutex.Unlock()
}

// DisableStreaming disables event streaming.
// The stream channel will no longer receive events.
func (t *ThrottledReporter) DisableStreaming() {
	t.streamEnabled.Store(false)
	t.streamMutex.Lock()
	t.streamChan = nil
	t.streamMutex.Unlock()
}

// sendToStream sends an event to the stream channel using non-blocking send.
// This ensures we never block progress reporting even if the consumer is slow.
func (t *ThrottledReporter) sendToStream(event ProgressEvent) {
	t.streamMutex.RLock()
	ch := t.streamChan
	t.streamMutex.RUnlock()

	if ch != nil {
		select {
		case ch <- event:
			// Event sent successfully
		default:
			// Channel full or closed, drop the event
		}
	}
}

// Reset resets the throttling state.
// This is useful when reusing the reporter for a new operation.
func (t *ThrottledReporter) Reset() {
	t.reportMutex.Lock()
	t.lastReportTime = time.Time{}
	t.lastReported = 0
	t.reportMutex.Unlock()
}
