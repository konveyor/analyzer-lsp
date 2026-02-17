package reporter

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/progress"
)

// ChannelReporter sends progress events to a Go channel for programmatic consumption.
//
// ChannelReporter provides a bridge between the progress reporting system and
// Go applications that need to consume events programmatically. This is ideal for:
//   - Building custom UIs (web dashboards, GUIs)
//   - Real-time monitoring applications
//   - Testing and validation
//   - Integration with other event processing systems
//
// The reporter uses a buffered channel with non-blocking sends to ensure that
// slow consumers never impact analysis performance. If the consumer can't keep
// up, events are dropped and counted (available via DroppedEvents()).
//
// Lifecycle management is handled via context - the channel automatically closes
// when the context is cancelled, making it easy to integrate with typical Go
// application shutdown patterns.
//
// Thread Safety:
// This reporter is safe for concurrent use. Multiple goroutines can call Report()
// simultaneously without coordination.
//
// Example:
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	reporter := reporter.NewChannelReporter(ctx)
//
//	// Start consuming events in a goroutine
//	go func() {
//	    for event := range reporter.Events() {
//	        fmt.Printf("Progress: %.1f%% - %s\n", event.Percent, event.Message)
//	    }
//	    fmt.Println("Progress reporting complete")
//	}()
//
//	// Use reporter with Progress
//	prog, _ := progress.New(
//	    progress.WithContext(ctx),
//	    progress.WithReporters(reporter),
//	)
type ChannelReporter struct {
	events        chan progress.Event
	mu            sync.RWMutex
	closed        bool
	droppedEvents atomic.Uint64
	log           logr.Logger
}

// ChannelReporterOption is a function that configures a ChannelReporter.
type ChannelReporterOption func(*ChannelReporter)

// WithLogger sets a logger for the ChannelReporter to log dropped events.
//
// When the channel buffer fills up (consumer too slow), events are dropped.
// With a logger configured, each drop is logged at V(1) level with details
// about the event and cumulative drop count.
//
// Example:
//
//	reporter := reporter.NewChannelReporter(ctx,
//	    reporter.WithLogger(log),
//	)
func WithLogger(log logr.Logger) ChannelReporterOption {
	return func(r *ChannelReporter) {
		r.log = log
	}
}

// NewChannelReporter creates a new channel-based progress reporter.
//
// The reporter uses a buffered channel (capacity 100) to prevent blocking the
// analysis. If the consumer is slow and the buffer fills up, events will be
// dropped rather than blocking. Track drops via DroppedEvents().
//
// The reporter automatically closes its channel when the provided context is
// cancelled. This ensures proper cleanup and allows consumers to detect
// completion by ranging over the Events() channel.
//
// Optional: Pass WithLogger to log when events are dropped due to backpressure.
//
// Example:
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	reporter := reporter.NewChannelReporter(ctx, reporter.WithLogger(log))
//
//	// Channel automatically closes when ctx is cancelled
//	for event := range reporter.Events() {
//	    // Process event
//	}
func NewChannelReporter(ctx context.Context, opts ...ChannelReporterOption) *ChannelReporter {
	r := &ChannelReporter{
		events: make(chan progress.Event, 100), // Buffered to prevent blocking
		log:    logr.Discard(),                 // Default to discard logger
	}

	// Apply options
	for _, opt := range opts {
		opt(r)
	}

	// Monitor context and close when cancelled
	go func() {
		<-ctx.Done()
		r.mu.Lock()
		close(r.events)
		r.closed = true
		r.mu.Unlock()
	}()

	return r
}

// Report sends a progress event to the channel.
//
// This method uses a non-blocking send to prevent impacting analysis performance.
// If the channel buffer is full (consumer not keeping up), the event is dropped
// and the drop counter is incremented.
//
// If the reporter has been closed (context cancelled), this method returns
// immediately without panicking, ensuring safe concurrent usage during shutdown.
//
// If the event's Timestamp is zero, it will be set to the current time.
// The Percent field is auto-calculated if not set.
//
// This method is safe for concurrent use.
func (c *ChannelReporter) Report(event progress.Event) {
	// Normalize event (set timestamp, calculate percent)
	normalize(&event)

	// Hold read lock during the entire send operation to prevent Close()
	// from closing the channel while we're sending
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return
	}

	// Non-blocking send
	select {
	case c.events <- event:
		// Event sent successfully
	default:
		// Channel is full, skip this event to avoid blocking analysis
		dropped := c.droppedEvents.Add(1)
		c.log.V(1).Info("progress event dropped due to slow consumer",
			"stage", event.Stage,
			"message", event.Message,
			"total_dropped", dropped,
		)
	}
}

// Events returns the read-only channel for receiving progress events.
//
// Consumers should range over this channel to process events. The channel
// will be closed when the context provided to NewChannelReporter is cancelled,
// allowing the range loop to exit cleanly.
//
// Example:
//
//	for event := range reporter.Events() {
//	    switch event.Stage {
//	    case progress.StageRuleExecution:
//	        updateProgressBar(event.Percent)
//	    case progress.StageComplete:
//	        showCompletionMessage()
//	    }
//	}
//
// The channel has a buffer of 100 events. If consumption is slower than
// production, events will be dropped (see DroppedEvents()).
func (c *ChannelReporter) Events() <-chan progress.Event {
	return c.events
}

// DroppedEvents returns the number of events that were dropped due to
// the channel buffer being full.
//
// A non-zero value indicates that the consumer isn't keeping up with event
// production. Consider:
//   - Optimizing the event consumer to process events faster
//   - Reducing the frequency of progress reporting (use ThrottledCollector)
//   - Increasing the buffer size (requires modifying NewChannelReporter)
//
// Example:
//
//	if dropped := reporter.DroppedEvents(); dropped > 0 {
//	    log.Warn("Dropped %d progress events due to slow consumer", dropped)
//	}
func (c *ChannelReporter) DroppedEvents() uint64 {
	return c.droppedEvents.Load()
}
