package provider

import (
	"fmt"
	"reflect"
)

// prepareProgressAdapter adapts a progress.ProgressReporter to implement PrepareProgressReporter.
// This allows the existing progress reporting infrastructure to be used for Prepare() phase reporting.
// We use interface{} and reflection to avoid import cycles between provider and progress packages.
type prepareProgressAdapter struct {
	reporter interface{} // Should be progress.ProgressReporter
}

// NewPrepareProgressAdapter creates a PrepareProgressReporter from a progress.ProgressReporter.
// This adapter converts Prepare() progress updates into ProgressEvents with "provider_prepare" stage.
//
// The reporter parameter should be a progress.ProgressReporter, but is typed as interface{}
// to avoid import cycles.
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
func NewPrepareProgressAdapter(reporter interface{}) PrepareProgressReporter {
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

	// Use reflection to call Report method on the progress reporter
	// This avoids import cycles between provider and progress packages
	reporterValue := reflect.ValueOf(a.reporter)
	reportMethod := reporterValue.MethodByName("Report")

	if !reportMethod.IsValid() {
		return
	}

	// Create the event struct using reflection
	// We know the structure matches progress.ProgressEvent
	eventType := reportMethod.Type().In(0)
	eventValue := reflect.New(eventType).Elem()

	// Set the fields
	eventValue.FieldByName("Stage").SetString("provider_prepare")
	eventValue.FieldByName("Message").SetString(fmt.Sprintf("Preparing %s provider", providerName))
	eventValue.FieldByName("Current").SetInt(int64(filesProcessed))
	eventValue.FieldByName("Total").SetInt(int64(totalFiles))

	// Set metadata
	metadata := make(map[string]interface{})
	metadata["provider"] = providerName
	eventValue.FieldByName("Metadata").Set(reflect.ValueOf(metadata))

	// Call Report method
	reportMethod.Call([]reflect.Value{eventValue})
}
