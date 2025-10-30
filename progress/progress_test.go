package progress

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestJSONReporter(t *testing.T) {
	var buf bytes.Buffer
	reporter := NewJSONReporter(&buf)

	event := ProgressEvent{
		Stage:   StageRuleExecution,
		Current: 10,
		Total:   45,
		Percent: 22.2,
		Message: "test-rule-001",
	}

	reporter.Report(event)

	// Parse the JSON output
	var decoded ProgressEvent
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("Failed to decode JSON: %v", err)
	}

	// Verify fields
	if decoded.Stage != StageRuleExecution {
		t.Errorf("Expected stage %s, got %s", StageRuleExecution, decoded.Stage)
	}
	if decoded.Current != 10 {
		t.Errorf("Expected current 10, got %d", decoded.Current)
	}
	if decoded.Total != 45 {
		t.Errorf("Expected total 45, got %d", decoded.Total)
	}
	if decoded.Percent != 22.2 {
		t.Errorf("Expected percent 22.2, got %f", decoded.Percent)
	}
	if decoded.Message != "test-rule-001" {
		t.Errorf("Expected message 'test-rule-001', got '%s'", decoded.Message)
	}
}

func TestJSONReporterMultipleEvents(t *testing.T) {
	var buf bytes.Buffer
	reporter := NewJSONReporter(&buf)

	// Report multiple events
	for i := 0; i < 3; i++ {
		reporter.Report(ProgressEvent{
			Stage:   StageRuleExecution,
			Current: i + 1,
			Total:   3,
		})
	}

	// Each event should be on a separate line
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Errorf("Expected 3 JSON lines, got %d", len(lines))
	}

	// Verify each line is valid JSON
	for i, line := range lines {
		var event ProgressEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Errorf("Line %d is not valid JSON: %v", i, err)
		}
	}
}

func TestTextReporter(t *testing.T) {
	var buf bytes.Buffer
	reporter := NewTextReporter(&buf)

	event := ProgressEvent{
		Stage:   StageRuleExecution,
		Current: 10,
		Total:   45,
		Percent: 22.2,
	}

	reporter.Report(event)

	output := buf.String()

	// Verify output contains expected information
	if !strings.Contains(output, "10/45") {
		t.Errorf("Expected '10/45' in output, got: %s", output)
	}
	if !strings.Contains(output, "22.") || !strings.Contains(output, "%") {
		t.Errorf("Expected percentage in output, got: %s", output)
	}

	// Test with message
	buf.Reset()
	event.Message = "test-rule-001"
	reporter.Report(event)

	output = buf.String()
	if !strings.Contains(output, "test-rule-001") {
		t.Errorf("Expected rule ID in output, got: %s", output)
	}
}

func TestTextReporterStages(t *testing.T) {
	tests := []struct {
		name          string
		event         ProgressEvent
		shouldContain []string
	}{
		{
			name: "init stage",
			event: ProgressEvent{
				Stage:   StageInit,
				Message: "Starting analysis",
			},
			shouldContain: []string{"Initializing"},
		},
		{
			name: "provider init stage",
			event: ProgressEvent{
				Stage:   StageProviderInit,
				Message: "java-provider",
			},
			shouldContain: []string{"Provider", "java-provider"},
		},
		{
			name: "rule parsing stage",
			event: ProgressEvent{
				Stage: StageRuleParsing,
				Total: 45,
			},
			shouldContain: []string{"45 rules"},
		},
		{
			name: "complete stage",
			event: ProgressEvent{
				Stage: StageComplete,
			},
			shouldContain: []string{"complete"},
		},
		{
			name: "rule execution with both total and message",
			event: ProgressEvent{
				Stage:   StageRuleExecution,
				Current: 10,
				Total:   45,
				Percent: 22.2,
				Message: "test-rule-001",
			},
			shouldContain: []string{"Processing rules", "10/45", "22.2", "test-rule-001"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			reporter := NewTextReporter(&buf)
			reporter.Report(tt.event)

			output := buf.String()
			for _, expected := range tt.shouldContain {
				if !strings.Contains(strings.ToLower(output), strings.ToLower(expected)) {
					t.Errorf("Expected '%s' in output, got: %s", expected, output)
				}
			}
		})
	}
}

func TestChannelReporter(t *testing.T) {
	ctx := context.Background()
	reporter := NewChannelReporter(ctx)
	defer reporter.Close()

	event := ProgressEvent{
		Stage:   StageRuleExecution,
		Current: 10,
		Total:   45,
		Percent: 22.2,
		Message: "test-rule",
	}

	// Send event in goroutine
	go reporter.Report(event)

	// Receive event
	select {
	case received := <-reporter.Events():
		if received.Stage != event.Stage {
			t.Errorf("Expected stage %s, got %s", event.Stage, received.Stage)
		}
		if received.Current != event.Current {
			t.Errorf("Expected current %d, got %d", event.Current, received.Current)
		}
		if received.Total != event.Total {
			t.Errorf("Expected total %d, got %d", event.Total, received.Total)
		}
		if received.Message != event.Message {
			t.Errorf("Expected message '%s', got '%s'", event.Message, received.Message)
		}
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for event")
	}
}

func TestChannelReporterMultipleEvents(t *testing.T) {
	ctx := context.Background()
	reporter := NewChannelReporter(ctx)
	defer reporter.Close()

	// Send multiple events
	go func() {
		for i := 0; i < 5; i++ {
			reporter.Report(ProgressEvent{
				Stage:   StageRuleExecution,
				Current: i + 1,
				Total:   5,
			})
		}
	}()

	// Receive and verify all events
	received := 0
	timeout := time.After(2 * time.Second)

	for received < 5 {
		select {
		case event := <-reporter.Events():
			received++
			if event.Current != received {
				t.Errorf("Expected current %d, got %d", received, event.Current)
			}
		case <-timeout:
			t.Fatalf("Timeout: only received %d events, expected 5", received)
		}
	}
}

func TestChannelReporterClose(t *testing.T) {
	ctx := context.Background()
	reporter := NewChannelReporter(ctx)

	// Send an event
	reporter.Report(ProgressEvent{Stage: StageInit})

	// Drain the channel
	<-reporter.Events()

	// Close the reporter
	reporter.Close()

	// Channel should be closed, reading should return zero value and ok=false
	_, ok := <-reporter.Events()
	if ok {
		t.Error("Expected channel to be closed")
	}
}

func TestChannelReporterCloseMultipleTimes(t *testing.T) {
	ctx := context.Background()
	reporter := NewChannelReporter(ctx)

	// Close multiple times should not panic
	reporter.Close()
	reporter.Close()
	reporter.Close()
}

func TestChannelReporterContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reporter := NewChannelReporter(ctx)

	// Send an event
	reporter.Report(ProgressEvent{Stage: StageInit})

	// Drain the channel
	<-reporter.Events()

	// Cancel the context
	cancel()

	// Give the goroutine time to close the channel
	time.Sleep(10 * time.Millisecond)

	// Channel should be closed
	_, ok := <-reporter.Events()
	if ok {
		t.Error("Expected channel to be closed after context cancellation")
	}
}

func TestChannelReporterRaceCondition(t *testing.T) {
	// This test verifies that concurrent Report() and Close() calls don't cause a panic
	ctx := context.Background()
	reporter := NewChannelReporter(ctx)

	// Start consuming events
	done := make(chan struct{})
	go func() {
		for range reporter.Events() {
			// Consume events
		}
		close(done)
	}()

	// Concurrently send events from multiple goroutines
	numSenders := 10
	sendersStarted := make(chan struct{})
	for i := 0; i < numSenders; i++ {
		go func(id int) {
			<-sendersStarted
			for j := 0; j < 100; j++ {
				reporter.Report(ProgressEvent{
					Stage:   StageRuleExecution,
					Current: j,
					Total:   100,
				})
			}
		}(i)
	}

	// Start all senders at once to maximize race potential
	close(sendersStarted)

	// Close the reporter while senders are still active
	time.Sleep(5 * time.Millisecond)
	reporter.Close()

	// Wait for consumer to finish
	<-done

	// If we get here without panicking, the test passes
}

func TestChannelReporterReportAfterClose(t *testing.T) {
	ctx := context.Background()
	reporter := NewChannelReporter(ctx)

	// Drain any existing events
	go func() {
		for range reporter.Events() {
		}
	}()

	// Close the reporter
	reporter.Close()

	// Reporting after close should not panic
	reporter.Report(ProgressEvent{Stage: StageInit})
	reporter.Report(ProgressEvent{Stage: StageRuleExecution})
}

func TestChannelReporterDroppedEvents(t *testing.T) {
	ctx := context.Background()
	reporter := NewChannelReporter(ctx)
	defer reporter.Close()

	// Don't consume events, so channel buffer fills up
	// The buffer is 100, so send 150 events
	for i := 0; i < 150; i++ {
		reporter.Report(ProgressEvent{
			Stage:   StageRuleExecution,
			Current: i,
			Total:   150,
		})
	}

	// At least some events should have been dropped
	dropped := reporter.DroppedEvents()
	if dropped == 0 {
		t.Error("Expected some events to be dropped when channel buffer is full")
	}

	// Should be approximately 50 dropped events
	if dropped < 40 || dropped > 60 {
		t.Logf("Warning: Expected ~50 dropped events, got %d (this is informational)", dropped)
	}
}

func TestNoopReporter(t *testing.T) {
	reporter := NewNoopReporter()

	// Should not panic or do anything
	reporter.Report(ProgressEvent{
		Stage:   StageRuleExecution,
		Current: 10,
		Total:   45,
	})

	// Multiple calls should also be fine
	for i := 0; i < 100; i++ {
		reporter.Report(ProgressEvent{
			Stage: StageRuleExecution,
		})
	}
}

func TestProgressEventTimestamp(t *testing.T) {
	var buf bytes.Buffer
	reporter := NewJSONReporter(&buf)

	event := ProgressEvent{
		Stage: StageInit,
		// No timestamp set
	}

	reporter.Report(event)

	var decoded ProgressEvent
	json.Unmarshal(buf.Bytes(), &decoded)

	// Reporter should set timestamp if not provided
	if decoded.Timestamp.IsZero() {
		t.Error("Expected timestamp to be set by reporter")
	}

	// Timestamp should be recent (within last second)
	if time.Since(decoded.Timestamp) > time.Second {
		t.Errorf("Timestamp is too old: %v", decoded.Timestamp)
	}
}

func TestProgressEventMetadata(t *testing.T) {
	var buf bytes.Buffer
	reporter := NewJSONReporter(&buf)

	event := ProgressEvent{
		Stage: StageRuleExecution,
		Metadata: map[string]interface{}{
			"ruleType": "java",
			"matched":  true,
			"duration": 1.5,
		},
	}

	reporter.Report(event)

	var decoded ProgressEvent
	json.Unmarshal(buf.Bytes(), &decoded)

	if decoded.Metadata["ruleType"] != "java" {
		t.Errorf("Expected ruleType 'java', got %v", decoded.Metadata["ruleType"])
	}
	if decoded.Metadata["matched"] != true {
		t.Errorf("Expected matched true, got %v", decoded.Metadata["matched"])
	}
}

func TestStageConstants(t *testing.T) {
	// Verify all stage constants are defined
	stages := []Stage{
		StageInit,
		StageProviderInit,
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

func BenchmarkJSONReporter(b *testing.B) {
	var buf bytes.Buffer
	reporter := NewJSONReporter(&buf)

	event := ProgressEvent{
		Stage:   StageRuleExecution,
		Current: 10,
		Total:   45,
		Percent: 22.2,
		Message: "test-rule",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reporter.Report(event)
	}
}

func BenchmarkTextReporter(b *testing.B) {
	var buf bytes.Buffer
	reporter := NewTextReporter(&buf)

	event := ProgressEvent{
		Stage:   StageRuleExecution,
		Current: 10,
		Total:   45,
		Percent: 22.2,
		Message: "test-rule",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reporter.Report(event)
	}
}

func BenchmarkChannelReporter(b *testing.B) {
	ctx := context.Background()
	reporter := NewChannelReporter(ctx)
	defer reporter.Close()

	event := ProgressEvent{
		Stage:   StageRuleExecution,
		Current: 10,
		Total:   45,
	}

	// Consumer goroutine
	go func() {
		for range reporter.Events() {
			// Consume events
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reporter.Report(event)
	}
}
