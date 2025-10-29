package progress

import (
	"time"
)

// ProgressReporter is the interface for reporting analysis progress
type ProgressReporter interface {
	Report(event ProgressEvent)
}

// ProgressEvent represents a progress update at a specific point in time
type ProgressEvent struct {
	Timestamp time.Time              `json:"timestamp"`
	Stage     Stage                  `json:"stage"`
	Message   string                 `json:"message,omitempty"`
	Current   int                    `json:"current,omitempty"`
	Total     int                    `json:"total,omitempty"`
	Percent   float64                `json:"percent,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Stage represents a phase of the analysis process
type Stage string

const (
	StageInit               Stage = "init"
	StageProviderInit       Stage = "provider_init"
	StageRuleParsing        Stage = "rule_parsing"
	StageRuleExecution      Stage = "rule_execution"
	StageDependencyAnalysis Stage = "dependency_analysis"
	StageComplete           Stage = "complete"
)
