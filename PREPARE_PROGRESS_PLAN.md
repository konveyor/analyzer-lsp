# Plan: Add Progress Reporting During Provider Prepare Phase

## Problem Statement

After recent changes that added a `Prepare()` phase to providers, there is now a very long silent pause between "Provider X ready" messages and when the progress bar appears. This pause can last several minutes for large codebases as symbol caches are being populated asynchronously.

## User Experience Examples

### Current Behavior (Before Implementation)

Analyzing a large codebase like tackle2-ui (2000+ TypeScript/JavaScript files):

```
$ ./kantra analyze --input /path/to/tackle2-ui --rules /path/to/rules --target patternfly-6

Starting analysis...
Loaded 47 rules
Initializing providers...
Provider builtin ready
Provider java ready
Provider nodejs ready

[COMPLETE SILENCE FOR 3-5 MINUTES - user thinks it's frozen]

Processing rules  0% |░░░░░░░░░░░░░░░░░░░░| 0/47
Processing rules  2% |█░░░░░░░░░░░░░░░░░░░| 1/47
Processing rules  4% |██░░░░░░░░░░░░░░░░░░| 2/47
...
```

**Problem:** During the silent pause, users:
- Think the analyzer has frozen
- Press Ctrl+C thinking it's stuck
- Don't know if it's actually working
- Can't estimate how long it will take

### Proposed Behavior (After Implementation)

Same analysis with progress reporting during prepare:

```
$ ./kantra analyze --input /path/to/tackle2-ui --rules /path/to/rules --target patternfly-6

Starting analysis...
Loaded 47 rules
Initializing providers...
Provider builtin ready
Provider java ready
Provider nodejs ready
Preparing nodejs provider... 0/2147 files
Preparing nodejs provider... 215/2147 files
Preparing nodejs provider... 523/2147 files
Preparing nodejs provider... 891/2147 files
Preparing nodejs provider... 1204/2147 files
Preparing nodejs provider... 1567/2147 files
Preparing nodejs provider... 1823/2147 files
Preparing nodejs provider... 2089/2147 files
Preparing nodejs provider... 2147/2147 files
Processing rules  0% |░░░░░░░░░░░░░░░░░░░░| 0/47
Processing rules  2% |█░░░░░░░░░░░░░░░░░░░| 1/47
Processing rules  4% |██░░░░░░░░░░░░░░░░░░| 2/47
...
```

### Example with Multiple Providers

Analyzing a polyglot codebase with Java and TypeScript:

```
$ ./kantra analyze --input /path/to/polyglot-app --rules /path/to/rules

Starting analysis...
Loaded 128 rules
Initializing providers...
Provider builtin ready
Provider java ready
Provider nodejs ready
Preparing java provider... 0/456 files
Preparing java provider... 89/456 files
Preparing nodejs provider... 0/1823 files
Preparing java provider... 203/456 files
Preparing nodejs provider... 312/1823 files
Preparing java provider... 367/456 files
Preparing nodejs provider... 687/1823 files
Preparing java provider... 456/456 files
Preparing nodejs provider... 1045/1823 files
Preparing nodejs provider... 1456/1823 files
Preparing nodejs provider... 1823/1823 files
Processing rules  0% |░░░░░░░░░░░░░░░░░░░░| 0/128
Processing rules  1% |█░░░░░░░░░░░░░░░░░░░| 1/128
...
```

**Benefits:**
- User sees continuous feedback - knows it's working
- Provider name shows which provider is being prepared
- File counts give sense of progress and time remaining
- Multiple providers show parallel preparation
- Smooth transition to rule execution phase

### Example with Fast Preparation

Small codebase where prepare finishes quickly:

```
$ ./kantra analyze --input /path/to/small-project --rules /path/to/rules

Starting analysis...
Loaded 23 rules
Initializing providers...
Provider builtin ready
Provider java ready
Provider nodejs ready
Preparing nodejs provider... 0/45 files
Preparing nodejs provider... 45/45 files
Processing rules  0% |░░░░░░░░░░░░░░░░░░░░| 0/23
Processing rules  4% |█░░░░░░░░░░░░░░░░░░░| 1/23
...
```

**Note:** For small codebases, preparation is fast and only shows briefly, providing minimal distraction.

### Example with Builtin Provider Only

When only builtin provider is used (no LSP providers need preparation):

```
$ ./kantra analyze --input /path/to/project --rules /path/to/builtin-rules

Starting analysis...
Loaded 12 rules
Initializing providers...
Provider builtin ready
Processing rules  0% |░░░░░░░░░░░░░░░░░░░░| 0/12
Processing rules  8% |██░░░░░░░░░░░░░░░░░░| 1/12
...
```

**Note:** No prepare messages since builtin provider doesn't do symbol cache preparation.

## Current Architecture

### Provider Prepare Flow (main.go:343-364)

1. `main.go` calls `Prepare()` on each provider sequentially
2. Each LSP-based provider spawns async goroutines to populate symbol cache
3. `main.go` immediately proceeds to `RunRulesWithOptions()` (which starts the progress bar)
4. Symbol cache updates happen in background via `symbolCacheUpdateHandler()`

### LSP Provider Prepare Implementation (base_service_client.go:355-369)

```go
func (sc *LSPServiceClientBase) Prepare(ctx context.Context, conditionsByCap []provider.ConditionsByCap) error {
    sc.allConditionsMutex.Lock()
    sc.allConditions = conditionsByCap
    sc.allConditionsMutex.Unlock()

    sc.symbolCacheUpdateWaitGroup.Add(1)
    go func() {
        defer sc.symbolCacheUpdateWaitGroup.Done()
        uris := sc.symbolSearchHelper.GetDocumentUris(conditionsByCap...)
        sc.symbolCacheUpdateWaitGroup.Add(len(uris))
        for _, uri := range uris {
            sc.symbolCacheUpdateChan <- uri
        }
    }()
    return nil
}
```

### Symbol Cache Update Flow (base_service_client.go:652-685)

- Background worker `symbolCacheUpdateHandler()` reads from `symbolCacheUpdateChan`
- Each URI is processed by `processSymbolCacheUpdate(fileURI)`
- `processSymbolCacheUpdate()` calls `populateDocumentSymbolCache()` for each file
- `symbolCacheUpdateWaitGroup` tracks pending updates

## Proposed Solution

### Approach: Add Progress Tracking to LSPServiceClientBase

Add progress tracking capabilities to `LSPServiceClientBase` that can report progress as files are processed. Then modify `main.go` to monitor this progress and report it via the progress reporter.

### Key Design Decisions

1. **Where to track progress**: In `LSPServiceClientBase` since that's where symbol cache updates happen
2. **How to report progress**: Add atomic counters for total/processed files, expose a method to query progress
3. **When to report progress**: In `main.go`, poll for progress after calling Prepare() and before RunRulesWithOptions()
4. **Progress stage**: Use existing `StageProviderInit` or add new `StageProviderPrepare`

## Implementation Plan

### Step 1: Add Progress Tracking Fields to LSPServiceClientBase

**File:** `lsp/base_service_client/base_service_client.go`

Add to struct (around line 154):
```go
// Progress tracking for symbol cache preparation
totalPrepareFiles     atomic.Int64
processedPrepareFiles atomic.Int64
prepareProviderName   string
```

### Step 2: Add Progress Query Method to LSPServiceClientBase

**File:** `lsp/base_service_client/base_service_client.go`

Add new method:
```go
// GetPrepareProgress returns the current progress of symbol cache preparation
// Returns (processed, total) counts
func (sc *LSPServiceClientBase) GetPrepareProgress() (int64, int64) {
    return sc.processedPrepareFiles.Load(), sc.totalPrepareFiles.Load()
}
```

### Step 3: Update Prepare() to Track Total Files

**File:** `lsp/base_service_client/base_service_client.go` (line 355)

Modify Prepare() to:
1. Reset counters
2. Store provider name
3. Set total count after GetDocumentUris()

```go
func (sc *LSPServiceClientBase) Prepare(ctx context.Context, conditionsByCap []provider.ConditionsByCap) error {
    sc.allConditionsMutex.Lock()
    sc.allConditions = conditionsByCap
    sc.allConditionsMutex.Unlock()

    // Reset progress counters
    sc.processedPrepareFiles.Store(0)
    sc.totalPrepareFiles.Store(0)

    sc.symbolCacheUpdateWaitGroup.Add(1)
    go func() {
        defer sc.symbolCacheUpdateWaitGroup.Done()
        uris := sc.symbolSearchHelper.GetDocumentUris(conditionsByCap...)

        // Set total files count
        sc.totalPrepareFiles.Store(int64(len(uris)))

        sc.symbolCacheUpdateWaitGroup.Add(len(uris))
        for _, uri := range uris {
            sc.symbolCacheUpdateChan <- uri
        }
    }()
    return nil
}
```

### Step 4: Update processSymbolCacheUpdate() to Increment Counter

**File:** `lsp/base_service_client/base_service_client.go` (line 667)

Add progress increment:
```go
func (sc *LSPServiceClientBase) processSymbolCacheUpdate(fileURI uri.URI) {
    defer sc.symbolCacheUpdateWaitGroup.Done()

    // Increment processed count when done
    defer sc.processedPrepareFiles.Add(1)

    if sc.symbolCache == nil || fileURI == "" {
        return
    }
    // ... rest of method unchanged
}
```

### Step 5: Add PrepareProgress Interface to provider.ServiceClient

**File:** `provider/provider.go` (line 478)

Add optional interface for providers that support prepare progress:
```go
// PrepareProgressReporter is an optional interface for providers that can report
// progress during the Prepare phase
type PrepareProgressReporter interface {
    ServiceClient
    // GetPrepareProgress returns (processed, total) file counts during preparation
    GetPrepareProgress() (int64, int64)
}
```

### Step 6: Add Progress Monitoring to main.go

**File:** `cmd/analyzer/main.go` (line 343)

Replace the current Prepare loop with progress monitoring:
```go
// Call Prepare() on all providers and monitor progress
prepareCtx, prepareCancel := context.WithCancel(ctx)
defer prepareCancel()

var wgPrepare sync.WaitGroup
prepareChan := make(chan struct {
    name      string
    processed int64
    total     int64
}, 10)

// Start all providers' Prepare() in parallel
for name, conditions := range providerConditions {
    if provider, ok := needProviders[name]; ok {
        wgPrepare.Add(1)
        go func(providerName string, prov provider.ServiceClient, conds map[string][]byte) {
            defer wgPrepare.Done()

            if err := prov.Prepare(prepareCtx, conditions); err != nil {
                errLog.Error(err, "unable to prepare provider", "provider", providerName)
                return
            }

            // If provider supports progress reporting, monitor it
            if progReporter, ok := prov.(provider.PrepareProgressReporter); ok {
                ticker := time.NewTicker(500 * time.Millisecond)
                defer ticker.Stop()

                for {
                    select {
                    case <-prepareCtx.Done():
                        return
                    case <-ticker.C:
                        processed, total := progReporter.GetPrepareProgress()
                        if total > 0 {
                            select {
                            case prepareChan <- struct {
                                name      string
                                processed int64
                                total     int64
                            }{providerName, processed, total}:
                            default:
                            }

                            // Exit when complete
                            if processed >= total {
                                return
                            }
                        }
                    }
                }
            }
        }(name, provider, conditions)
    }
}

// Monitor and report progress
go func() {
    lastReported := time.Now()
    for prog := range prepareChan {
        // Throttle progress updates to every 500ms
        if time.Since(lastReported) >= 500*time.Millisecond {
            progressReporter.Report(progress.ProgressEvent{
                Stage:   progress.StageProviderInit,
                Current: int(prog.processed),
                Total:   int(prog.total),
                Message: fmt.Sprintf("Preparing %s provider... %d/%d files", prog.name, prog.processed, prog.total),
            })
            lastReported = time.Now()
        }
    }
}()

// Wait for all Prepare() calls to complete
wgPrepare.Wait()
close(prepareChan)
prepareCancel()
```

## Alternative Approaches Considered

### Alternative 1: Add Callback to Prepare() Signature

**Pros:**
- More explicit progress reporting
- Doesn't require interface type assertion

**Cons:**
- Breaks existing Prepare() interface
- All providers would need to be updated
- External providers (gRPC) would need protocol changes

**Decision:** Rejected - too invasive

### Alternative 2: Wait for Prepare to Complete Before Starting Rules

**Pros:**
- Simpler implementation
- No need for progress monitoring goroutines

**Cons:**
- Doesn't solve the silent pause problem
- User still sees no feedback during Prepare

**Decision:** Rejected - doesn't meet user requirements

### Alternative 3: Use Channel-based Progress Reporting

**Pros:**
- Real-time progress without polling
- More efficient

**Cons:**
- More complex implementation
- Requires channel management
- Channel lifecycle issues with cancellation

**Decision:** Rejected - polling is simpler and adequate for this use case

## Files to Modify

1. **lsp/base_service_client/base_service_client.go**
   - Add progress tracking fields (atomic.Int64)
   - Add GetPrepareProgress() method
   - Modify Prepare() to initialize counters
   - Modify processSymbolCacheUpdate() to increment counter

2. **provider/provider.go**
   - Add PrepareProgressReporter interface

3. **cmd/analyzer/main.go**
   - Replace sequential Prepare() loop with parallel monitored version
   - Add progress monitoring goroutine
   - Report progress via progressReporter

4. **progress/progress.go** (optional)
   - Consider adding StageProviderPrepare constant if we want separate stage

## Testing Considerations

1. **Manual Testing:**
   - Run analyzer on large codebase (tackle2-ui)
   - Verify progress messages appear during Prepare phase
   - Verify no regression in rule execution progress

2. **Unit Testing:**
   - Test GetPrepareProgress() returns correct values
   - Test atomic counters thread-safety
   - Mock progress reporter to verify correct events

3. **Integration Testing:**
   - Verify builtin provider (no Prepare) still works
   - Verify grpc providers work correctly
   - Verify java external provider works correctly

## Rollback Plan

If issues arise:
1. Progress tracking fields are additive - remove them
2. GetPrepareProgress() is new - remove it
3. main.go changes - revert to sequential Prepare() loop
4. No database migrations or persistent state changes

## Success Criteria

1. ✅ No silent pause between "Provider ready" and progress bar
2. ✅ Progress updates appear every ~500ms during Prepare phase
3. ✅ Messages show provider name and file counts
4. ✅ Existing functionality unchanged (all tests pass)
5. ✅ Works with all provider types (LSP, builtin, grpc)

## Implementation Timeline

1. **Phase 1:** Add progress tracking to LSPServiceClientBase (30 min)
2. **Phase 2:** Add interface and main.go monitoring (45 min)
3. **Phase 3:** Testing and refinement (30 min)
4. **Total:** ~2 hours

## Open Questions

1. Should we use existing `StageProviderInit` or add new `StageProviderPrepare`?
   - **Recommendation:** Use StageProviderInit with descriptive message

2. How should we handle providers that don't support progress (builtin, grpc)?
   - **Recommendation:** Type assertion - only monitor providers that implement PrepareProgressReporter

3. Should Prepare() calls run in parallel or sequential?
   - **Recommendation:** Parallel - they're independent and waiting is wasted time

4. What if a provider's Prepare() fails?
   - **Recommendation:** Log error, continue with other providers (current behavior)
