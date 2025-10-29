package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/konveyor/analyzer-lsp/pkg/progress"
)

// Demo program that shows how to use the channel reporter
func main() {
	fmt.Println("=== Progress Reporting Demo ===")

	// Create a channel reporter
	reporter := progress.NewChannelReporter()

	// Simulate rule processing in the background
	go simulateRuleProcessing(reporter)

	// Display progress updates
	displayProgress(reporter)

	fmt.Println("\n=== Demo Complete ===")
}

// simulateRuleProcessing simulates an analysis with progress updates
func simulateRuleProcessing(reporter *progress.ChannelReporter) {
	totalRules := 45

	// Initial progress
	reporter.Report(progress.ProgressEvent{
		Stage:   progress.StageInit,
		Message: "Initializing analyzer...",
	})
	time.Sleep(500 * time.Millisecond)

	// Provider initialization
	reporter.Report(progress.ProgressEvent{
		Stage:   progress.StageProviderInit,
		Message: "java-external-provider",
	})
	time.Sleep(800 * time.Millisecond)

	reporter.Report(progress.ProgressEvent{
		Stage:   progress.StageProviderInit,
		Message: "generic-external-provider",
	})
	time.Sleep(600 * time.Millisecond)

	// Rule parsing
	reporter.Report(progress.ProgressEvent{
		Stage:   progress.StageRuleParsing,
		Total:   totalRules,
		Message: fmt.Sprintf("Loaded %d rules", totalRules),
	})
	time.Sleep(300 * time.Millisecond)

	// Rule execution
	reporter.Report(progress.ProgressEvent{
		Stage:   progress.StageRuleExecution,
		Current: 0,
		Total:   totalRules,
		Percent: 0.0,
		Message: "Starting rule execution",
	})

	// Simulate processing each rule
	for i := 1; i <= totalRules; i++ {
		time.Sleep(100 * time.Millisecond) // Simulate work

		reporter.Report(progress.ProgressEvent{
			Stage:   progress.StageRuleExecution,
			Current: i,
			Total:   totalRules,
			Percent: float64(i) / float64(totalRules) * 100.0,
			Message: fmt.Sprintf("rule-%03d", i),
		})
	}

	// Completion
	reporter.Report(progress.ProgressEvent{
		Stage:   progress.StageComplete,
		Current: totalRules,
		Total:   totalRules,
		Percent: 100.0,
		Message: "Analysis complete",
	})

	reporter.Close()
}

// displayProgress shows progress updates with a nice progress bar
func displayProgress(reporter *progress.ChannelReporter) {
	for event := range reporter.Events() {
		switch event.Stage {
		case progress.StageInit:
			fmt.Printf("⏳ %s\n", event.Message)

		case progress.StageProviderInit:
			fmt.Printf("🔌 Provider: %s\n", event.Message)

		case progress.StageRuleParsing:
			fmt.Printf("📋 %s\n", event.Message)

		case progress.StageRuleExecution:
			if event.Total > 0 {
				// Draw progress bar
				bar := drawProgressBar(event.Percent, 40)
				fmt.Printf("\r🔍 Processing: %s %3.0f%% (%d/%d) - %s",
					bar,
					event.Percent,
					event.Current,
					event.Total,
					event.Message)
			}

		case progress.StageComplete:
			fmt.Printf("\n✅ %s\n", event.Message)
		}
	}
}

// drawProgressBar creates a visual progress bar
func drawProgressBar(percent float64, width int) string {
	filled := int(percent / 100.0 * float64(width))
	if filled > width {
		filled = width
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return fmt.Sprintf("[%s]", bar)
}
