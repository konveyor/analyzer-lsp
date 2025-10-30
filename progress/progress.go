// Package progress provides real-time progress reporting for analyzer execution.
//
// This package enables tracking and reporting of analysis progress through multiple
// stages including provider initialization, rule parsing, and rule execution. It
// supports multiple output formats (JSON, text, channel-based) and is designed to
// have zero overhead when disabled.
//
// Basic usage:
//
//	// Create a text reporter
//	reporter := progress.NewTextReporter(os.Stderr)
//
//	// Create engine with progress reporting
//	eng := engine.CreateRuleEngine(ctx, workers, log,
//	    engine.WithProgressReporter(reporter),
//	)
//
//	// Progress events will be automatically emitted during analysis
//
// For programmatic consumption:
//
//	// Use channel-based reporter
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	reporter := progress.NewChannelReporter(ctx)
//
//	go func() {
//	    for event := range reporter.Events() {
//	        // Handle progress event
//	        fmt.Printf("Progress: %d%%\n", int(event.Percent))
//	    }
//	}()
package progress

import (
	"time"
)

// ProgressReporter is the interface for reporting analysis progress.
//
// Implementations must be safe for concurrent use. The Report method should
// not block to avoid impacting analysis performance.
type ProgressReporter interface {
	// Report emits a progress event. This method may be called concurrently
	// and should not block. Implementations should handle errors internally
	// to avoid disrupting the analysis.
	Report(event ProgressEvent)
}

// ProgressEvent represents a progress update at a specific point in time.
//
// Events are emitted at key points during analysis:
//   - Provider initialization (start and completion)
//   - Rule parsing (total count discovered)
//   - Rule execution (per-rule completion with percentage)
//   - Analysis completion
//
// Not all fields are populated for all events. For example, init events
// may only have Stage and Message, while rule execution events include
// Current, Total, and Percent.
type ProgressEvent struct {
	// Timestamp is when the event occurred. If not set by the caller,
	// reporters will populate it automatically.
	Timestamp time.Time `json:"timestamp"`

	// Stage indicates which phase of analysis this event relates to.
	Stage Stage `json:"stage"`

	// Message provides human-readable context (e.g., rule ID, provider name).
	Message string `json:"message,omitempty"`

	// Current is the number of items completed so far (e.g., rules processed).
	Current int `json:"current,omitempty"`

	// Total is the total number of items to process.
	Total int `json:"total,omitempty"`

	// Percent is the completion percentage (0-100).
	Percent float64 `json:"percent,omitempty"`

	// Metadata contains additional stage-specific information.
	// For example, error details for failed providers.
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// Stage represents a phase of the analysis process.
//
// Stages occur in a typical sequence:
//  1. StageInit - Analysis starting
//  2. StageProviderInit - Initializing language providers
//  3. StageRuleParsing - Loading and parsing rules
//  4. StageRuleExecution - Processing rules
//  5. StageComplete - Analysis finished
type Stage string

const (
	// StageInit indicates analysis initialization.
	StageInit Stage = "init"

	// StageProviderInit indicates provider initialization.
	// Events include provider name and readiness status.
	StageProviderInit Stage = "provider_init"

	// StageRuleParsing indicates rule loading and parsing.
	// Events include the total number of rules discovered.
	StageRuleParsing Stage = "rule_parsing"

	// StageRuleExecution indicates rule processing.
	// Events include current/total counts and percentage completion.
	StageRuleExecution Stage = "rule_execution"

	// StageDependencyAnalysis indicates dependency analysis (future).
	StageDependencyAnalysis Stage = "dependency_analysis"

	// StageComplete indicates analysis completion.
	StageComplete Stage = "complete"
)
