// Package progress provides progress reporting for long-running operations
// in analyzer-lsp.
//
// The architecture follows a pipeline: Collectors gather events from
// components, Progress multiplexes them, and Reporters output them.
//
//   - Collector: gathers events from a component (see progress/collector)
//   - Progress: central hub that subscribes to collectors and fans out to reporters
//   - Reporter: outputs events in a specific format (see progress/reporter)
//
// All types are safe for concurrent use.
package progress
