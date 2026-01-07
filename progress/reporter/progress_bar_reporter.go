package reporter

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/konveyor/analyzer-lsp/progress"
)

// ProgressBarReporter writes progress as a visual progress bar with real-time updates.
//
// ProgressBarReporter provides an interactive terminal experience by displaying
// a dynamic progress bar that updates in-place. This is ideal for:
//   - Interactive terminal sessions where users want visual feedback
//   - Long-running analysis where percentage completion is important
//   - Situations where detailed per-event logging would be too verbose
//
// The reporter uses carriage returns (\r) to update the same line, creating
// an animated effect. It shows:
//   - Percentage completion
//   - Visual bar with filled (█) and empty (░) segments
//   - Current/total counts
//   - Currently processing item (e.g., rule name)
//
// IMPORTANT: This reporter is designed for TTY (terminal) output where ANSI
// control characters work. For non-TTY output (pipes, files, CI/CD logs),
// use TextReporter or JSONReporter instead.
//
// The reporter is thread-safe and uses a mutex to ensure the progress bar
// updates atomically without corruption.
//
// Example output:
//
//	Initializing nodejs provider
//	Provider nodejs ready
//	Loaded 235 rules
//	Processing rules  42% |██████████░░░░░░░░░░░░░░░| 99/235  hibernate6-00280
//	Analysis complete!
//
// Usage:
//
//	reporter := reporter.NewProgressBarReporter(os.Stderr)
//	prog, _ := progress.New(
//	    progress.WithReporters(reporter),
//	)
type ProgressBarReporter struct {
	writer      io.Writer
	mu          sync.Mutex
	barWidth    int
	lastLineLen int
	inProgress  bool
}

// NewProgressBarReporter creates a new progress bar reporter that writes to w.
//
// The writer should typically be os.Stderr for terminal output. The progress bar
// will dynamically update in place using carriage returns (\r).
//
// The visual bar width is fixed at 25 characters for consistent formatting.
// The bar uses Unicode block characters (█ for filled, ░ for empty).
//
// Example:
//
//	reporter := reporter.NewProgressBarReporter(os.Stderr)
//	// Progress bar will be displayed during rule execution:
//	// Processing rules  50% |████████████░░░░░░░░░░░░░| 5/10  rule-name
func NewProgressBarReporter(w io.Writer) *ProgressBarReporter {
	return &ProgressBarReporter{
		writer:   w,
		barWidth: 25, // Width of the visual bar
	}
}

// Report processes a progress event and updates the progress bar.
//
// The output format varies by stage:
//   - StageProviderInit: "<message>\n" (static line)
//   - StageRuleParsing: "Loaded X rules\n" (static line)
//   - StageRuleExecution: "Processing rules XX% |█████░░░| X/Y  rule-name" (updates in-place)
//   - StageDependencyAnalysis: "Analyzing dependencies...\n" (static line)
//   - StageComplete: "Analysis complete!\n" (static line)
//
// During rule execution, the progress bar updates in-place by clearing the
// previous line and redrawing. When reaching 100%, a newline is added to
// preserve the final state.
//
// If the event's Timestamp is zero, it will be set to the current time
// (though this reporter doesn't display timestamps).
//
// This method is safe for concurrent use.
func (p *ProgressBarReporter) Report(event progress.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Normalize event (set timestamp, calculate percent)
	normalize(&event)

	switch event.Stage {
	case progress.StageProviderInit:
		// Clear any existing progress bar
		p.clearLine()
		if event.Message != "" {
			fmt.Fprintf(p.writer, "%s\n", event.Message)
		}

	case progress.StageRuleParsing:
		// Clear any existing progress bar
		p.clearLine()
		if event.Total > 0 {
			fmt.Fprintf(p.writer, "Loaded %d rules\n", event.Total)
		}

	case progress.StageProviderPrepare:
		if event.Total > 0 {
			p.updateProviderPrepareBar(event)
		}

	case progress.StageRuleExecution:
		if event.Total > 0 {
			p.updateProgressBar(event)
		}

	case progress.StageDependencyAnalysis:
		// Clear progress bar before printing dependency message
		p.clearLine()
		fmt.Fprintf(p.writer, "Analyzing dependencies...\n")

	case progress.StageComplete:
		// Clear progress bar and print completion message
		p.clearLine()
		fmt.Fprintf(p.writer, "Analysis complete!\n")

	default:
		// For other stages, clear progress bar and print message
		p.clearLine()
		if event.Message != "" {
			fmt.Fprintf(p.writer, "%s\n", event.Message)
		}
	}
}

// updateProgressBar renders and updates the visual progress bar.
//
// This method handles the in-place update logic:
//  1. Clear the previous line by overwriting with spaces
//  2. Render the new progress bar string
//  3. Write without newline (so next update overwrites)
//  4. Add newline when reaching 100%
//
// Format: Processing rules  42% |██████████░░░░░░░░░░░░░░░| 99/235  current-rule-name
func (p *ProgressBarReporter) updateProgressBar(event progress.Event) {
	// Build the progress bar string
	barString := p.buildProgressBar(event)

	// Clear the previous line if needed
	if p.lastLineLen > 0 {
		// Move cursor to beginning of line
		fmt.Fprint(p.writer, "\r")
		// Overwrite with spaces to clear
		fmt.Fprint(p.writer, strings.Repeat(" ", p.lastLineLen))
		// Move back to beginning
		fmt.Fprint(p.writer, "\r")
	}

	// Write the new progress bar (without newline - will update in place)
	fmt.Fprint(p.writer, barString)
	p.lastLineLen = utf8.RuneCountInString(barString)
	p.inProgress = true

	// If we've completed (100%), add a newline
	if event.Current >= event.Total {
		fmt.Fprint(p.writer, "\n")
		p.lastLineLen = 0
		p.inProgress = false
	}
}

// updateProviderPrepareBar renders and updates the visual progress bar for provider preparation.
//
// Format: Preparing nodejs provider  92% |███████████████████████░░| 546/592 files
func (p *ProgressBarReporter) updateProviderPrepareBar(event progress.Event) {
	// Build the progress bar string
	barString := p.buildProviderPrepareBar(event)

	// Clear the previous line if needed
	if p.lastLineLen > 0 {
		// Move cursor to beginning of line
		fmt.Fprint(p.writer, "\r")
		// Overwrite with spaces to clear
		fmt.Fprint(p.writer, strings.Repeat(" ", p.lastLineLen))
		// Move back to beginning
		fmt.Fprint(p.writer, "\r")
	}

	// Write the new progress bar (without newline - will update in place)
	fmt.Fprint(p.writer, barString)
	p.lastLineLen = utf8.RuneCountInString(barString)
	p.inProgress = true

	// If we've completed (100%), add a newline
	if event.Current >= event.Total {
		fmt.Fprint(p.writer, "\n")
		p.lastLineLen = 0
		p.inProgress = false
	}
}

// buildProviderPrepareBar constructs the provider preparation progress bar string.
//
// Returns a string like: "Preparing nodejs provider  92% |███████████████████████░░| 546/592 files"
func (p *ProgressBarReporter) buildProviderPrepareBar(event progress.Event) string {
	// Calculate filled portion of the bar
	filledWidth := int(float64(p.barWidth) * event.Percent / 100.0)
	if filledWidth > p.barWidth {
		filledWidth = p.barWidth
	}
	emptyWidth := p.barWidth - filledWidth

	// Build the visual bar
	filledBar := strings.Repeat("█", filledWidth)
	emptyBar := strings.Repeat("░", emptyWidth)
	visualBar := fmt.Sprintf("|%s%s|", filledBar, emptyBar)

	// Build the full line
	// Format: "Preparing <provider> provider  XX% |bar| current/total files"
	percentStr := fmt.Sprintf("%3d%%", int(event.Percent))
	countStr := fmt.Sprintf("%d/%d files", event.Current, event.Total)

	// Assemble the full progress line
	line := fmt.Sprintf("%s %s %s %s",
		event.Message, percentStr, visualBar, countStr)

	return line
}

// buildProgressBar constructs the progress bar string.
//
// Calculates the filled vs empty portions of the bar based on percentage,
// then assembles the complete line with all components.
//
// Returns a string like: "Processing rules  42% |██████████░░░░░░░░░░░░░░░| 99/235  rule-name"
func (p *ProgressBarReporter) buildProgressBar(event progress.Event) string {
	// Calculate filled portion of the bar
	filledWidth := int(float64(p.barWidth) * event.Percent / 100.0)
	if filledWidth > p.barWidth {
		filledWidth = p.barWidth
	}
	emptyWidth := p.barWidth - filledWidth

	// Build the visual bar
	filledBar := strings.Repeat("█", filledWidth)
	emptyBar := strings.Repeat("░", emptyWidth)
	visualBar := fmt.Sprintf("|%s%s|", filledBar, emptyBar)

	// Build the full line
	// Format: "Processing rules  XX% |bar| current/total  rule-name"
	percentStr := fmt.Sprintf("%3d%%", int(event.Percent))
	countStr := fmt.Sprintf("%d/%d", event.Current, event.Total)

	// Assemble the full progress line
	var line string
	if event.Message != "" {
		// Include current rule name with truncation if too long
		ruleName := event.Message
		maxRuleLen := 50 // Maximum length for rule name display
		if len(ruleName) > maxRuleLen {
			ruleName = ruleName[:maxRuleLen-3] + "..."
		}
		line = fmt.Sprintf("Processing rules %s %s %s  %s",
			percentStr, visualBar, countStr, ruleName)
	} else {
		line = fmt.Sprintf("Processing rules %s %s %s",
			percentStr, visualBar, countStr)
	}

	return line
}

// clearLine clears the current progress bar line if one is displayed.
//
// This is called before printing static messages to ensure the progress bar
// doesn't leave artifacts on the terminal.
func (p *ProgressBarReporter) clearLine() {
	if p.lastLineLen > 0 {
		// Move to beginning, clear with spaces, move back
		fmt.Fprint(p.writer, "\r")
		fmt.Fprint(p.writer, strings.Repeat(" ", p.lastLineLen))
		fmt.Fprint(p.writer, "\r")
		p.lastLineLen = 0
		p.inProgress = false
	}
}
