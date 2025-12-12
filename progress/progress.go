package progress

import (
	"context"
	"sync"
)

// Progress coordinates the flow of progress events between collectors and reporters.
//
// Progress acts as the central hub for progress reporting, managing the lifecycle
// of event collection and distribution. It receives events from multiple collectors
// (which gather progress from various sources) and distributes them to multiple
// reporters (which output progress in different formats).
//
// Architecture:
//   - Collectors send events via channels that Progress subscribes to
//   - Progress multiplexes events from all collectors into a central channel
//   - Events are then fanned out to all registered reporters
//   - Each reporter runs in its own goroutine with buffered channels
//
// Lifecycle:
//  1. Create with New() and options (WithContext, WithReporters, WithCollectors)
//  2. Progress automatically subscribes to collectors and starts reporter workers
//  3. Events flow: Collector -> Progress.collectorChan -> Reporter channels -> Reporters
//  4. Cleanup via context cancellation stops all goroutines
//
// Example:
//
//	ctx := context.Background()
//	textReporter := reporter.NewTextReporter(os.Stderr)
//	throttledCollector := collector.NewThrottledCollector("provider_init")
//
//	prog, err := progress.New(
//	    progress.WithContext(ctx),
//	    progress.WithReporters(textReporter),
//	    progress.WithCollectors(throttledCollector),
//	)
//
//	// Events sent to collectors automatically flow to reporters
//	throttledCollector.Report(progress.Event{
//	    Stage: progress.StageProviderInit,
//	    Message: "Starting initialization",
//	})
//
// Thread Safety:
// Progress is safe for concurrent use. Multiple collectors can send events
// simultaneously, and all reporters receive events concurrently.
type Progress struct {
	ctx                context.Context
	reporters          []Reporter
	reporterChannels   []chan Event
	collectors         []Collector
	collectorChan      chan Event
	collecterCancelMap map[int]context.CancelFunc
	subscribeMutex     sync.Mutex
}

// ProgressOption configures a Progress instance during creation.
type ProgressOption func(p *Progress)

// WithContext sets the context for the Progress instance.
//
// The context is used to control the lifecycle of all background goroutines.
// When the context is cancelled, all reporters and collector subscriptions
// will stop processing events.
func WithContext(ctx context.Context) ProgressOption {
	return func(p *Progress) {
		p.ctx = ctx
	}
}

// WithReporters adds one or more reporters to the Progress instance.
//
// Reporters receive events and output them in various formats (text, JSON,
// progress bar, etc.). Multiple reporters can be active simultaneously,
// each receiving all events.
//
// Example:
//
//	progress.New(
//	    progress.WithReporters(
//	        reporter.NewTextReporter(os.Stderr),
//	        reporter.NewJSONReporter(logFile),
//	    ),
//	)
func WithReporters(reporters ...Reporter) ProgressOption {
	return func(p *Progress) {
		p.reporters = append(p.reporters, reporters...)
	}
}

// WithCollectors adds one or more collectors to the Progress instance.
//
// Collectors gather progress events from various sources and send them
// to Progress for distribution to reporters. Progress automatically
// subscribes to all collectors during initialization.
//
// Example:
//
//	progress.New(
//	    progress.WithCollectors(
//	        collector.NewThrottledCollector("provider_init"),
//	        collector.NewThrottledCollector("rule_execution"),
//	    ),
//	)
func WithCollectors(collectors ...Collector) ProgressOption {
	return func(p *Progress) {
		p.collectors = append(p.collectors, collectors...)
	}
}

// New creates a new Progress instance with the provided options.
//
// If no reporters are specified, a NoopReporter is used by default to ensure
// zero overhead when progress reporting is disabled. If no context is provided,
// the Progress will run until explicitly cancelled.
//
// Options:
//   - WithContext: Provides a context for lifecycle management
//   - WithReporters: Adds reporters for output (text, JSON, progress bar, etc.)
//   - WithCollectors: Adds collectors that will send events to Progress
//
// The function starts background goroutines for:
//   - Multiplexing collector events to reporter channels
//   - Running each reporter worker
//   - Subscribing to each collector's event channel
//
// Example:
//
//	prog, err := progress.New(
//	    progress.WithContext(ctx),
//	    progress.WithReporters(reporter.NewTextReporter(os.Stderr)),
//	)
func New(opts ...ProgressOption) (*Progress, error) {
	pg := &Progress{
		collectorChan:      make(chan Event, 100),
		collecterCancelMap: map[int]context.CancelFunc{},
		subscribeMutex:     sync.Mutex{},
	}
	for _, opt := range opts {
		opt(pg)
	}
	if pg.ctx == nil {
		pg.ctx = context.Background()
	}

	if len(pg.reporters) == 0 {
		// No reporets, will create a no-op reporter
		pg.reporters = append(pg.reporters, &NoopReporter{})
	}

	for _, reporter := range pg.reporters {
		reporterChannel := make(chan Event, 100)
		pg.reporterChannels = append(pg.reporterChannels, reporterChannel)
		go pg.reporterWorker(reporter, reporterChannel)
	}

	go func() {
		for {
			select {
			case event := <-pg.collectorChan:
				for _, ch := range pg.reporterChannels {
					ch <- event
				}
			case <-pg.ctx.Done():
				return
			}
		}
	}()

	for _, collector := range pg.collectors {
		pg.Subscribe(collector)
	}

	return pg, nil

}

// Unsubscribe stops receiving events from the specified collector.
//
// This cancels the goroutine that was listening to the collector's channel.
// Events already in flight may still be processed.
func (p *Progress) Unsubscribe(collector Collector) {
	p.subscribeMutex.Lock()
	subscribeCancel := p.collecterCancelMap[collector.ID()]
	p.subscribeMutex.Unlock()
	subscribeCancel()
}

// Subscribe starts receiving events from the specified collector.
//
// This starts a goroutine that reads from the collector's event channel
// and forwards events to Progress's central collector channel. The goroutine
// continues until either the Progress context is cancelled or Unsubscribe is called.
func (p *Progress) Subscribe(collector Collector) {
	subscribeContext, subscribeCancel := context.WithCancel(p.ctx)
	p.subscribeMutex.Lock()
	p.collecterCancelMap[collector.ID()] = subscribeCancel
	p.subscribeMutex.Unlock()

	go func() {
		for {
			select {
			case event := <-collector.CollectChannel():
				p.collectorChan <- event
			case <-subscribeContext.Done():
				return
			}
		}
	}()
}

// reporterWorker runs in a goroutine, forwarding events to a reporter.
//
// Each reporter has its own worker goroutine and buffered channel to prevent
// slow reporters from blocking event collection. The worker stops when the
// Progress context is cancelled.
func (p *Progress) reporterWorker(reporter Reporter, events chan Event) {
	for {
		select {
		case event := <-events:
			reporter.Report(event)
		case <-p.ctx.Done():
			return
		}
	}
}
