package progress

import (
	"context"
	"sync"
	"testing"
	"time"
)

// mockCollector implements the Collector interface for testing
type mockCollector struct {
	id  int
	ch  chan Event
	mu  sync.Mutex
	rep []Event
}

func newMockCollector(id int) *mockCollector {
	return &mockCollector{
		id: id,
		ch: make(chan Event, 100),
	}
}

func (m *mockCollector) ID() int {
	return m.id
}

func (m *mockCollector) CollectChannel() chan Event {
	return m.ch
}

func (m *mockCollector) Report(event Event) {
	m.mu.Lock()
	m.rep = append(m.rep, event)
	m.mu.Unlock()

	select {
	case m.ch <- event:
	default:
		// Channel full, drop event
	}
}

func (m *mockCollector) getReported() []Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Event{}, m.rep...)
}

// mockReporter implements the Reporter interface for testing
type mockReporter struct {
	events []Event
	mu     sync.Mutex
}

func (m *mockReporter) Report(event Event) {
	m.mu.Lock()
	m.events = append(m.events, event)
	m.mu.Unlock()
}

func (m *mockReporter) GetEvents() []Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Event{}, m.events...)
}

func (m *mockReporter) EventCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

func TestNew_DefaultNoopReporter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	prog, err := New(WithContext(ctx))
	if err != nil {
		t.Fatalf("Failed to create Progress: %v", err)
	}

	if prog == nil {
		t.Fatal("Expected non-nil Progress")
	}

	// Should have created a default NoopReporter (check internal state)
	if len(prog.reporters) != 1 {
		t.Errorf("Expected 1 default reporter, got %d", len(prog.reporters))
	}
}

func TestNew_WithReporters(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reporter1 := &mockReporter{}
	reporter2 := &mockReporter{}

	prog, err := New(
		WithContext(ctx),
		WithReporters(reporter1, reporter2),
	)
	if err != nil {
		t.Fatalf("Failed to create Progress: %v", err)
	}

	if len(prog.reporters) != 2 {
		t.Errorf("Expected 2 reporters, got %d", len(prog.reporters))
	}
}

func TestNew_WithCollectors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	collector1 := newMockCollector(1)
	collector2 := newMockCollector(2)

	prog, err := New(
		WithContext(ctx),
		WithCollectors(collector1, collector2),
	)
	if err != nil {
		t.Fatalf("Failed to create Progress: %v", err)
	}

	if len(prog.collectors) != 2 {
		t.Errorf("Expected 2 collectors, got %d", len(prog.collectors))
	}
}

func TestProgress_EventFlow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	collector := newMockCollector(1)
	reporter := &mockReporter{}

	_, err := New(
		WithContext(ctx),
		WithCollectors(collector),
		WithReporters(reporter),
	)
	if err != nil {
		t.Fatalf("Failed to create Progress: %v", err)
	}

	// Send events through collector
	events := []Event{
		{Stage: StageInit, Message: "Starting"},
		{Stage: StageRuleExecution, Current: 1, Total: 10},
		{Stage: StageRuleExecution, Current: 5, Total: 10},
		{Stage: StageComplete, Message: "Done"},
	}

	for _, event := range events {
		collector.Report(event)
	}

	// Give time for events to flow through the system
	time.Sleep(100 * time.Millisecond)

	// Verify events were received by reporter
	reportedEvents := reporter.GetEvents()
	if len(reportedEvents) != len(events) {
		t.Errorf("Expected %d events at reporter, got %d", len(events), len(reportedEvents))
	}

	// Verify event content
	for i, expected := range events {
		if i >= len(reportedEvents) {
			break
		}
		actual := reportedEvents[i]
		if actual.Stage != expected.Stage {
			t.Errorf("Event %d: expected stage %s, got %s", i, expected.Stage, actual.Stage)
		}
		if actual.Message != expected.Message {
			t.Errorf("Event %d: expected message %s, got %s", i, expected.Message, actual.Message)
		}
	}
}

func TestProgress_MultipleReporters(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	collector := newMockCollector(1)
	reporter1 := &mockReporter{}
	reporter2 := &mockReporter{}

	_, err := New(
		WithContext(ctx),
		WithCollectors(collector),
		WithReporters(reporter1, reporter2),
	)
	if err != nil {
		t.Fatalf("Failed to create Progress: %v", err)
	}

	// Send events
	collector.Report(Event{Stage: StageInit})
	collector.Report(Event{Stage: StageComplete})

	// Give time for events to flow
	time.Sleep(100 * time.Millisecond)

	// Both reporters should receive all events
	if reporter1.EventCount() != 2 {
		t.Errorf("Reporter 1: expected 2 events, got %d", reporter1.EventCount())
	}
	if reporter2.EventCount() != 2 {
		t.Errorf("Reporter 2: expected 2 events, got %d", reporter2.EventCount())
	}
}

func TestProgress_MultipleCollectors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	collector1 := newMockCollector(1)
	collector2 := newMockCollector(2)
	reporter := &mockReporter{}

	_, err := New(
		WithContext(ctx),
		WithCollectors(collector1, collector2),
		WithReporters(reporter),
	)
	if err != nil {
		t.Fatalf("Failed to create Progress: %v", err)
	}

	// Send events from both collectors
	collector1.Report(Event{Stage: StageInit, Message: "Collector 1"})
	collector2.Report(Event{Stage: StageInit, Message: "Collector 2"})

	// Give time for events to flow
	time.Sleep(100 * time.Millisecond)

	// Reporter should receive events from both collectors
	events := reporter.GetEvents()
	if len(events) != 2 {
		t.Errorf("Expected 2 events, got %d", len(events))
	}
}

func TestProgress_Subscribe(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reporter := &mockReporter{}

	prog, err := New(
		WithContext(ctx),
		WithReporters(reporter),
	)
	if err != nil {
		t.Fatalf("Failed to create Progress: %v", err)
	}

	// Subscribe a new collector after creation
	collector := newMockCollector(1)
	prog.Subscribe(collector)

	// Send event
	collector.Report(Event{Stage: StageInit})

	// Give time for event to flow
	time.Sleep(100 * time.Millisecond)

	// Reporter should receive the event
	if reporter.EventCount() != 1 {
		t.Errorf("Expected 1 event, got %d", reporter.EventCount())
	}
}

func TestProgress_Unsubscribe(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	collector := newMockCollector(1)
	reporter := &mockReporter{}

	prog, err := New(
		WithContext(ctx),
		WithCollectors(collector),
		WithReporters(reporter),
	)
	if err != nil {
		t.Fatalf("Failed to create Progress: %v", err)
	}

	// Send first event
	collector.Report(Event{Stage: StageInit, Message: "Before unsubscribe"})
	time.Sleep(50 * time.Millisecond)

	// Unsubscribe
	prog.Unsubscribe(collector)
	time.Sleep(50 * time.Millisecond)

	// Send second event (should not be received)
	collector.Report(Event{Stage: StageComplete, Message: "After unsubscribe"})
	time.Sleep(50 * time.Millisecond)

	// Reporter should only have received the first event
	events := reporter.GetEvents()
	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}
	if len(events) > 0 && events[0].Message != "Before unsubscribe" {
		t.Errorf("Expected first event message, got: %s", events[0].Message)
	}
}

func TestProgress_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	collector := newMockCollector(1)
	reporter := &mockReporter{}

	_, err := New(
		WithContext(ctx),
		WithCollectors(collector),
		WithReporters(reporter),
	)
	if err != nil {
		t.Fatalf("Failed to create Progress: %v", err)
	}

	// Send event before cancellation
	collector.Report(Event{Stage: StageInit})
	time.Sleep(50 * time.Millisecond)

	// Cancel context
	cancel()
	time.Sleep(50 * time.Millisecond)

	// Send event after cancellation (should not be processed)
	collector.Report(Event{Stage: StageComplete})
	time.Sleep(50 * time.Millisecond)

	// Should have only received the first event
	events := reporter.GetEvents()
	if len(events) > 1 {
		t.Errorf("Expected at most 1 event after context cancellation, got %d", len(events))
	}
}

func TestProgress_ConcurrentReporting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	collector := newMockCollector(1)
	reporter := &mockReporter{}

	_, err := New(
		WithContext(ctx),
		WithCollectors(collector),
		WithReporters(reporter),
	)
	if err != nil {
		t.Fatalf("Failed to create Progress: %v", err)
	}

	// Send events concurrently from multiple goroutines
	var wg sync.WaitGroup
	goroutines := 10
	eventsPerGoroutine := 10

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				collector.Report(Event{
					Stage:   StageRuleExecution,
					Current: j,
					Total:   eventsPerGoroutine,
				})
			}
		}(i)
	}

	wg.Wait()

	// Give time for all events to be processed
	time.Sleep(200 * time.Millisecond)

	// Should have received all events
	totalExpected := goroutines * eventsPerGoroutine
	actualCount := reporter.EventCount()
	if actualCount != totalExpected {
		t.Errorf("Expected %d events, got %d", totalExpected, actualCount)
	}
}

func TestNoopReporter(t *testing.T) {
	reporter := NewNoopReporter()

	// Should not panic or do anything
	reporter.Report(Event{
		Stage:   StageRuleExecution,
		Current: 10,
		Total:   45,
	})

	// Multiple calls should also be fine
	for i := 0; i < 100; i++ {
		reporter.Report(Event{
			Stage: StageRuleExecution,
		})
	}
}

func TestStageConstants(t *testing.T) {
	// Verify all stage constants are defined
	stages := []Stage{
		StageInit,
		StageProviderInit,
		StageProviderPrepare,
		StageRuleParsing,
		StageRuleExecution,
		StageDependencyAnalysis,
		StageComplete,
	}

	// Just verify they exist and are not empty
	for _, stage := range stages {
		if stage == "" {
			t.Error("Stage constant is empty")
		}
	}
}

func BenchmarkProgress_SingleCollectorSingleReporter(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	collector := newMockCollector(1)
	reporter := &mockReporter{}

	_, err := New(
		WithContext(ctx),
		WithCollectors(collector),
		WithReporters(reporter),
	)
	if err != nil {
		b.Fatalf("Failed to create Progress: %v", err)
	}

	event := Event{
		Stage:   StageRuleExecution,
		Current: 10,
		Total:   100,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		collector.Report(event)
	}
}

func BenchmarkProgress_MultipleCollectorsMultipleReporters(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	collector1 := newMockCollector(1)
	collector2 := newMockCollector(2)
	reporter1 := &mockReporter{}
	reporter2 := &mockReporter{}

	_, err := New(
		WithContext(ctx),
		WithCollectors(collector1, collector2),
		WithReporters(reporter1, reporter2),
	)
	if err != nil {
		b.Fatalf("Failed to create Progress: %v", err)
	}

	event := Event{
		Stage:   StageRuleExecution,
		Current: 10,
		Total:   100,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			collector1.Report(event)
		} else {
			collector2.Report(event)
		}
	}
}
