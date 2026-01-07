package progress

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockReporter captures all reported events for testing
type mockReporter struct {
	events []ProgressEvent
	mu     sync.Mutex
}

func (m *mockReporter) Report(event ProgressEvent) {
	m.mu.Lock()
	m.events = append(m.events, event)
	m.mu.Unlock()
}

func (m *mockReporter) GetEvents() []ProgressEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]ProgressEvent{}, m.events...)
}

func (m *mockReporter) EventCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

func TestThrottledReporter_FirstAndLastAlwaysReported(t *testing.T) {
	mock := &mockReporter{}
	reporter := NewThrottledReporter("test", mock)

	total := 100
	for i := 1; i <= total; i++ {
		reporter.Report(ProgressEvent{
			Current: i,
			Total:   total,
		})
	}

	events := mock.GetEvents()
	if len(events) == 0 {
		t.Fatal("Expected at least some events")
	}

	// First event should always be reported
	if events[0].Current != 1 {
		t.Errorf("First event should have Current=1, got %d", events[0].Current)
	}

	// Last event should always be reported
	lastEvent := events[len(events)-1]
	if lastEvent.Current != total {
		t.Errorf("Last event should have Current=%d, got %d", total, lastEvent.Current)
	}
}

func TestThrottledReporter_Throttling(t *testing.T) {
	mock := &mockReporter{}
	// Use short interval for faster testing
	reporter := NewThrottledReporterWithInterval("test", mock, 50*time.Millisecond)

	total := 10
	for i := 1; i <= total; i++ {
		reporter.Report(ProgressEvent{
			Current: i,
			Total:   total,
		})
		// Sleep less than throttle interval
		time.Sleep(10 * time.Millisecond)
	}

	events := mock.GetEvents()

	// Should have first, last, and very few intermediate events due to throttling
	// With 10ms sleeps and 50ms throttle, we expect: first (1) + maybe 1-2 intermediate + last (10)
	if len(events) > 5 {
		t.Errorf("Expected throttling to reduce events to < 5, got %d", len(events))
	}

	// Verify first and last are present
	if events[0].Current != 1 {
		t.Error("First event missing")
	}
	if events[len(events)-1].Current != total {
		t.Error("Last event missing")
	}
}

func TestThrottledReporter_IntervalElapsed(t *testing.T) {
	mock := &mockReporter{}
	reporter := NewThrottledReporterWithInterval("test", mock, 50*time.Millisecond)

	// Send events with sufficient delay to bypass throttling
	reporter.Report(ProgressEvent{Current: 1, Total: 100})
	time.Sleep(60 * time.Millisecond)

	reporter.Report(ProgressEvent{Current: 50, Total: 100})
	time.Sleep(60 * time.Millisecond)

	reporter.Report(ProgressEvent{Current: 100, Total: 100})

	events := mock.GetEvents()
	if len(events) != 3 {
		t.Errorf("Expected 3 events (all delays exceeded interval), got %d", len(events))
	}
}

func TestThrottledReporter_Streaming(t *testing.T) {
	mock := &mockReporter{}
	reporter := NewThrottledReporter("test", mock)

	// Create buffered channel to avoid blocking
	eventChan := make(chan ProgressEvent, 100)
	reporter.EnableStreaming(eventChan)

	total := 10
	for i := 1; i <= total; i++ {
		reporter.Report(ProgressEvent{
			Current: i,
			Total:   total,
		})
	}

	// Close channel to signal completion
	reporter.DisableStreaming()
	close(eventChan)

	// Collect streamed events
	var streamedEvents []ProgressEvent
	for event := range eventChan {
		streamedEvents = append(streamedEvents, event)
	}

	// Stream should receive the same events as the reporter
	reporterEvents := mock.GetEvents()
	if len(streamedEvents) != len(reporterEvents) {
		t.Errorf("Stream received %d events, reporter received %d",
			len(streamedEvents), len(reporterEvents))
	}
}

func TestThrottledReporter_NonBlockingStream(t *testing.T) {
	mock := &mockReporter{}
	reporter := NewThrottledReporter("test", mock)

	// Create unbuffered channel (will block immediately)
	eventChan := make(chan ProgressEvent)
	reporter.EnableStreaming(eventChan)

	// This should not block even though channel has no consumer
	reporter.Report(ProgressEvent{
		Current: 1,
		Total:   10,
	})

	// Clean up
	reporter.DisableStreaming()
	close(eventChan)

	// Reporter should still have received the event
	if mock.EventCount() != 1 {
		t.Errorf("Expected 1 event in reporter, got %d", mock.EventCount())
	}
}

func TestThrottledReporter_NilReporter(t *testing.T) {
	// Should work fine with nil reporter (stream-only mode)
	reporter := NewThrottledReporter("test", nil)

	eventChan := make(chan ProgressEvent, 10)
	reporter.EnableStreaming(eventChan)

	reporter.Report(ProgressEvent{Current: 1, Total: 10})
	reporter.Report(ProgressEvent{Current: 10, Total: 10})

	reporter.DisableStreaming()
	close(eventChan)

	eventCount := 0
	for range eventChan {
		eventCount++
	}

	if eventCount != 2 {
		t.Errorf("Expected 2 events in stream, got %d", eventCount)
	}
}

func TestThrottledReporter_EventNormalization(t *testing.T) {
	mock := &mockReporter{}
	reporter := NewThrottledReporter("provider_prepare", mock)

	reporter.Report(ProgressEvent{
		Current: 50,
		Total:   100,
		// Timestamp and Percent not set
	})

	events := mock.GetEvents()
	if len(events) == 0 {
		t.Fatal("Expected at least one event")
	}

	event := events[0]

	// Timestamp should be set
	if event.Timestamp.IsZero() {
		t.Error("Expected timestamp to be set")
	}

	// Percent should be calculated
	expectedPercent := 50.0
	if event.Percent != expectedPercent {
		t.Errorf("Expected percent=%f, got %f", expectedPercent, event.Percent)
	}

	// Stage should be set from reporter's stageName
	if event.Stage != "provider_prepare" {
		t.Errorf("Expected stage=provider_prepare, got %s", event.Stage)
	}
}

func TestThrottledReporter_ConcurrentUse(t *testing.T) {
	mock := &mockReporter{}
	reporter := NewThrottledReporterWithInterval("test", mock, 10*time.Millisecond)

	var wg sync.WaitGroup
	goroutines := 10
	reportsPerGoroutine := 100

	// Launch multiple goroutines reporting concurrently
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 1; j <= reportsPerGoroutine; j++ {
				reporter.Report(ProgressEvent{
					Current: j,
					Total:   reportsPerGoroutine,
				})
			}
		}()
	}

	wg.Wait()

	// Should not panic and should have some events
	eventCount := mock.EventCount()
	if eventCount == 0 {
		t.Error("Expected some events from concurrent reporters")
	}
}

func TestThrottledReporter_Reset(t *testing.T) {
	mock := &mockReporter{}
	reporter := NewThrottledReporterWithInterval("test", mock, 100*time.Millisecond)

	// First operation
	reporter.Report(ProgressEvent{Current: 1, Total: 10})
	time.Sleep(50 * time.Millisecond) // Less than throttle interval
	reporter.Report(ProgressEvent{Current: 5, Total: 10})

	firstOpEvents := mock.EventCount()

	// Reset and second operation
	reporter.Reset()
	reporter.Report(ProgressEvent{Current: 1, Total: 10})

	secondOpEvents := mock.EventCount()

	// After reset, first event should be reported again
	if secondOpEvents <= firstOpEvents {
		t.Error("Expected more events after reset + new first event")
	}
}

func TestThrottledReporter_StreamEnableDisable(t *testing.T) {
	mock := &mockReporter{}
	reporter := NewThrottledReporter("test", mock)

	eventChan1 := make(chan ProgressEvent, 10)
	reporter.EnableStreaming(eventChan1)

	reporter.Report(ProgressEvent{Current: 1, Total: 10})

	reporter.DisableStreaming()

	// This should not go to the stream
	reporter.Report(ProgressEvent{Current: 5, Total: 10})

	eventChan2 := make(chan ProgressEvent, 10)
	reporter.EnableStreaming(eventChan2)

	reporter.Report(ProgressEvent{Current: 10, Total: 10})

	reporter.DisableStreaming()
	close(eventChan1)
	close(eventChan2)

	// First channel should have 1 event
	count1 := 0
	for range eventChan1 {
		count1++
	}
	if count1 != 1 {
		t.Errorf("Expected 1 event in first channel, got %d", count1)
	}

	// Second channel should have 1 event
	count2 := 0
	for range eventChan2 {
		count2++
	}
	if count2 != 1 {
		t.Errorf("Expected 1 event in second channel, got %d", count2)
	}
}

func BenchmarkThrottledReporter(b *testing.B) {
	mock := &mockReporter{}
	reporter := NewThrottledReporter("test", mock)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reporter.Report(ProgressEvent{
			Current: i,
			Total:   b.N,
		})
	}
}

func BenchmarkThrottledReporter_WithStreaming(b *testing.B) {
	mock := &mockReporter{}
	reporter := NewThrottledReporter("test", mock)

	eventChan := make(chan ProgressEvent, 1000)
	reporter.EnableStreaming(eventChan)

	// Consumer goroutine
	done := make(chan struct{})
	go func() {
		for range eventChan {
			// Consume events
		}
		close(done)
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reporter.Report(ProgressEvent{
			Current: i,
			Total:   b.N,
		})
	}

	reporter.DisableStreaming()
	close(eventChan)
	<-done
}

func BenchmarkThrottledReporter_Concurrent(b *testing.B) {
	mock := &mockReporter{}
	reporter := NewThrottledReporter("test", mock)

	var count atomic.Int32

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			current := count.Add(1)
			reporter.Report(ProgressEvent{
				Current: int(current),
				Total:   b.N,
			})
		}
	})
}
