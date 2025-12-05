package collector

import (
	"math/rand"
	"sync"
	"time"

	"github.com/konveyor/analyzer-lsp/progress"
)

// ThrottledCollector is a collector with intelligent throttling for high-frequency events.
//
// It prevents overwhelming the progress reporting system by:
//   - Throttling updates to a configured interval (default 500ms)
//   - Always forwarding first and last events regardless of timing
//   - Dropping intermediate events that occur too frequently
//
// This collector is ideal for high-frequency progress sources like file processing
// loops where reporting every single file would create excessive overhead.
//
// The collector is safe for concurrent use and can be reused across different
// contexts or providers.
//
// Example usage:
//
//	throttled := collector.NewThrottledCollector("provider_prepare")
//	prog, _ := progress.New(
//	    progress.WithCollectors(throttled),
//	    progress.WithReporters(reporter.NewTextReporter(os.Stderr)),
//	)
//
//	// Report progress for many files - automatically throttled
//	for i := 0; i < 10000; i++ {
//	    throttled.Report(progress.Event{
//	        Stage: progress.StageProviderPrepare,
//	        Current: i,
//	        Total: 10000,
//	    })
//	}
type ThrottledCollector struct {
	// stageName is the default stage name for events (e.g., "provider_prepare")
	stageName progress.Stage

	// Throttling configuration and state
	throttleInterval time.Duration
	lastReportTime   time.Time
	lastReported     int // Last current value we reported
	reportMutex      sync.Mutex

	streamChan chan progress.Event
	id         int
}

// ID returns the unique identifier for this collector.
func (t *ThrottledCollector) ID() int {
	return t.id
}

// NewThrottledCollector creates a new throttled collector with default 500ms interval.
//
// Parameters:
//   - stageName: Default stage name for events (e.g., "provider_prepare", "rule_execution")
//
// The collector will automatically:
//   - Forward the first event (current == 1 or first call)
//   - Forward the last event (current == total)
//   - Throttle intermediate events to once per 500ms
//
// Events are forwarded to a buffered channel (capacity 100) that Progress subscribes to.
func NewThrottledCollector(stageName progress.Stage) *ThrottledCollector {
	return &ThrottledCollector{
		stageName:        stageName,
		throttleInterval: 500 * time.Millisecond,
		id:               rand.Int(),
		streamChan:       make(chan progress.Event, 100),
	}
}

// NewThrottledCollectorWithInterval creates a throttled collector with custom throttle interval.
//
// Use this when you need finer control over the throttling rate. For example,
// a 100ms interval for more frequent updates or 1s for less frequent updates.
func NewThrottledCollectorWithInterval(stageName progress.Stage, interval time.Duration) *ThrottledCollector {
	return &ThrottledCollector{
		stageName:        stageName,
		throttleInterval: interval,
		id:               rand.Int(),
		streamChan:       make(chan progress.Event, 100),
	}
}

// Report accepts a progress event and forwards it based on throttling rules.
//
// The event will be forwarded if:
//   - It's the first event (Current == 1 or first call)
//   - It's the last event (Current == Total)
//   - throttleInterval has elapsed since the last forwarded event
//
// Otherwise the event is dropped to prevent overwhelming the reporting system.
//
// If the event's Stage is empty, it will be set to the collector's default stageName.
// The event is sent via a non-blocking channel send - if the buffer is full, the
// event is dropped.
//
// This method is safe for concurrent use.
func (t *ThrottledCollector) Report(event progress.Event) {
	defer func() {
		if r := recover(); r != nil {
			// Channel was closed during send, ignore the panic
			// This can happen during shutdown
		}
	}()

	// If stage is not set, use the default
	if event.Stage == "" {
		event.Stage = t.stageName
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
		select {
		case t.streamChan <- event:
			// Event sent successfully
		default:
			// Channel full or closed, drop the event
		}
	} else {
		t.reportMutex.Unlock()
	}
}

// CollectChannel returns the channel that Progress reads events from.
//
// Progress subscribes to this channel to receive throttled events from the collector.
func (t *ThrottledCollector) CollectChannel() chan progress.Event {
	return t.streamChan
}
