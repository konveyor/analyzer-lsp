# Testing Guide

This document explains how to run tests in the analyzer-lsp project, including unit tests, E2E tests, and the `make test-all` workflow.

## Table of Contents

- [Quick Start](#quick-start)
- [Test Types](#test-types)
- [Running Unit Tests](#running-unit-tests)
- [Running E2E Tests](#running-e2e-tests)
- [Understanding make test-all](#understanding-make-test-all)
- [Writing Tests](#writing-tests)
- [Debugging Failed Tests](#debugging-failed-tests)

## Quick Start

```bash
# Run all unit tests
go test ./...

# Run all E2E tests
make test-all

# Run specific provider E2E tests
make test-java
make test-generic
make test-yaml

# Run just the analyzer integration test
make test-analyzer
```

## Test Types

### 1. Unit Tests

Unit tests are Go test files (`*_test.go`) located throughout the codebase. They test individual functions and packages in isolation.

**Location:** Alongside source files in each package

**Examples:**
- `engine/engine_test.go`
- `parser/rule_parser_test.go`
- `provider/provider_test.go`

### 2. E2E (End-to-End) Tests

E2E tests validate that providers work correctly by running the full analyzer with real rules and comparing output against expected results.

**Location:** `external-providers/*/e2e-tests/`

**Structure:**
```
external-providers/
├── java-external-provider/e2e-tests/
│   ├── rule-example.yaml          # Java-specific test rules
│   ├── demo-output.yaml           # Expected output
│   └── provider_settings.json     # Provider configuration
├── generic-external-provider/e2e-tests/
│   ├── golang-e2e/
│   ├── python-e2e/
│   └── nodejs-e2e/
└── yq-external-provider/e2e-tests/
    ├── rule-example.yaml
    ├── demo-output.yaml
    └── provider_settings.json
```

### 3. Integration Tests

Integration tests run the complete analyzer with all providers to validate multi-provider scenarios.

**Location:** Root-level test files
- `rule-example.yaml`
- `demo-output.yaml`
- `provider_pod_local_settings.json`

### 4. Benchmarks

Performance benchmarks measure the speed of critical operations.

**Location:** `/benchmarks`

**Running benchmarks:**
```bash
go test -bench=. -benchmem ./benchmarks/...

# Java dependency index benchmark
make run-index-benchmark
```

## Running Unit Tests

### Run All Unit Tests

```bash
go test ./...
```

### Run Tests for a Specific Package

```bash
go test ./engine/...
go test ./parser/...
go test ./provider/...
```

### Run Tests with Verbose Output

```bash
go test -v ./...
```

### Run Tests with Coverage

```bash
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Run a Specific Test

```bash
go test ./engine -run TestProcessRule
```

## Running E2E Tests

E2E tests use container-based providers to simulate real-world usage.

### Prerequisites

1. **Build the images:**
   ```bash
   make build-external
   ```

   This builds:
   - `localhost/analyzer-lsp:latest` - Main analyzer
   - `localhost/java-provider:latest` - Java provider
   - `localhost/generic-provider:latest` - Go/Python/Node.js provider
   - `localhost/yq-provider:latest` - YAML provider

2. **Ensure a container tool is installed and running**

### Test Individual Providers

#### Java Provider

```bash
make test-java
```

**What happens:**
1. Creates a `test-data` volume
2. Copies Java example files to the volume
3. Starts `analyzer-java` pod with Java provider container
4. Runs analyzer with Java-specific rules
5. Compares output against `demo-output.yaml`
6. Cleans up pod and volume

**Manual steps:**
```bash
# Start provider
make run-java-provider-pod

# Run test
make run-demo-java

# Check logs if needed
podman logs java-provider

# Stop provider
make stop-java-provider-pod
```

#### Generic Provider (Go/Python/Node.js)

```bash
make test-generic
```

This runs tests for all three languages sequentially:
- `test-golang`
- `test-python`
- `test-nodejs`

**Individual language tests:**
```bash
make test-golang
make test-python
make test-nodejs
```

#### YAML Provider

```bash
make test-yaml
```

**What happens:**
1. Starts YQ provider for YAML analysis
2. Runs analyzer with YAML-specific rules
3. Validates Kubernetes manifest detection
4. Cleans up

### Test All Providers

```bash
make test-all-providers
```

Runs all provider-specific tests:
- Java
- Go
- Python
- Node.js
- YAML

### Test Full Integration

```bash
make test-analyzer
```

Runs the complete analyzer with all providers running simultaneously in a single pod.

## Understanding make test-all

The `make test-all` target is the comprehensive test suite that validates the entire system.

### Execution Flow

```bash
make test-all
```

**Steps executed:**

1. **test-all-providers** - Tests each provider individually
   - `make test-java` - Java provider E2E tests
   - `make test-generic` - Generic provider E2E tests (Go, Python, Node.js)
   - `make test-yaml` - YAML provider E2E tests

2. **test-analyzer** - Full integration test
   - `make run-external-providers-pod` - Start all providers in one pod
   - `make run-demo-image` - Run analyzer with all providers
   - `make stop-external-providers-pod` - Clean up

### How E2E Tests Work

Each E2E test follows this pattern:

1. **Setup Phase**
   ```bash
   # Create volume for test data
   podman volume create test-data

   # Copy example files to volume
   podman run --rm -v test-data:/target -v $(PWD)/examples:/src \
       --entrypoint=cp alpine -a /src/. /target/

   # Create pod
   podman pod create --name=analyzer-java

   # Start provider container
   podman run --pod analyzer-java --name java-provider -d \
       -v test-data:/analyzer-lsp/examples \
       localhost/java-provider:latest --port 14651
   ```

2. **Execution Phase**
   ```bash
   # Run analyzer in the same pod
   podman run --entrypoint /usr/local/bin/konveyor-analyzer \
       --pod=analyzer-java \
       -v test-data:/analyzer-lsp/examples \
       -v $(PWD)/external-providers/java-external-provider/e2e-tests/demo-output.yaml:/analyzer-lsp/output.yaml \
       -v $(PWD)/external-providers/java-external-provider/e2e-tests/provider_settings.json:/analyzer-lsp/provider_settings.json \
       -v $(PWD)/external-providers/java-external-provider/e2e-tests/rule-example.yaml:/analyzer-lsp/rule-example.yaml \
       localhost/analyzer-lsp:latest \
       --output-file=/analyzer-lsp/output.yaml \
       --rules=/analyzer-lsp/rule-example.yaml \
       --provider-settings=/analyzer-lsp/provider_settings.json
   ```

3. **Cleanup Phase**
   ```bash
   # Kill and remove pod
   podman pod kill analyzer-java
   podman pod rm analyzer-java

   # Remove volume
   podman volume rm test-data
   ```

Verification of the results is done with git, this workflow also allows you to re-generate the results and commit them, if there should be changes.

### Test Configuration Files

Each E2E test requires three files:

#### 1. rule-example.yaml

Defines rules specific to the provider being tested.

Example (Java):
```yaml
- ruleID: java-servlet-reference
  when:
    java.referenced:
      pattern: "javax.servlet.*"
  message: "Found reference to Java Servlet API"
  effort: 3
  category: mandatory
```

#### 2. demo-output.yaml

Expected output from the analyzer. This is what the test validates against.

Example structure:
```yaml
- name: konveyor-analysis
  violations:
    java-servlet-reference:
      description: "..."
      category: mandatory
      incidents:
        - uri: "file:///analyzer-lsp/examples/java/..."
          message: "Found reference to Java Servlet API"
          lineNumber: 42
```

#### 3. provider_settings.json

Provider configuration for the test.

Example (Java):
```json
[
  {
    "name": "java",
    "address": "127.0.0.1:14651",
    "initConfig": [
      {
        "location": "/analyzer-lsp/examples/java",
        "analysisMode": "full",
        "providerSpecificConfig": {
          "lspServerPath": "/jdtls/bin/jdtls",
          "bundles": "/jdtls/java-analyzer-bundle/java-analyzer-bundle.core/target/java-analyzer-bundle.core-1.0.0-SNAPSHOT.jar"
        }
      }
    ]
  }
]
```

## Writing Tests

### Writing Unit Tests

Unit tests follow standard Go testing conventions:

```go
// engine/engine_test.go
package engine

import "testing"

func TestProcessRule(t *testing.T) {
    // Setup
    rule := Rule{
        RuleID: "test-rule",
        When: SimpleCondition{...},
    }

    // Execute
    result, err := processRule(ctx, rule, ruleCtx, logger)

    // Assert
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if !result.Matched {
        t.Error("expected rule to match")
    }
}
```

### Writing E2E Tests

To add a new E2E test for a provider:

1. **Add rule to `e2e-tests/rule-example.yaml`:**
   ```yaml
   - ruleID: my-new-test
     when:
       java.referenced:
         pattern: "my.package.*"
     message: "Test message"
     effort: 1
   ```

2. **Add expected output to `e2e-tests/demo-output.yaml`:**
   ```yaml
   - name: konveyor-analysis
     violations:
       my-new-test:
         description: "..."
         incidents:
           - uri: "file:///analyzer-lsp/examples/..."
             message: "Test message"
   ```

3. **Run the test:**
   ```bash
   make test-java  # or appropriate provider
   ```

### Writing Benchmarks

```go
// benchmarks/rule_bench_test.go
package benchmarks

import "testing"

func BenchmarkRuleProcessing(b *testing.B) {
    // Setup
    engine := CreateRuleEngine(...)
    rules := loadTestRules()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        engine.RunRules(context.Background(), rules)
    }
}
```

## Debugging Failed Tests

### Unit Test Failures

1. **Run test with verbose output:**
   ```bash
   go test -v ./engine -run TestFailingTest
   ```

2. **Add debug logging:**
   ```go
   t.Logf("Debug info: %+v", someValue)
   ```

3. **Use a debugger:**

For full setup see [Debuging documentation](../../debug/README.md).
   ```bash
   # Install delve
   go install github.com/go-delve/delve/cmd/dlv@latest

   # Debug test
   dlv test ./engine -- -test.run TestFailingTest
   ```

### E2E Test Failures

1. **Check provider logs:**
   ```bash
   podman logs java-provider
   podman logs golang-provider
   ```

2. **Inspect actual output:**
   ```bash
   # The output is written to the mounted demo-output.yaml
   cat external-providers/java-external-provider/e2e-tests/demo-output.yaml
   ```

3. **Run provider manually:**
   ```bash
   # Start provider pod
   make run-java-provider-pod

   # Don't stop it - inspect while running
   podman exec -it java-provider /bin/sh

   # When done
   make stop-java-provider-pod
   ```

4. **Compare expected vs actual:**
   ```bash
   git diff main -- <path_to_output_file>
   ```

5. **Check analyzer logs:**
   ```bash
   # Analyzer runs in a container, so check its output
   # You may need to run it with additional verbosity

   # Edit the Makefile temporarily to add --log-level=9
   make run-demo-java
   ```

### Common Issues

#### Pod Already Exists
```bash
# Error: pod already exists
make stop-java-provider-pod  # Clean up old pod
make test-java               # Try again
```

#### Volume Already Exists
```bash
# Error: volume test-data already exists
podman volume rm test-data
make test-java
```

#### Provider Not Starting
```bash
# Check if port is already in use
netstat -an | grep 14651

# Check provider container logs
podman logs java-provider
```

#### Test Output Doesn't Match

Common causes:
- File paths differ (check URI formatting)
- Line numbers off by one (check line counting logic)
- Extra/missing incidents (check rule conditions)
- Order of incidents changed (output is sorted)

**Fix:**
1. Review actual vs expected output carefully
2. If actual is correct, update `demo-output.yaml`
3. If actual is wrong, debug the provider or rule

## Test Data

### Examples Directory

Test data is located in `/examples`:

```
examples/
├── java/          # Java test projects
├── golang/        # Go test projects
├── python/        # Python test projects
├── nodejs/        # Node.js test projects
├── yaml/          # YAML/K8s manifests
└── builtin/       # Test files for builtin provider
```

### Provider-Specific Examples

Providers may have additional examples:
```
external-providers/java-external-provider/examples/
```

## Continuous Integration

The test suite is designed to run in CI/CD pipelines:

```bash
# Full test suite for CI
make test-all
```

This runs:
- All provider E2E tests
- Full integration test
- Validates all providers work independently and together

## Test Coverage

To measure test coverage across the project:

```bash
# Generate coverage for all packages
go test -coverprofile=coverage.out ./...

# View coverage in browser
go tool cover -html=coverage.out

# View coverage by package
go tool cover -func=coverage.out
```

## Performance Testing

### Running Benchmarks

```bash
# Run all benchmarks
go test -bench=. -benchmem ./benchmarks/...

# Run specific benchmark
go test -bench=BenchmarkRuleProcessing -benchmem ./benchmarks/...

# Run with longer benchtime
go test -bench=. -benchtime=10s ./benchmarks/...
```

### Java Index Benchmark

Special benchmark for Java dependency indexing:

```bash
make run-index-benchmark
```

## Best Practices

1. **Always run tests before committing:**
   ```bash
   go test ./...
   ```

2. **Add tests for new features:**
   - Unit tests for new functions
   - E2E tests for new provider capabilities

3. **Update E2E expected output when behavior changes:**
   - Review changes carefully
   - Update `demo-output.yaml` files

4. **Clean up test resources:**
   - Make targets handle cleanup
   - Don't leave pods/volumes running

5. **Use descriptive test names:**
   ```go
   func TestRuleEngineProcessesTaggingRulesFirst(t *testing.T)
   ```

6. **Test error cases:**
   ```go
   func TestRuleParserHandlesMalformedYAML(t *testing.T)
   ```

## Next Steps

- [Development Setup](setup.md) - Set up your development environment
- [Provider Development](provider_development.md) - Build new providers
- [Architecture](architecture.md) - Understand the codebase structure
