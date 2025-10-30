package progress

import (
	"sync"
	"sync/atomic"
	"time"
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
//	reporter := progress.NewChannelReporter()
//	defer reporter.Close()
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
}

// NewChannelReporter creates a new channel-based progress reporter.
//
// The reporter uses a buffered channel (capacity 100) to prevent blocking the
// analysis. If the consumer is slow and the buffer fills up, events will be
// dropped rather than blocking.
//
// Always call Close() when finished to release resources.
func NewChannelReporter() *ChannelReporter {
	return &ChannelReporter{
		events: make(chan ProgressEvent, 100), // Buffered to prevent blocking
	}
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
		c.droppedEvents.Add(1)
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
// This should be called when the analysis is complete. It's safe to call Close()
// multiple times, though subsequent calls have no effect.
//
// Always defer Close() immediately after creating the reporter:
//
//	reporter := progress.NewChannelReporter()
//	defer reporter.Close()
func (c *ChannelReporter) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.closed {
		c.closed = true
		close(c.events)
	}
}
