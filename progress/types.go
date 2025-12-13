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
//	// Create engine
//	eng := engine.CreateRuleEngine(ctx, workers, log)
//
//	// Run analysis with progress reporting
//	results := eng.RunRulesWithOptions(ctx, ruleSets, []engine.RunOption{
//	    engine.WithProgressReporter(reporter),
//	})
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

// ProgressInterface defines the contract for managing collector subscriptions.
//
// This interface is implemented by the Progress struct and allows for
// dynamic subscription management - collectors can be added or removed
// at runtime.
type ProgressInterface interface {
	// Subscribe starts receiving events from a collector.
	Subscribe(collector Collector)

	// Unsubscribe stops receiving events from a collector.
	Unsubscribe(collector Collector)
}

// Reporter is the interface for outputting progress events.
//
// Reporters receive events from Progress and format/output them in various ways:
//   - TextReporter: Human-readable text output with timestamps
//   - JSONReporter: Structured JSON for logging or external consumers
//   - ProgressBarReporter: Interactive terminal progress bars
//   - ChannelReporter: Exposes events via a Go channel for programmatic use
//   - NoopReporter: Discards events (used as default when no reporter configured)
//
// Implementations must be safe for concurrent use. The Report method should
// not block to avoid impacting analysis performance, as it's called from
// Progress's reporter worker goroutines.
//
// Each reporter runs in its own goroutine with a buffered channel, so slow
// reporters won't block event collection or other reporters.
type Reporter interface {
	// Report outputs a progress event.
	//
	// This method is called by Progress's reporter workers and should not block.
	// Events arrive pre-normalized with timestamps and calculated percentages.
	Report(event Event)
}

// Collector is the interface for gathering progress events from various sources.
//
// Collectors receive progress events (typically via a Collect or Report method on
// their concrete implementation) and make them available through a channel that
// Progress can subscribe to. Collectors enable decoupling of event generation
// from event reporting.
//
// Implementations must be safe for concurrent use and typically include:
//   - An event channel that Progress reads from via CollectChannel()
//   - A unique ID for subscription management via ID()
//   - Buffering and/or throttling to prevent overwhelming the system
//
// Common collector types include:
//   - ThrottledCollector: Throttles high-frequency events to a reasonable rate
//   - BaseCollector: Simple pass-through collector without throttling
//
// Collectors embed the Reporter interface, meaning they accept events via Report()
// and forward them through their collection channel.
type Collector interface {
	// Reporter embeds the ability to receive events.
	// Concrete collectors implement Report() to accept events and forward
	// them to their internal channel.
	Reporter

	// ID returns a unique identifier for this collector.
	// Used by Progress to manage subscriptions and unsubscriptions.
	// This should be auto-generated when creating a collector.
	ID() int

	// CollectChannel returns the channel from which Progress reads events.
	// Progress subscribes to this channel to receive events from the collector.
	CollectChannel() chan Event
}

// Event represents a progress update at a specific point in time.
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
type Event struct {
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
	// This field is automatically calculated from Current and Total if not set.
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

	// StageProviderStart indicates that we start the provider binary
	// on an open port.
	StageProviderStart Stage = "provider_start"

	// StageProviderInit indicates provider initialization.
	// Events include provider name and readiness status.
	StageProviderInit Stage = "provider_init"

	// StageProviderPrepare indicates provider Prepare() phase (symbol cache population).
	// Events include provider name, files processed, and total files.
	StageProviderPrepare Stage = "provider_prepare"

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
