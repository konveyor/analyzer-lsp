package reporter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/progress"
)

func TestJSONReporter(t *testing.T) {
	var buf bytes.Buffer
	reporter := NewJSONReporter(&buf)

	event := progress.Event{
		Stage:   progress.StageRuleExecution,
		Current: 10,
		Total:   45,
		Percent: 22.2,
		Message: "test-rule-001",
	}

	reporter.Report(event)

	// Parse the JSON output
	var decoded progress.Event
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) == 0 {
		t.Fatal("Expected at least one line of output")
	}
	if err := json.Unmarshal([]byte(lines[0]), &decoded); err != nil {
		t.Fatalf("Failed to decode JSON: %v", err)
	}

	// Verify fields
	if decoded.Stage != progress.StageRuleExecution {
		t.Errorf("Expected stage %s, got %s", progress.StageRuleExecution, decoded.Stage)
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
		reporter.Report(progress.Event{
			Stage:   progress.StageRuleExecution,
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
		var event progress.Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Errorf("Line %d is not valid JSON: %v", i, err)
		}
	}
}

func TestTextReporter(t *testing.T) {
	var buf bytes.Buffer
	reporter := NewTextReporter(&buf)

	event := progress.Event{
		Stage:   progress.StageRuleExecution,
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
		event         progress.Event
		shouldContain []string
	}{
		{
			name: "init stage",
			event: progress.Event{
				Stage:   progress.StageInit,
				Message: "Starting analysis",
			},
			shouldContain: []string{"Initializing"},
		},
		{
			name: "provider init stage",
			event: progress.Event{
				Stage:   progress.StageProviderInit,
				Message: "java-provider",
			},
			shouldContain: []string{"Provider", "java-provider"},
		},
		{
			name: "rule parsing stage",
			event: progress.Event{
				Stage: progress.StageRuleParsing,
				Total: 45,
			},
			shouldContain: []string{"45 rules"},
		},
		{
			name: "complete stage",
			event: progress.Event{
				Stage: progress.StageComplete,
			},
			shouldContain: []string{"complete"},
		},
		{
			name: "rule execution with both total and message",
			event: progress.Event{
				Stage:   progress.StageRuleExecution,
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reporter := NewChannelReporter(ctx)

	event := progress.Event{
		Stage:   progress.StageRuleExecution,
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reporter := NewChannelReporter(ctx)

	// Send multiple events
	go func() {
		for i := 0; i < 5; i++ {
			reporter.Report(progress.Event{
				Stage:   progress.StageRuleExecution,
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

func TestChannelReporterContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reporter := NewChannelReporter(ctx)

	// Send an event
	reporter.Report(progress.Event{Stage: progress.StageInit})

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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
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
				reporter.Report(progress.Event{
					Stage:   progress.StageRuleExecution,
					Current: j,
					Total:   100,
				})
			}
		}(i)
	}

	// Start all senders at once to maximize race potential
	close(sendersStarted)

	// Cancel context while senders are still active
	time.Sleep(5 * time.Millisecond)
	cancel()

	// Wait for consumer to finish
	<-done

	// If we get here without panicking, the test passes
}

func TestChannelReporterReportAfterClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reporter := NewChannelReporter(ctx)

	// Drain any existing events
	go func() {
		for range reporter.Events() {
		}
	}()

	// Cancel context
	cancel()
	time.Sleep(10 * time.Millisecond)

	// Reporting after close should not panic
	reporter.Report(progress.Event{Stage: progress.StageInit})
	reporter.Report(progress.Event{Stage: progress.StageRuleExecution})
}

func TestChannelReporterDroppedEvents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reporter := NewChannelReporter(ctx)

	// Don't consume events, so channel buffer fills up
	// The buffer is 100, so send 150 events
	for i := 0; i < 150; i++ {
		reporter.Report(progress.Event{
			Stage:   progress.StageRuleExecution,
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

func TestChannelReporterWithLogger(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a test logger that captures log calls
	loggedDrops := 0
	testLogger := testLogr{
		logFunc: func(level int, msg string, keysAndValues ...interface{}) {
			if msg == "progress event dropped due to slow consumer" {
				loggedDrops++
			}
		},
	}

	reporter := NewChannelReporter(ctx, WithLogger(logr.New(testLogger)))

	// Don't consume events, so channel buffer fills up
	// The buffer is 100, so send 120 events to ensure some are dropped
	for i := 0; i < 120; i++ {
		reporter.Report(progress.Event{
			Stage:   progress.StageRuleExecution,
			Current: i,
			Total:   120,
			Message: "test-rule",
		})
	}

	// Verify that dropped events were logged
	dropped := reporter.DroppedEvents()
	if dropped == 0 {
		t.Error("Expected some events to be dropped")
	}

	if loggedDrops != int(dropped) {
		t.Errorf("Expected %d logged drops, got %d", dropped, loggedDrops)
	}
}

// testLogr is a simple test implementation of logr.Logger
type testLogr struct {
	logFunc func(level int, msg string, keysAndValues ...interface{})
}

func (t testLogr) Init(info logr.RuntimeInfo) {}

func (t testLogr) Enabled(level int) bool {
	return true
}

func (t testLogr) Info(level int, msg string, keysAndValues ...interface{}) {
	if t.logFunc != nil {
		t.logFunc(level, msg, keysAndValues...)
	}
}

func (t testLogr) Error(err error, msg string, keysAndValues ...interface{}) {}

func (t testLogr) WithValues(keysAndValues ...interface{}) logr.LogSink {
	return t
}

func (t testLogr) WithName(name string) logr.LogSink {
	return t
}

func TestProgressBarReporter(t *testing.T) {
	var buf bytes.Buffer
	reporter := NewProgressBarReporter(&buf)

	event := progress.Event{
		Stage:   progress.StageRuleExecution,
		Current: 10,
		Total:   45,
		Percent: 22.2,
		Message: "test-rule-001",
	}

	reporter.Report(event)

	output := buf.String()

	// Verify output contains expected components
	if !strings.Contains(output, "Processing rules") {
		t.Errorf("Expected 'Processing rules' in output, got: %s", output)
	}
	if !strings.Contains(output, "22%") {
		t.Errorf("Expected percentage in output, got: %s", output)
	}
	if !strings.Contains(output, "10/45") {
		t.Errorf("Expected '10/45' in output, got: %s", output)
	}
	if !strings.Contains(output, "test-rule-001") {
		t.Errorf("Expected rule ID in output, got: %s", output)
	}
	// Progress bar should use block characters
	if !strings.Contains(output, "█") && !strings.Contains(output, "░") {
		t.Errorf("Expected progress bar characters in output, got: %s", output)
	}
}

func TestProgressBarReporterProgressPercentages(t *testing.T) {
	tests := []struct {
		name         string
		current      int
		total        int
		percent      float64
		expectFilled bool // Should have filled portion
		expectEmpty  bool // Should have empty portion
	}{
		{
			name:         "0 percent",
			current:      0,
			total:        100,
			percent:      0.0,
			expectFilled: false,
			expectEmpty:  true,
		},
		{
			name:         "50 percent",
			current:      50,
			total:        100,
			percent:      50.0,
			expectFilled: true,
			expectEmpty:  true,
		},
		{
			name:         "100 percent",
			current:      100,
			total:        100,
			percent:      100.0,
			expectFilled: true,
			expectEmpty:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			reporter := NewProgressBarReporter(&buf)

			event := progress.Event{
				Stage:   progress.StageRuleExecution,
				Current: tt.current,
				Total:   tt.total,
				Percent: tt.percent,
			}

			reporter.Report(event)

			output := buf.String()

			hasFilled := strings.Contains(output, "█")
			hasEmpty := strings.Contains(output, "░")

			if tt.expectFilled && !hasFilled {
				t.Errorf("Expected filled bar characters (█) in output, got: %s", output)
			}
			if !tt.expectFilled && hasFilled {
				t.Errorf("Did not expect filled bar characters (█) in output, got: %s", output)
			}
			if tt.expectEmpty && !hasEmpty {
				t.Errorf("Expected empty bar characters (░) in output, got: %s", output)
			}
			if !tt.expectEmpty && hasEmpty {
				t.Errorf("Did not expect empty bar characters (░) in output, got: %s", output)
			}

			// 100% should end with newline
			if tt.percent >= 100.0 && !strings.HasSuffix(output, "\n") {
				t.Errorf("Expected output to end with newline at 100%%, got: %s", output)
			}
		})
	}
}

func TestProgressBarReporterStages(t *testing.T) {
	tests := []struct {
		name             string
		event            progress.Event
		shouldContain    []string
		shouldNotContain []string
	}{
		{
			name: "provider init stage",
			event: progress.Event{
				Stage:   progress.StageProviderInit,
				Message: "Initializing java provider",
			},
			shouldContain: []string{"Initializing java provider"},
		},
		{
			name: "rule parsing stage",
			event: progress.Event{
				Stage: progress.StageRuleParsing,
				Total: 235,
			},
			shouldContain: []string{"Loaded 235 rules"},
		},
		{
			name: "dependency analysis stage",
			event: progress.Event{
				Stage: progress.StageDependencyAnalysis,
			},
			shouldContain: []string{"Analyzing dependencies"},
		},
		{
			name: "complete stage",
			event: progress.Event{
				Stage: progress.StageComplete,
			},
			shouldContain: []string{"Analysis complete"},
		},
		{
			name: "rule execution clears previous lines",
			event: progress.Event{
				Stage:   progress.StageRuleExecution,
				Current: 10,
				Total:   45,
			},
			shouldContain: []string{"Processing rules", "10/45"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			reporter := NewProgressBarReporter(&buf)
			reporter.Report(tt.event)

			output := buf.String()
			for _, expected := range tt.shouldContain {
				if !strings.Contains(output, expected) {
					t.Errorf("Expected '%s' in output, got: %s", expected, output)
				}
			}
			for _, notExpected := range tt.shouldNotContain {
				if strings.Contains(output, notExpected) {
					t.Errorf("Did not expect '%s' in output, got: %s", notExpected, output)
				}
			}
		})
	}
}

func TestProgressBarReporterRuleNameTruncation(t *testing.T) {
	var buf bytes.Buffer
	reporter := NewProgressBarReporter(&buf)

	// Very long rule name (over 50 characters)
	longRuleName := "this-is-a-very-long-rule-name-that-exceeds-the-maximum-display-length-and-should-be-truncated"

	event := progress.Event{
		Stage:   progress.StageRuleExecution,
		Current: 10,
		Total:   45,
		Percent: 22.2,
		Message: longRuleName,
	}

	reporter.Report(event)

	output := buf.String()

	// Output should not contain the full long rule name
	if strings.Contains(output, longRuleName) {
		t.Errorf("Expected long rule name to be truncated, but found full name in: %s", output)
	}

	// Should contain truncation indicator (...)
	if !strings.Contains(output, "...") {
		t.Errorf("Expected truncation indicator '...' in output, got: %s", output)
	}
}

func TestProgressBarReporterMultipleUpdates(t *testing.T) {
	var buf bytes.Buffer
	reporter := NewProgressBarReporter(&buf)

	// Simulate progress from 0% to 100%
	for i := 0; i <= 10; i++ {
		reporter.Report(progress.Event{
			Stage:   progress.StageRuleExecution,
			Current: i,
			Total:   10,
			Percent: float64(i) * 10.0,
			Message: fmt.Sprintf("rule-%d", i),
		})
	}

	output := buf.String()

	// Should contain carriage returns for in-place updates
	if !strings.Contains(output, "\r") {
		t.Errorf("Expected carriage returns for in-place updates, got: %s", output)
	}

	// Final output at 100% should have a newline
	if !strings.HasSuffix(output, "\n") {
		t.Errorf("Expected final output to end with newline, got: %s", output)
	}

	// Should contain the final rule name
	if !strings.Contains(output, "rule-10") {
		t.Errorf("Expected final rule name 'rule-10' in output, got: %s", output)
	}
}

func TestProgressBarReporterConcurrency(t *testing.T) {
	// Verify that concurrent Report() calls don't cause a panic
	var buf bytes.Buffer
	reporter := NewProgressBarReporter(&buf)

	// Concurrently send events from multiple goroutines
	numSenders := 5
	done := make(chan struct{})

	for i := 0; i < numSenders; i++ {
		go func(id int) {
			for j := 0; j < 20; j++ {
				reporter.Report(progress.Event{
					Stage:   progress.StageRuleExecution,
					Current: j,
					Total:   20,
					Percent: float64(j) * 5.0,
					Message: fmt.Sprintf("rule-%d-%d", id, j),
				})
			}
			if id == 0 {
				close(done)
			}
		}(i)
	}

	// Wait for at least one sender to complete
	<-done

	// If we get here without panicking, the test passes
}

func TestEventTimestamp(t *testing.T) {
	var buf bytes.Buffer
	reporter := NewJSONReporter(&buf)

	event := progress.Event{
		Stage: progress.StageInit,
		// No timestamp set
	}

	reporter.Report(event)

	var decoded progress.Event
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	json.Unmarshal([]byte(lines[0]), &decoded)

	// Reporter should set timestamp if not provided
	if decoded.Timestamp.IsZero() {
		t.Error("Expected timestamp to be set by reporter")
	}

	// Timestamp should be recent (within last second)
	if time.Since(decoded.Timestamp) > time.Second {
		t.Errorf("Timestamp is too old: %v", decoded.Timestamp)
	}
}

func TestEventMetadata(t *testing.T) {
	var buf bytes.Buffer
	reporter := NewJSONReporter(&buf)

	event := progress.Event{
		Stage: progress.StageRuleExecution,
		Metadata: map[string]interface{}{
			"ruleType": "java",
			"matched":  true,
			"duration": 1.5,
		},
	}

	reporter.Report(event)

	var decoded progress.Event
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	json.Unmarshal([]byte(lines[0]), &decoded)

	if decoded.Metadata["ruleType"] != "java" {
		t.Errorf("Expected ruleType 'java', got %v", decoded.Metadata["ruleType"])
	}
	if decoded.Metadata["matched"] != true {
		t.Errorf("Expected matched true, got %v", decoded.Metadata["matched"])
	}
}

func BenchmarkJSONReporter(b *testing.B) {
	var buf bytes.Buffer
	reporter := NewJSONReporter(&buf)

	event := progress.Event{
		Stage:   progress.StageRuleExecution,
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

	event := progress.Event{
		Stage:   progress.StageRuleExecution,
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reporter := NewChannelReporter(ctx)

	event := progress.Event{
		Stage:   progress.StageRuleExecution,
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

func BenchmarkProgressBarReporter(b *testing.B) {
	var buf bytes.Buffer
	reporter := NewProgressBarReporter(&buf)

	event := progress.Event{
		Stage:   progress.StageRuleExecution,
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
