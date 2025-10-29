package progress

import (
	"time"
)

// ChannelReporter sends progress events to a Go channel
// Useful for programmatic consumption and building UIs
type ChannelReporter struct {
	events chan ProgressEvent
}

// NewChannelReporter creates a new channel-based progress reporter
func NewChannelReporter() *ChannelReporter {
	return &ChannelReporter{
		events: make(chan ProgressEvent, 100), // Buffered to prevent blocking
	}
}

// Report sends a progress event to the channel
func (c *ChannelReporter) Report(event ProgressEvent) {
	// Set timestamp if not already set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Non-blocking send
	select {
	case c.events <- event:
	default:
		// Channel is full, skip this event to avoid blocking analysis
	}
}

// Events returns the channel for receiving progress events
func (c *ChannelReporter) Events() <-chan ProgressEvent {
	return c.events
}

// Close closes the events channel
func (c *ChannelReporter) Close() {
	close(c.events)
}
