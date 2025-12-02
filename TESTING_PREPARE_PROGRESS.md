# Testing Prepare Progress Reporting

This document describes how to test the progress reporting feature for the Prepare() phase.

## Unit Tests

Unit tests for the PrepareProgressAdapter are located in `provider/prepare_progress_test.go`.

Run them with:
```bash
go test ./provider -v -run TestPrepareProgress
```

Tests cover:
- ✅ Nil reporter handling
- ✅ Valid reporter creation
- ✅ Progress reporting with correct values
- ✅ Concurrent progress reporting
- ✅ Interface compliance

## Integration Testing

### Option 1: Quick Test with Existing Codebase

The easiest way to test is to run the analyzer on a real codebase and observe the progress output:

```bash
# Build the analyzer
go build ./cmd/analyzer

# Run on a TypeScript/JavaScript project (e.g., tackle2-ui)
./analyzer \
  --rules=/path/to/rules.yaml \
  --provider-settings=/path/to/settings.yaml \
  --output-file=output.yaml \
  --log-level=0

# You should see output like:
# Preparing nodejs provider... 10/500 files (2.0%)
# Preparing nodejs provider... 215/500 files (43.0%)
# Preparing nodejs provider... 500/500 files (100.0%)
```

### Option 2: Test with Text Reporter

Create a simple test settings file:

```yaml
# test-settings.yaml
- name: nodejs
  location: /path/to/your/project
  providerSpecificConfig:
    lspServerPath: /path/to/typescript-language-server
    lspServerName: typescript-language-server
```

Run with text progress reporting (default):
```bash
./analyzer \
  --rules=/path/to/rules.yaml \
  --provider-settings=test-settings.yaml \
  --output-file=output.yaml
```

### Option 3: Test with JSON Progress Reporter

For programmatic testing:

```bash
# This will output JSON progress events to stderr
./analyzer \
  --rules=/path/to/rules.yaml \
  --provider-settings=test-settings.yaml \
  --output-file=output.yaml \
  2> progress.json

# Then examine progress.json for provider_prepare events:
jq 'select(.stage == "provider_prepare")' progress.json
```

## Expected Progress Events

During the Prepare() phase, you should see events with:

```json
{
  "timestamp": "2024-01-15T10:30:00Z",
  "stage": "provider_prepare",
  "message": "Preparing nodejs provider",
  "current": 215,
  "total": 2147,
  "percent": 10.01,
  "metadata": {
    "provider": "nodejs"
  }
}
```

Progress events are throttled to ~500ms intervals, so you won't see one for every single file.

## Validating the Feature

### 1. **Progress Updates Appear**
   - Progress should update regularly (every ~500ms)
   - No silent pauses > 1 second during Prepare()

### 2. **Correct Counts**
   - `current` should increment from 0 to `total`
   - `total` should match number of files being processed
   - `percent` should be calculated correctly

### 3. **Provider Name**
   - `message` should include the provider name (e.g., "nodejs", "java")
   - `metadata.provider` should match the provider name

### 4. **Performance**
   - Progress reporting should not significantly slow down Prepare()
   - Overhead should be minimal (< 1% performance impact)

### 5. **Concurrent Providers**
   - If multiple providers are active, each should report independently
   - No race conditions or garbled output

## Manual Testing Checklist

- [ ] Progress appears during Prepare() phase
- [ ] Progress updates at reasonable intervals (~500ms)
- [ ] File counts are accurate
- [ ] Provider names are correct
- [ ] No crashes or panics
- [ ] Performance is acceptable
- [ ] Works with text reporter
- [ ] Works with JSON reporter
- [ ] Works with multiple providers
- [ ] Gracefully handles nil reporter (no crashes)

## Troubleshooting

### No Progress Appears

Check that:
1. You're using the built analyzer from this branch
2. The provider is actually processing files (check logs)
3. Progress reporting is enabled (not disabled in config)

### Incorrect File Counts

The file count represents files processed for symbol caching, not all files in the workspace. This is expected and depends on:
- Which rules are being used
- What symbols the rules reference
- File type filters (e.g., only .ts/.tsx/.js/.jsx for nodejs)

### Performance Issues

If progress reporting causes slowness:
1. Check throttling is working (should be ~500ms)
2. Verify reporter implementation is efficient
3. Look for excessive logging or I/O in reporter

## Example Test Session

Here's a complete example test session:

```bash
# 1. Build latest
cd /path/to/analyzer-lsp
git checkout feature/prepare-progress-reporting
go build ./cmd/analyzer

# 2. Prepare test project
cd /tmp
git clone https://github.com/konveyor/tackle2-ui
cd tackle2-ui
npm install

# 3. Create minimal rules
cat > test-rules.yaml <<EOF
- ruleID: test-button-usage
  when:
    nodejs.referenced:
      pattern: Button
  message: Found Button usage
EOF

# 4. Create settings
cat > test-settings.yaml <<EOF
- name: nodejs
  location: $(pwd)
  providerSpecificConfig:
    lspServerPath: $(which typescript-language-server)
    lspServerName: typescript-language-server
EOF

# 5. Run analyzer and watch progress
/path/to/analyzer-lsp/analyzer \
  --rules=test-rules.yaml \
  --provider-settings=test-settings.yaml \
  --output-file=output.yaml \
  --log-level=0

# Expected output:
# Initializing nodejs provider
# Provider nodejs ready
# Preparing nodejs provider... 1/215 files (0.5%)
# Preparing nodejs provider... 50/215 files (23.3%)
# Preparing nodejs provider... 100/215 files (46.5%)
# Preparing nodejs provider... 150/215 files (69.8%)
# Preparing nodejs provider... 200/215 files (93.0%)
# Preparing nodejs provider... 215/215 files (100.0%)
# Parsing rules...
# [etc.]
```

## Automated Testing

To add to CI/CD:

```bash
# Run unit tests
go test ./provider -v -run TestPrepareProgress

# Run a smoke test with real analyzer
./analyzer \
  --rules=testdata/rules.yaml \
  --provider-settings=testdata/settings.yaml \
  --output-file=/tmp/output.yaml \
  2>&1 | grep -q "Preparing.*provider"

if [ $? -eq 0 ]; then
  echo "✓ Progress reporting working"
else
  echo "✗ Progress reporting not working"
  exit 1
fi
```

## Related Files

- `provider/prepare_progress.go` - Adapter implementation
- `provider/prepare_progress_test.go` - Unit tests
- `lsp/base_service_client/base_service_client.go` - Progress tracking logic
- `progress/progress.go` - Progress event definitions
- `cmd/analyzer/main.go` - Integration point
