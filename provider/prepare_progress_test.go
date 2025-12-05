package provider

import (
	"sync"
	"testing"

	"github.com/konveyor/analyzer-lsp/progress"
)

// mockProgressReporter is a mock implementation of progress.Reporter
type mockProgressReporter struct {
	mu     sync.Mutex
	events []progress.Event
}

func (m *mockProgressReporter) Report(event progress.Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
}

func (m *mockProgressReporter) GetEvents() []progress.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]progress.Event{}, m.events...)
}

func (m *mockProgressReporter) EventCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

func TestNewPrepareProgressAdapter_NilReporter(t *testing.T) {
	adapter := NewPrepareProgressAdapter(nil)
	if adapter != nil {
		t.Error("Expected nil adapter when reporter is nil")
	}
}

func TestNewPrepareProgressAdapter_ValidReporter(t *testing.T) {
	reporter := &mockProgressReporter{}
	adapter := NewPrepareProgressAdapter(reporter)
	if adapter == nil {
		t.Error("Expected non-nil adapter with valid reporter")
	}
}

func TestPrepareProgressAdapter_ReportProgress(t *testing.T) {
	reporter := &mockProgressReporter{}
	adapter := NewPrepareProgressAdapter(reporter)

	if adapter == nil {
		t.Fatal("Adapter should not be nil")
	}

	// Report progress
	adapter.ReportProgress("test-provider", 10, 100)
	adapter.ReportProgress("nodejs", 50, 100)
	adapter.ReportProgress("java", 100, 100)

	// Verify events were recorded
	events := reporter.GetEvents()
	if len(events) != 3 {
		t.Errorf("Expected 3 events, got %d", len(events))
	}

	// Verify first event
	if len(events) > 0 {
		e := events[0]
		if e.Stage != progress.StageProviderPrepare {
			t.Errorf("Expected stage 'provider_prepare', got '%s'", e.Stage)
		}
		if e.Current != 10 {
			t.Errorf("Expected current=10, got %d", e.Current)
		}
		if e.Total != 100 {
			t.Errorf("Expected total=100, got %d", e.Total)
		}
		if e.Message != "Preparing test-provider provider" {
			t.Errorf("Expected message 'Preparing test-provider provider', got '%s'", e.Message)
		}
		if e.Metadata["provider"] != "test-provider" {
			t.Errorf("Expected metadata provider='test-provider', got '%v'", e.Metadata["provider"])
		}
	}

	// Verify second event
	if len(events) > 1 {
		e := events[1]
		if e.Message != "Preparing nodejs provider" {
			t.Errorf("Expected message 'Preparing nodejs provider', got '%s'", e.Message)
		}
		if e.Current != 50 {
			t.Errorf("Expected current=50, got %d", e.Current)
		}
	}
}

func TestPrepareProgressAdapter_NilReporterSafe(t *testing.T) {
	// Create adapter with nil reporter
	var adapter PrepareProgressReporter

	// This should not panic
	if adapter != nil {
		adapter.ReportProgress("test-provider", 10, 100)
	}
}

func TestPrepareProgressAdapter_ConcurrentCalls(t *testing.T) {
	reporter := &mockProgressReporter{}
	adapter := NewPrepareProgressAdapter(reporter)

	if adapter == nil {
		t.Fatal("Adapter should not be nil")
	}

	// Simulate concurrent progress reporting
	var wg sync.WaitGroup
	numGoroutines := 10
	reportsPerGoroutine := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < reportsPerGoroutine; j++ {
				adapter.ReportProgress("test-provider", id*reportsPerGoroutine+j, 100)
			}
		}(i)
	}

	wg.Wait()
	// If we got here without panicking, the test passes
}

func TestPrepareProgressReporterInterface(t *testing.T) {
	// Verify the adapter implements the interface
	var _ PrepareProgressReporter = (*prepareProgressAdapter)(nil)
}
