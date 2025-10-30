package progress

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
)

// ChannelReporter sends progress events to a Go channel for programmatic consumption.
//
// This reporter is designed for building custom UIs, web dashboards, or other tools
// that need to consume progress events in real-time within a Go program. Events are
// sent to a buffered channel using non-blocking sends to prevent impacting analysis
// performance.
//
// Important: Always call Close() when done to release resources and signal completion
// to consumers.
//
// Example:
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	reporter := progress.NewChannelReporter(ctx)
//
//	// Start consuming events in a goroutine
//	go func() {
//	    for event := range reporter.Events() {
//	        fmt.Printf("Progress: %d%%\n", int(event.Percent))
//	    }
//	}()
//
//	// Use reporter with engine
//	eng := engine.CreateRuleEngine(ctx, 10, log,
//	    engine.WithProgressReporter(reporter),
//	)
type ChannelReporter struct {
	events        chan ProgressEvent
	mu            sync.RWMutex
	closed        bool
	droppedEvents atomic.Uint64
	log           logr.Logger
}

// ChannelReporterOption is a function that configures a ChannelReporter.
type ChannelReporterOption func(*ChannelReporter)

// WithLogger sets a logger for the ChannelReporter to log dropped events.
func WithLogger(log logr.Logger) ChannelReporterOption {
	return func(r *ChannelReporter) {
		r.log = log
	}
}

// NewChannelReporter creates a new channel-based progress reporter.
//
// The reporter uses a buffered channel (capacity 100) to prevent blocking the
// analysis. If the consumer is slow and the buffer fills up, events will be
// dropped rather than blocking.
//
// The reporter automatically closes when the provided context is cancelled,
// following the standard Go pattern for shutdown logic. You can also manually
// call Close() when finished to release resources.
//
// Optional: Pass WithLogger to log when events are dropped due to backpressure.
//
// Example:
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	reporter := progress.NewChannelReporter(ctx, progress.WithLogger(log))
//	// When ctx is cancelled, the channel will automatically close
func NewChannelReporter(ctx context.Context, opts ...ChannelReporterOption) *ChannelReporter {
	r := &ChannelReporter{
		events: make(chan ProgressEvent, 100), // Buffered to prevent blocking
		log:    logr.Discard(),                // Default to discard logger
	}

	// Apply options
	for _, opt := range opts {
		opt(r)
	}

	// Monitor context and close when cancelled
	go func() {
		<-ctx.Done()
		r.Close()
	}()

	return r
}

// Report sends a progress event to the channel.
//
// This method uses a non-blocking send. If the channel buffer is full (meaning
// the consumer is not keeping up), the event will be dropped to avoid blocking
// the analysis. This ensures progress reporting never impacts performance.
//
// If the reporter has been closed, this method returns immediately without
// panicking, ensuring safe concurrent usage.
//
// If the event's Timestamp is zero, it will be set to the current time.
func (c *ChannelReporter) Report(event ProgressEvent) {
	// Set timestamp if not already set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

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
// Consumers should range over this channel to process events:
//
//	for event := range reporter.Events() {
//	    // Process event
//	}
//
// The channel will be closed when Close() is called.
func (c *ChannelReporter) Events() <-chan ProgressEvent {
	return c.events
}

// DroppedEvents returns the number of events that were dropped due to
// the channel buffer being full.
//
// This can be used to monitor backpressure. If this number is high, consider:
//   - Increasing the buffer size in NewChannelReporter
//   - Optimizing the event consumer to process events faster
//   - Reducing the frequency of progress reporting
func (c *ChannelReporter) DroppedEvents() uint64 {
	return c.droppedEvents.Load()
}

// Close closes the events channel, signaling to consumers that no more events will be sent.
//
// Note: The reporter automatically closes when the context passed to NewChannelReporter
// is cancelled, so explicit Close() calls are often unnecessary. However, it's safe to
// call Close() multiple times, and subsequent calls have no effect.
//
// You can still manually close if needed:
//
//	reporter.Close()
func (c *ChannelReporter) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.closed {
		c.closed = true
		close(c.events)
	}
}
