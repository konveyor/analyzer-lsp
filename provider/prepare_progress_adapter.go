package provider

import (
	"github.com/konveyor/analyzer-lsp/progress"
)

// prepareProgressReporterAdapter adapts a PrepareProgressReporter to implement progress.Reporter.
// This allows PrepareProgressReporter implementations to work with the new ThrottledReporter.
type prepareProgressReporterAdapter struct {
	providerName string
	reporter     PrepareProgressReporter
}

// NewPrepareProgressReporterAdapter creates a progress.Reporter from a PrepareProgressReporter.
// The adapter converts progress.Event to PrepareProgressReporter.ReportProgress calls.
//
// Parameters:
//   - providerName: Name of the provider (e.g., "java", "nodejs")
//   - reporter: The underlying PrepareProgressReporter
//
// Example usage:
//
//	adapter := provider.NewPrepareProgressReporterAdapter("java", prepareReporter)
//	throttled := progress.NewThrottledReporter("provider_prepare", adapter)
func NewPrepareProgressReporterAdapter(providerName string, reporter PrepareProgressReporter) progress.Reporter {
	if reporter == nil {
		return nil
	}
	return &prepareProgressReporterAdapter{
		providerName: providerName,
		reporter:     reporter,
	}
}

// Report implements progress.Reporter by converting ProgressEvent to ReportProgress call.
func (a *prepareProgressReporterAdapter) Report(event progress.Event) {
	if a.reporter != nil {
		a.reporter.ReportProgress(a.providerName, event.Current, event.Total)
	}
}
