package collector

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/konveyor/analyzer-lsp/progress"
)

func TestThrottledCollector_FirstAndLastAlwaysReported(t *testing.T) {
	collector := NewThrottledCollector("test")

	// Consume events in background
	var events []progress.Event
	var mu sync.Mutex
	done := make(chan struct{})

	go func() {
		for event := range collector.CollectChannel() {
			mu.Lock()
			events = append(events, event)
			mu.Unlock()
		}
		close(done)
	}()

	total := 100
	for i := 1; i <= total; i++ {
		collector.Report(progress.Event{
			Current: i,
			Total:   total,
		})
	}

	// Give time for events to be processed
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	eventCount := len(events)
	mu.Unlock()

	if eventCount == 0 {
		t.Fatal("Expected at least some events")
	}

	// Verify first event
	mu.Lock()
	firstEvent := events[0]
	mu.Unlock()
	if firstEvent.Current != 1 {
		t.Errorf("First event should have Current=1, got %d", firstEvent.Current)
	}

	// Verify last event
	mu.Lock()
	lastEvent := events[len(events)-1]
	mu.Unlock()
	if lastEvent.Current != total {
		t.Errorf("Last event should have Current=%d, got %d", total, lastEvent.Current)
	}
}

func TestThrottledCollector_Throttling(t *testing.T) {
	// Use short interval for faster testing
	collector := NewThrottledCollectorWithInterval("test", 50*time.Millisecond)

	// Consume events in background
	var events []progress.Event
	var mu sync.Mutex
	done := make(chan struct{})

	go func() {
		for event := range collector.CollectChannel() {
			mu.Lock()
			events = append(events, event)
			mu.Unlock()
		}
		close(done)
	}()

	total := 10
	for i := 1; i <= total; i++ {
		collector.Report(progress.Event{
			Current: i,
			Total:   total,
		})
		// Sleep less than throttle interval
		time.Sleep(10 * time.Millisecond)
	}

	// Give time for events to be processed
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	eventCount := len(events)
	firstCurrent := events[0].Current
	lastCurrent := events[len(events)-1].Current
	mu.Unlock()

	// Should have first, last, and very few intermediate events due to throttling
	// With 10ms sleeps and 50ms throttle, we expect: first (1) + maybe 1-2 intermediate + last (10)
	if eventCount > 5 {
		t.Errorf("Expected throttling to reduce events to < 5, got %d", eventCount)
	}

	// Verify first and last are present
	if firstCurrent != 1 {
		t.Error("First event missing")
	}
	if lastCurrent != total {
		t.Error("Last event missing")
	}
}

func TestThrottledCollector_IntervalElapsed(t *testing.T) {
	collector := NewThrottledCollectorWithInterval("test", 50*time.Millisecond)

	// Consume events in background
	var events []progress.Event
	var mu sync.Mutex
	done := make(chan struct{})

	go func() {
		for event := range collector.CollectChannel() {
			mu.Lock()
			events = append(events, event)
			mu.Unlock()
		}
		close(done)
	}()

	// Send events with sufficient delay to bypass throttling
	collector.Report(progress.Event{Current: 1, Total: 100})
	time.Sleep(60 * time.Millisecond)

	collector.Report(progress.Event{Current: 50, Total: 100})
	time.Sleep(60 * time.Millisecond)

	collector.Report(progress.Event{Current: 100, Total: 100})

	// Give time for events to be processed
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	eventCount := len(events)
	mu.Unlock()

	if eventCount != 3 {
		t.Errorf("Expected 3 events (all delays exceeded interval), got %d", eventCount)
	}
}

func TestThrottledCollector_DefaultStage(t *testing.T) {
	stageName := progress.StageProviderPrepare
	collector := NewThrottledCollector(stageName)

	// Consume events in background
	var events []progress.Event
	var mu sync.Mutex
	done := make(chan struct{})

	go func() {
		for event := range collector.CollectChannel() {
			mu.Lock()
			events = append(events, event)
			mu.Unlock()
		}
		close(done)
	}()

	// Send event without stage set
	collector.Report(progress.Event{
		Current: 50,
		Total:   100,
	})

	// Give time for event to be processed
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if len(events) == 0 {
		mu.Unlock()
		t.Fatal("Expected at least one event")
	}

	event := events[0]
	mu.Unlock()

	// Stage should be set from collector's stageName
	if event.Stage != stageName {
		t.Errorf("Expected stage=%s, got %s", stageName, event.Stage)
	}
}

func TestThrottledCollector_ConcurrentUse(t *testing.T) {
	collector := NewThrottledCollectorWithInterval("test", 10*time.Millisecond)

	// Consume events in background
	var eventCount atomic.Int32
	done := make(chan struct{})

	go func() {
		for range collector.CollectChannel() {
			eventCount.Add(1)
		}
		close(done)
	}()

	var wg sync.WaitGroup
	goroutines := 10
	reportsPerGoroutine := 100

	// Launch multiple goroutines reporting concurrently
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 1; j <= reportsPerGoroutine; j++ {
				collector.Report(progress.Event{
					Current: j,
					Total:   reportsPerGoroutine,
				})
			}
		}()
	}

	wg.Wait()

	// Give time for events to be processed
	time.Sleep(100 * time.Millisecond)

	// Should not panic and should have some events
	count := eventCount.Load()
	if count == 0 {
		t.Error("Expected some events from concurrent reporters")
	}
}

func TestThrottledCollector_ChannelBuffer(t *testing.T) {
	collector := NewThrottledCollector("test")

	// Don't consume events - let them accumulate
	// Send 150 events to a 100-capacity buffer
	for i := 0; i < 150; i++ {
		collector.Report(progress.Event{
			Current: i + 1,
			Total:   150,
		})
	}

	// Verify we can still read from the channel (non-blocking sends worked)
	select {
	case event := <-collector.CollectChannel():
		// Should receive the first event
		if event.Current != 1 {
			t.Errorf("Expected first event to have Current=1, got %d", event.Current)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout reading from channel")
	}
}

func TestThrottledCollector_ID(t *testing.T) {
	collector1 := NewThrottledCollector("test1")
	collector2 := NewThrottledCollector("test2")

	id1 := collector1.ID()
	id2 := collector2.ID()

	// IDs should be different
	if id1 == id2 {
		t.Error("Expected different collector IDs")
	}

	// ID should be consistent
	if collector1.ID() != id1 {
		t.Error("Collector ID changed on second call")
	}
}

func TestBaseCollector_ForwardsAllEvents(t *testing.T) {
	collector := New()

	// Consume events in background
	var events []progress.Event
	var mu sync.Mutex
	done := make(chan struct{})

	go func() {
		for event := range collector.CollectChannel() {
			mu.Lock()
			events = append(events, event)
			mu.Unlock()
		}
		close(done)
	}()

	// Send events
	total := 10
	for i := 1; i <= total; i++ {
		collector.Report(progress.Event{
			Stage:   progress.StageRuleExecution,
			Current: i,
			Total:   total,
		})
	}

	// Give time for events to be processed
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	eventCount := len(events)
	mu.Unlock()

	// Base collector should forward all events (no throttling)
	if eventCount != total {
		t.Errorf("Expected %d events, got %d", total, eventCount)
	}

	// Verify events are in order
	mu.Lock()
	for i, event := range events {
		expected := i + 1
		if event.Current != expected {
			t.Errorf("Event %d: expected Current=%d, got %d", i, expected, event.Current)
		}
	}
	mu.Unlock()
}

func TestBaseCollector_ID(t *testing.T) {
	collector1 := New()
	collector2 := New()

	id1 := collector1.ID()
	id2 := collector2.ID()

	// IDs should be different
	if id1 == id2 {
		t.Error("Expected different collector IDs")
	}

	// ID should be consistent
	if collector1.ID() != id1 {
		t.Error("Collector ID changed on second call")
	}
}

func TestBaseCollector_ChannelBuffer(t *testing.T) {
	collector := New()

	// Don't consume events - let them accumulate
	// Send 150 events to a 100-capacity buffer
	for i := 0; i < 150; i++ {
		collector.Report(progress.Event{
			Current: i + 1,
			Total:   150,
		})
	}

	// Verify we can still read from the channel (non-blocking sends worked)
	select {
	case event := <-collector.CollectChannel():
		// Should receive the first event
		if event.Current != 1 {
			t.Errorf("Expected first event to have Current=1, got %d", event.Current)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout reading from channel")
	}
}

func BenchmarkThrottledCollector(b *testing.B) {
	collector := NewThrottledCollector("test")

	// Consume events in background
	go func() {
		for range collector.CollectChannel() {
			// Discard events
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		collector.Report(progress.Event{
			Current: i,
			Total:   b.N,
		})
	}
}

func BenchmarkThrottledCollector_Concurrent(b *testing.B) {
	collector := NewThrottledCollector("test")

	// Consume events in background
	go func() {
		for range collector.CollectChannel() {
			// Discard events
		}
	}()

	var count atomic.Int32

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			current := count.Add(1)
			collector.Report(progress.Event{
				Current: int(current),
				Total:   b.N,
			})
		}
	})
}

func BenchmarkBaseCollector(b *testing.B) {
	collector := New()

	// Consume events in background
	go func() {
		for range collector.CollectChannel() {
			// Discard events
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		collector.Report(progress.Event{
			Current: i,
			Total:   b.N,
		})
	}
}
