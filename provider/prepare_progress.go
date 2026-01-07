package provider

import (
	"fmt"
	"time"

	"github.com/konveyor/analyzer-lsp/progress"
)

// prepareProgressAdapter adapts a progress.ProgressReporter to implement PrepareProgressReporter.
// This allows the existing progress reporting infrastructure to be used for Prepare() phase reporting.
type prepareProgressAdapter struct {
	reporter progress.ProgressReporter
}

// NewPrepareProgressAdapter creates a PrepareProgressReporter from a progress.ProgressReporter.
// This adapter converts Prepare() progress updates into ProgressEvents with "provider_prepare" stage.
//
// Example usage:
//
//	reporter := progress.NewTextReporter(os.Stderr)
//	prepareReporter := provider.NewPrepareProgressAdapter(reporter)
//
//	initConfig := provider.InitConfig{
//	    Location: "/path/to/code",
//	    PrepareProgressReporter: prepareReporter,
//	}
func NewPrepareProgressAdapter(reporter progress.ProgressReporter) PrepareProgressReporter {
	if reporter == nil {
		return nil
	}
	return &prepareProgressAdapter{
		reporter: reporter,
	}
}

// ReportProgress implements PrepareProgressReporter.
// It converts prepare progress into a ProgressEvent and forwards to the underlying reporter.
func (a *prepareProgressAdapter) ReportProgress(providerName string, filesProcessed, totalFiles int) {
	if a.reporter == nil {
		return
	}

	a.reporter.Report(progress.ProgressEvent{
		Timestamp: time.Now(),
		Stage:     progress.StageProviderPrepare,
		Message:   fmt.Sprintf("Preparing %s provider", providerName),
		Current:   filesProcessed,
		Total:     totalFiles,
		Metadata: map[string]interface{}{
			"provider": providerName,
		},
	})
}
