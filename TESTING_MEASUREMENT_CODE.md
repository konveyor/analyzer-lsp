# Testing the Prepare Phase Measurement Code

## Current Status

✅ **Code Implemented:** Measurement logging added to `Prepare()` method
✅ **Build Successful:** All providers compile with measurement code
✅ **Branch:** `prepare-optimization`
✅ **Commit:** `308938e`

## Challenge: Kantra Uses Containerized Providers

When rebuilding kantra with the local analyzer-lsp, the **kantra binary** itself is updated, but kantra launches providers in containers which use pre-built images that **don't** include the new measurement code.

```bash
# Evidence:
$ podman logs nodejs | head -1
time="2025-11-28T23:21:19Z" level=info msg="server listening at [::]:14654"
# ^ Container from Nov 28, before measurement code was added
```

## Option 1: Rebuild Provider Container Images (Recommended for Full Test)

To see the measurement logs in kantra, you need to rebuild the provider container images:

```bash
cd /Users/tsanders/Workspace/analyzer-lsp

# Build provider images with measurement code
make build-generic-provider   # nodejs, python, golang providers
make build-java-provider      # java provider

# Then use kantra which will use these updated images
cd /Users/tsanders/Workspace/kantra
./kantra analyze --input /path/to/project --rules /path/to/rules

# Check container logs for measurement output
podman logs nodejs | grep "Prepare phase"
```

## Option 2: Use Local analyzer-lsp Directly (Simpler, But Limited)

For a quicker test with built-in providers only:

```bash
cd /Users/tsanders/Workspace/analyzer-lsp

# Run analyzer directly (uses builtin provider, no containers)
./build/konveyor-analyzer \
  --provider-settings examples/provider_settings_builtin.json \
  --rules examples/rule-example.yaml \
  --output-file /tmp/test-output.yaml \
  --verbose 1 2>&1 | grep "Prepare phase"
```

**Limitation:** Builtin provider doesn't use LSPServiceClientBase, so it won't show Prepare phase logs. You'd need to set up external providers manually.

## Option 3: Manual Provider Testing (Most Control)

Run the generic-external-provider directly to test measurement code:

```bash
# Terminal 1: Start nodejs provider with measurement code
./build/generic-external-provider \
  --port 14654 \
  --name nodejs 2>&1 | tee /tmp/nodejs-provider.log

# Terminal 2: Run analyzer pointing to local provider
# (Would need custom provider_settings.json pointing to localhost:14654)

# Check logs
grep "Prepare phase" /tmp/nodejs-provider.log
```

## Expected Log Output

When working correctly, you should see logs like:

```
time="2025-11-29T..." level=info msg="Prepare phase started" provider=nodejs totalFiles=2147 getDocumentUrisDuration=2.34
time="2025-11-29T..." level=info msg="Prepare phase completed" provider=nodejs totalFiles=2147 getDocumentUrisDuration=2.34 processingDuration=186.78 totalDuration=189.12
```

### Interpreting the Results

**If `processingDuration` >> `getDocumentUrisDuration`:**
- **Hypothesis B is correct** - Single worker is the bottleneck
- **Solution:** Implement worker pool (Step 1 of optimization plan)
- **Expected improvement:** 9-10x speedup

**If `getDocumentUrisDuration` is very high (>30s):**
- **Hypothesis A is correct** - Filesystem scanning is slow
- **Solution:** Cache filesystem scan results
- **Expected improvement:** Depends on caching strategy

**Example for tackle2-ui (2147 TypeScript files):**
```
# Hypothesis B (most likely):
getDocumentUrisDuration=12.5     (scanning 2147 files takes ~12s)
processingDuration=178.3          (single worker processing takes ~3 min)
totalDuration=190.8               (total ~3.2 min)

# After worker pool optimization:
getDocumentUrisDuration=12.5     (unchanged)
processingDuration=18.9           (10 workers → 10x faster)
totalDuration=31.4                (total ~31s, 6x overall speedup)
```

## Recommendation

For the most realistic test that will provide actionable data:

1. **Rebuild provider containers** (Option 1)
2. **Run kantra on tackle2-ui** with comprehensive PatternFly rules
3. **Extract timing logs** from nodejs container
4. **Analyze** whether GetDocumentUris or processing is the bottleneck
5. **Implement** appropriate optimization based on data

## Quick Test Script

```bash
#!/bin/bash
# test-measurement-code.sh

set -e

echo "Building provider images with measurement code..."
cd /Users/tsanders/Workspace/analyzer-lsp
make build-generic-provider

echo "Running kantra analysis..."
cd /Users/tsanders/Workspace/kantra
./kantra analyze \
  --input /Users/tsanders/Workspace/tackle2-ui \
  --output /tmp/measurement-test-output \
  --rules /Users/tsanders/Workspace/analyzer-rule-generator/examples/output/patternfly-v6/comprehensive \
  --overwrite

echo "Extracting measurement logs..."
podman logs nodejs 2>&1 | grep "Prepare phase" | tee /tmp/prepare-phase-measurements.txt

echo "Results:"
cat /tmp/prepare-phase-measurements.txt
```

## Next Steps After Getting Measurements

1. **Analyze timing data** to confirm bottleneck
2. **Implement worker pool** if processing is slow (most likely)
3. **Re-test** to measure improvement
4. **Create PR** with benchmarks showing before/after performance
