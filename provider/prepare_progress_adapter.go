package provider

import (
	"github.com/konveyor/analyzer-lsp/progress"
)

// prepareProgressReporterAdapter adapts a PrepareProgressReporter to implement progress.ProgressReporter.
// This allows PrepareProgressReporter implementations to work with the new ThrottledReporter.
type prepareProgressReporterAdapter struct {
	providerName string
	reporter     PrepareProgressReporter
}

// NewPrepareProgressReporterAdapter creates a progress.ProgressReporter from a PrepareProgressReporter.
// The adapter converts progress.ProgressEvent to PrepareProgressReporter.ReportProgress calls.
//
// Parameters:
//   - providerName: Name of the provider (e.g., "java", "nodejs")
//   - reporter: The underlying PrepareProgressReporter
//
// Example usage:
//
//	adapter := provider.NewPrepareProgressReporterAdapter("java", prepareReporter)
//	throttled := progress.NewThrottledReporter("provider_prepare", adapter)
func NewPrepareProgressReporterAdapter(providerName string, reporter PrepareProgressReporter) progress.ProgressReporter {
	if reporter == nil {
		return nil
	}
	return &prepareProgressReporterAdapter{
		providerName: providerName,
		reporter:     reporter,
	}
}

// Report implements progress.ProgressReporter by converting ProgressEvent to ReportProgress call.
func (a *prepareProgressReporterAdapter) Report(event progress.ProgressEvent) {
	if a.reporter != nil {
		a.reporter.ReportProgress(a.providerName, event.Current, event.Total)
	}
}
