# Analysis: Performance Optimization for Provider Prepare Phase

## Current Performance Characteristics

### Observed Behavior
- **Large codebase (tackle2-ui ~2000 files)**: 3-5 minute prepare phase
- **Medium codebase (~500 files)**: 1-2 minute prepare phase
- **Small codebase (~50 files)**: 5-10 seconds prepare phase

### Bottleneck Identification

After analyzing the code, I've identified several performance bottlenecks in the symbol cache preparation:

## Bottleneck #1: Single Worker Goroutine (CRITICAL)

**Location:** `lsp/base_service_client/base_service_client.go:277`

**Problem:**
```go
sc.symbolCacheUpdateChan = make(chan uri.URI, 10)
go sc.symbolCacheUpdateHandler()  // Only ONE worker!
```

Only **one** worker goroutine processes all file URIs sequentially. For a codebase with 2000 files, this means:
- Worker processes file 1, waits for completion
- Worker processes file 2, waits for completion
- ... 2000 times sequentially

**Impact:** For 2000 files @ 100ms average per file = **200 seconds minimum** (3.3 minutes)

**Solution:** Use a worker pool with N concurrent workers

### Proposed Fix #1: Multiple Worker Goroutines

```go
const symbolCacheWorkers = 10  // configurable

sc.symbolCacheUpdateChan = make(chan uri.URI, symbolCacheWorkers*2)
for i := 0; i < symbolCacheWorkers; i++ {
    go sc.symbolCacheUpdateHandler()
}
```

**Expected Impact:**
- 10 workers = **10x speedup** → 3 minutes → 18 seconds
- Limited by LSP server capacity and CPU cores

## Bottleneck #2: Small Channel Buffer

**Location:** `lsp/base_service_client/base_service_client.go:276`

**Problem:**
```go
sc.symbolCacheUpdateChan = make(chan uri.URI, 10)
```

Buffer size of only 10 URIs means the goroutine in Prepare() that feeds URIs can get blocked:

```go
for _, uri := range uris {  // 2000 URIs
    sc.symbolCacheUpdateChan <- uri  // Blocks after 10 URIs!
}
```

**Impact:** The producer goroutine wastes time waiting for consumer to process URIs instead of quickly queuing all work

**Solution:** Increase buffer size

### Proposed Fix #2: Larger Channel Buffer

```go
const symbolCacheBufferSize = 100  // or len(uris) for unbounded

sc.symbolCacheUpdateChan = make(chan uri.URI, symbolCacheBufferSize)
```

**Expected Impact:** Minimal blocking, faster work distribution

## Bottleneck #3: Expensive Per-File Processing

**Location:** `lsp/base_service_client/base_service_client.go:518-650`

**What happens for EACH file:**

1. **Read file content** (I/O)
   ```go
   content, err := os.ReadFile(fileURI.Filename())
   ```

2. **Text search for matched symbols** (CPU-intensive)
   ```go
   matchedSymbols := sc.searchContentForWorkspaceSymbols(ctx, string(content), fileURI)
   ```

3. **For EACH matched symbol:**
   - Make `textDocument/definition` LSP RPC call
   - Read definition file content (I/O)
   - Make `textDocument/documentSymbol` LSP RPC call
   - Parse and find symbol at location

**Already Optimized:**
- Parallelizes definition requests with semaphore limiting to 10 concurrent (line 559)
- Skips files already in cache (line 534-537)

**Potential Optimizations:**

### Optimization 3a: Early Skip Check Before Queue

Currently we queue ALL URIs, then skip cached ones during processing:

```go
// In processSymbolCacheUpdate:
if _, exists := sc.symbolCache.GetWorkspaceSymbols(fileURI); exists {
    sc.Log.V(9).Info("Skipping...already exist for file", "uri", fileURI)
    continue
}
```

**Better:** Filter before queuing

```go
// In Prepare():
for _, uri := range uris {
    if _, exists := sc.symbolCache.GetWorkspaceSymbols(uri); !exists {
        sc.symbolCacheUpdateChan <- uri
    } else {
        sc.symbolCacheUpdateWaitGroup.Done()  // Don't forget to decrement!
    }
}
```

**Expected Impact:** Reduced work for incremental analysis (files already cached)

### Optimization 3b: Batch File Reads

Group files by directory and process together to improve disk I/O locality:

```go
// Sort URIs by directory path
sort.Slice(uris, func(i, j int) bool {
    return filepath.Dir(uris[i].Filename()) < filepath.Dir(uris[j].Filename())
})
```

**Expected Impact:** Better disk cache utilization, 10-20% speedup

### Optimization 3c: Increase LSP Request Concurrency

Currently limited to 10 concurrent definition requests per file (line 559):

```go
sem := make(chan struct{}, 10) // limit concurrency to 10
```

**Could increase to:**
```go
sem := make(chan struct{}, 50) // or runtime.NumCPU() * 10
```

**Expected Impact:** Faster if LSP server can handle more concurrent requests

## Bottleneck #4: GetDocumentUris() May Be Slow

**Location:** `lsp/base_service_client/base_service_client.go:362`

```go
uris := sc.symbolSearchHelper.GetDocumentUris(conditionsByCap...)
```

**Need to investigate:**
- How long does this take?
- Does it scan the entire filesystem?
- Can it be cached or optimized?

Let me check the implementation...

## Recommended Optimization Priority

### Phase 1: Quick Wins (30 minutes implementation)

1. **Add multiple worker goroutines** ✅ HIGH IMPACT
   - Change from 1 worker to 10 workers
   - Expected: **10x speedup** for CPU-bound workloads
   - Risk: Low (workers are independent)

2. **Increase channel buffer** ✅ MEDIUM IMPACT
   - Change from 10 to 100 (or dynamic based on file count)
   - Expected: **5-10% speedup** (less blocking)
   - Risk: Minimal (just more memory)

**Combined Phase 1 Impact:** 3 minutes → ~20 seconds (**9x speedup**)

### Phase 2: Medium Effort (1-2 hours implementation)

3. **Early skip check before queueing** ✅ MEDIUM IMPACT (for incremental)
   - Filter cached files before queueing
   - Expected: **2-5x speedup for incremental analysis**
   - Risk: Low (logic change only)

4. **Batch file reads by directory** ✅ LOW-MEDIUM IMPACT
   - Sort URIs by directory before processing
   - Expected: **10-20% speedup**
   - Risk: Low

5. **Make worker count configurable** ✅ LOW IMPACT (usability)
   - Allow tuning via config/env var
   - Expected: Better resource utilization
   - Risk: Minimal

**Combined Phase 1+2 Impact:** 3 minutes → ~15 seconds (**12x speedup**)

### Phase 3: Investigation Needed (2-4 hours)

6. **Profile GetDocumentUris()** 🔍 UNKNOWN IMPACT
   - Measure actual time spent
   - Identify if it's scanning entire filesystem
   - Consider caching results

7. **Increase LSP request concurrency** ⚠️ REQUIRES TESTING
   - Test LSP server capacity
   - May overload language server
   - Expected: 20-50% speedup IF server can handle it

8. **Consider caching strategy improvements** 🔍 COMPLEX
   - Cache at provider level vs per-file level
   - Investigate cache hit rates
   - Consider persistent cache across runs

## Implementation Plan

### Step 1: Add Worker Pool (Quick Win)

**File:** `lsp/base_service_client/base_service_client.go`

**Location 1:** Add constant (around line 40)
```go
const (
    // symbolCacheWorkerCount is the number of concurrent workers
    // processing symbol cache updates during Prepare phase
    symbolCacheWorkerCount = 10
)
```

**Location 2:** Modify initialization (line 276-277)
```go
// Before:
sc.symbolCacheUpdateChan = make(chan uri.URI, 10)
go sc.symbolCacheUpdateHandler()

// After:
sc.symbolCacheUpdateChan = make(chan uri.URI, symbolCacheWorkerCount*2)
for i := 0; i < symbolCacheWorkerCount; i++ {
    go sc.symbolCacheUpdateHandler()
}
```

**That's it!** The existing symbolCacheUpdateHandler already:
- Uses channel for coordination
- Decrements WaitGroup properly
- Handles context cancellation
- Thread-safe symbol cache operations

### Step 2: Increase Channel Buffer

**File:** `lsp/base_service_client/base_service_client.go` (line 276)

```go
const symbolCacheBufferSize = 100

sc.symbolCacheUpdateChan = make(chan uri.URI, symbolCacheBufferSize)
```

### Step 3: Early Cache Skip (Medium Effort)

**File:** `lsp/base_service_client/base_service_client.go` (line 360-367)

```go
// In Prepare():
sc.symbolCacheUpdateWaitGroup.Add(1)
go func() {
    defer sc.symbolCacheUpdateWaitGroup.Done()
    uris := sc.symbolSearchHelper.GetDocumentUris(conditionsByCap...)

    // Filter out already-cached URIs before queueing
    filteredUris := make([]uri.URI, 0, len(uris))
    skipped := 0
    for _, uri := range uris {
        if _, exists := sc.symbolCache.GetWorkspaceSymbols(uri); !exists {
            filteredUris = append(filteredUris, uri)
        } else {
            skipped++
        }
    }

    sc.Log.V(5).Info("Filtered cached files during prepare",
        "total", len(uris), "toProcess", len(filteredUris), "skipped", skipped)

    sc.symbolCacheUpdateWaitGroup.Add(len(filteredUris))
    for _, uri := range filteredUris {
        sc.symbolCacheUpdateChan <- uri
    }
}()
```

**Also remove the skip check from populateDocumentSymbolCache** (line 534-537) since it's now redundant.

## Testing Strategy

### Performance Testing

1. **Baseline measurement:**
   ```bash
   time ./kantra analyze --input tackle2-ui --rules patternfly-rules
   ```
   Record: Total time, prepare time (from logs)

2. **After Phase 1 (worker pool):**
   - Run same command
   - Compare prepare time
   - Verify output unchanged

3. **Stress testing:**
   - Very large codebase (5000+ files)
   - Monitor CPU/memory usage
   - Ensure no goroutine leaks

### Correctness Testing

1. Run existing test suite: `make test`
2. Run demo: `make run-demo`
3. Compare output with baseline (should be identical)
4. Test with different providers (java, nodejs, builtin)

### Monitoring

Add metrics/logging:
```go
sc.Log.Info("Symbol cache preparation stats",
    "totalFiles", len(uris),
    "workers", symbolCacheWorkerCount,
    "durationSeconds", time.Since(start).Seconds())
```

## Expected Results

### Before Optimization (baseline)
```
tackle2-ui analysis (2000 files)
├── Provider init: 2s
├── Prepare phase: 180s  ← BOTTLENECK
├── Rule execution: 45s
└── Total: 227s
```

### After Phase 1 (10 workers)
```
tackle2-ui analysis (2000 files)
├── Provider init: 2s
├── Prepare phase: 20s  ← 9x speedup
├── Rule execution: 45s
└── Total: 67s (70% reduction)
```

### After Phase 1+2 (workers + early skip, incremental run)
```
tackle2-ui analysis (500 new files, 1500 cached)
├── Provider init: 2s
├── Prepare phase: 5s  ← 36x speedup vs baseline
├── Rule execution: 45s
└── Total: 52s (77% reduction)
```

## Risks and Mitigation

### Risk 1: LSP Server Overload
**Symptom:** Increased errors, timeouts, crashed language servers
**Mitigation:**
- Start with conservative worker count (10)
- Add configuration option to tune
- Monitor error rates
- Add rate limiting if needed

### Risk 2: Memory Usage
**Symptom:** OOM errors with many workers
**Mitigation:**
- Each worker uses minimal memory (mostly waiting on I/O)
- Monitor memory usage during testing
- Channel buffer size is bounded

### Risk 3: Race Conditions
**Symptom:** Intermittent test failures, corrupted cache
**Mitigation:**
- Symbol cache already uses proper synchronization (mutexes)
- WaitGroup usage is thread-safe
- Test extensively with `-race` flag

### Risk 4: Goroutine Leaks
**Symptom:** Growing goroutine count, memory leaks
**Mitigation:**
- Workers exit on context cancellation (already implemented)
- Add goroutine count monitoring in tests
- Proper cleanup in Stop()

## Configuration Options

Allow users to tune performance:

```yaml
# analyzer-lsp config
lsp:
  symbolCache:
    workerCount: 10  # number of concurrent workers
    bufferSize: 100  # channel buffer size
    maxConcurrentDefinitionRequests: 10  # per-file concurrency
```

Or via environment variables:
```bash
export ANALYZER_LSP_SYMBOL_CACHE_WORKERS=20
export ANALYZER_LSP_SYMBOL_CACHE_BUFFER=200
```

## Open Questions

1. **Q:** What's the optimal number of workers?
   **A:** Needs experimentation. Likely `runtime.NumCPU()` or `runtime.NumCPU() * 2`

2. **Q:** Does this work with all LSP servers (java, nodejs, python)?
   **A:** Yes, the change is in the generic LSPServiceClientBase used by all

3. **Q:** What about external providers (gRPC)?
   **A:** External providers have their own Prepare() implementation, would need separate optimization

4. **Q:** Can we cache symbol data persistently across runs?
   **A:** Possible future optimization, requires cache invalidation strategy

5. **Q:** How long does GetDocumentUris() take?
   **A:** Need to profile - may be scanning entire filesystem

## Next Steps

1. ✅ Document plan (this file)
2. Create issue in GitHub
3. Implement Phase 1 (worker pool)
4. Measure performance improvement
5. If successful, implement Phase 2
6. Create PR with benchmarks

## Related Issues

- Issue #1009: Add progress reporting during prepare (this enables users to see the speedup)
- Issue #1006: Fix chained condition filepath filtering (unrelated but recently fixed)
