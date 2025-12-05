package collector

import (
	"math/rand"

	"github.com/konveyor/analyzer-lsp/progress"
)

// collector is a simple pass-through collector that forwards all events.
//
// This is the base collector implementation without any throttling or
// filtering. It accepts events via Report() and makes them available
// through a buffered channel for Progress to subscribe to.
//
// Unlike ThrottledCollector, this collector forwards every event without
// any rate limiting. Use this when you want all events delivered or when
// the event source already controls its own rate.
type collector struct {
	id int
	ch chan progress.Event
}

// New creates a new base collector.
//
// The collector has a buffered channel (capacity 100) to prevent blocking
// the event source. Events are dropped if the buffer is full.
//
// Example:
//
//	col := collector.New()
//	prog, _ := progress.New(
//	    progress.WithCollectors(col),
//	)
//	col.Report(progress.Event{Stage: progress.StageInit})
func New() progress.Collector {
	return &collector{
		id: rand.Int(),
		ch: make(chan progress.Event, 100),
	}
}

// ID returns the unique identifier for this collector.
func (c *collector) ID() int {
	return c.id
}

// CollectChannel returns the channel that Progress reads events from.
func (c *collector) CollectChannel() chan progress.Event {
	return c.ch
}

// Report accepts an event and forwards it to the collection channel.
//
// This method uses a non-blocking send. If the channel is full or closed,
// the event is dropped to prevent blocking the caller. A panic recovery
// handler catches any issues from concurrent channel closure.
func (c *collector) Report(event progress.Event) {
	defer func() {
		if r := recover(); r != nil {
			// Channel was closed during send, ignore the panic
			// This can happen during shutdown
		}
	}()
	select {
	case c.ch <- event:
		// Event sent successfully
	default:
		// Channel full or closed, drop the event
	}
}
