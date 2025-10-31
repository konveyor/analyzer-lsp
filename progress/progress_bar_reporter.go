package progress

import (
	"fmt"
	"io"
	"strings"
	"sync"
)

// ProgressBarReporter writes progress as a visual progress bar with real-time updates.
//
// This reporter displays a dynamic progress bar that updates in place using carriage returns.
// It shows percentage completion, a visual bar, current/total counts, and the current rule being processed.
//
// Example output:
//
//	Processing rules  42% |██████████░░░░░░░░░░░░░░░| 99/235  hibernate6-00280
//
// The bar updates in place during rule execution and prints a final newline when complete.
// This reporter is designed for terminal (TTY) output where ANSI control characters work.
// For non-TTY output (pipes, files), consider using TextReporter instead.
type ProgressBarReporter struct {
	writer     io.Writer
	mu         sync.Mutex
	barWidth   int
	lastLineLen int
	inProgress bool
}

// NewProgressBarReporter creates a new progress bar reporter that writes to w.
//
// The writer is typically os.Stderr for terminal output. The progress bar will
// dynamically update in place using carriage returns (\r).
//
// Example:
//
//	reporter := progress.NewProgressBarReporter(os.Stderr)
//	// Progress bar will be displayed during rule execution
func NewProgressBarReporter(w io.Writer) *ProgressBarReporter {
	return &ProgressBarReporter{
		writer:   w,
		barWidth: 25, // Width of the visual bar
	}
}

// Report processes a progress event and updates the progress bar.
//
// The output format varies by stage:
//   - Provider init: "<message>\n"
//   - Rule parsing: "Loaded X rules\n"
//   - Rule execution: "Processing rules XX% |█████░░░| X/Y  rule-name" (updates in-place)
//   - Dependency analysis: "Analyzing dependencies...\n"
//   - Complete: "Analysis complete!\n"
//
// During rule execution, the progress bar updates in-place using carriage returns (\r)
// until reaching 100%, at which point a newline is printed.
//
// If the event's Timestamp is zero, it will be set to the current time.
// This method is safe for concurrent use.
func (p *ProgressBarReporter) Report(event ProgressEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Normalize event (set timestamp, calculate percent)
	event.normalize()

	switch event.Stage {
	case StageProviderInit:
		// Clear any existing progress bar
		p.clearLine()
		if event.Message != "" {
			fmt.Fprintf(p.writer, "%s\n", event.Message)
		}

	case StageRuleParsing:
		// Clear any existing progress bar
		p.clearLine()
		if event.Total > 0 {
			fmt.Fprintf(p.writer, "Loaded %d rules\n", event.Total)
		}

	case StageRuleExecution:
		if event.Total > 0 {
			p.updateProgressBar(event)
		}

	case StageDependencyAnalysis:
		// Clear progress bar before printing dependency message
		p.clearLine()
		fmt.Fprintf(p.writer, "Analyzing dependencies...\n")

	case StageComplete:
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
// Format: Processing rules  42% |██████████░░░░░░░░░░░░░░░| 99/235  current-rule-name
func (p *ProgressBarReporter) updateProgressBar(event ProgressEvent) {
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
	p.lastLineLen = len(barString)
	p.inProgress = true

	// If we've completed (100%), add a newline
	if event.Current >= event.Total {
		fmt.Fprint(p.writer, "\n")
		p.lastLineLen = 0
		p.inProgress = false
	}
}

// buildProgressBar constructs the progress bar string.
//
// Returns a string like: "Processing rules  42% |██████████░░░░░░░░░░░░░░░| 99/235  rule-name"
func (p *ProgressBarReporter) buildProgressBar(event ProgressEvent) string {
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
