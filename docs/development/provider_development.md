# Provider Development Guide

This guide explains how to create new providers for analyzer-lsp, enabling support for additional languages or analysis capabilities.

## Table of Contents

- [Overview](#overview)
- [Provider Types](#provider-types)
- [Quick Start: Adding a New Language](#quick-start-adding-a-new-language)
- [Provider Architecture](#provider-architecture)
- [Implementing Provider Interfaces](#implementing-provider-interfaces)
- [Adding Capabilities](#adding-capabilities)
- [Testing Your Provider](#testing-your-provider)
- [Best Practices](#best-practices)

## Overview

Providers are the mechanism by which analyzer-lsp performs language-specific analysis. Each provider:

- Implements the `ServiceClient` interface
- Exposes one or more **capabilities** (e.g., `referenced`, `dependency`)
- Communicates with the analyzer via gRPC
- Optionally wraps an LSP server for language analysis

## Provider Types

### 1. Built-in Provider

**Location:** `/provider/internal/builtin`

**Purpose:** File pattern matching, regex, XML/JSON querying without LSP

**Capabilities:**
- `file` - File pattern matching
- `filecontent` - Content search with regex
- `xml` - XML querying with XPath
- `json` - JSON querying with JSONPath
- `hasTags` - Tag checking

**When to use:** Simple pattern matching, non-semantic analysis

### 2. External LSP-Based Providers

**Location:** `/external-providers/`

**Purpose:** Language-specific semantic analysis using LSP servers

**Examples:**
- `java-external-provider` - Java analysis via Eclipse JDTLS
- `generic-external-provider` - Go, Python, Node.js via their LSP servers
- `yq-external-provider` - YAML analysis via yq

**When to use:** Semantic analysis requiring language understanding

### 3. Custom Providers

**Purpose:** Specialized analysis not covered by LSP

**Examples:**
- `golang-dependency-provider` - Go module dependency analysis

**When to use:** Domain-specific analysis beyond LSP capabilities

## Quick Start: Adding a New Language

The easiest way to add support for a new language is using the generic-external-provider with a generic LSP server.

### Prerequisites

Your language must have an LSP server. Check: https://microsoft.github.io/language-server-protocol/implementors/servers/

### Step 1: Install the LSP Server

For example, for Rust:

```bash
rustup component add rust-analyzer
```

### Step 2: Create Provider Configuration

Create a `provider_settings.json` entry:

```json
[
  {
    "name": "rust",
    "binaryPath": "/path/to/generic-external-provider",
    "initConfig": [{
      "location": "/path/to/rust/project",
      "analysisMode": "full",
      "providerSpecificConfig": {
        "lspServerName": "generic",
        "lspServerPath": "rust-analyzer",
        "lspServerArgs": [],
        "lspServerInitializationOptions": {
          "cargo": {
            "buildScripts": {
              "enable": true
            }
          }
        },
        "workspaceFolders": ["file:///path/to/rust/project"],
        "dependencyFolders": []
      }
    }]
  }
]
```

### Step 3: Test the Provider

```bash
# Build generic provider
make external-generic

# Start provider
./build/generic-external-provider --port 14660 --name rust

# In another terminal, run analyzer
go run cmd/analyzer/main.go \
  --provider-settings=provider_settings.json \
  --rules=rust-rules.yaml
```

### Step 4: Write Rules

Create rules using the provider:

```yaml
- ruleID: rust-unsafe-usage
  when:
    rust.referenced:
      pattern: "unsafe"
  message: "Usage of unsafe code detected"
  effort: 3
  category: potential
```

## Provider Architecture

### Communication Flow

```text
Analyzer                     External Provider              LSP Server
    |                               |                            |
    |-- gRPC: Init -------------->|                            |
    |                               |-- Initialize ------------>|
    |                               |<- Initialized ------------|
    |<- InitConfig ----------------|                            |
    |                               |                            |
    |-- gRPC: Prepare ------------>|                            |
    |                               |-- Cache symbols --------->|
    |<- OK -----------------------|                            |
    |                               |                            |
    |-- gRPC: Evaluate ----------->|                            |
    |                               |-- textDocument/* -------->|
    |                               |<- Locations --------------|
    |<- ProviderEvaluateResponse --|                            |
```

### Provider Lifecycle

1. **Initialization**
   - Provider starts (as subprocess or container)
   - Analyzer connects via gRPC
   - Provider starts LSP server
   - LSP server indexes workspace

2. **Preparation**
   - Analyzer sends all rule conditions to provider
   - Provider pre-processes/caches for performance

3. **Evaluation**
   - For each rule, analyzer calls `Evaluate()`
   - Provider translates to LSP request
   - Provider returns incidents

4. **Cleanup**
   - Analyzer calls `Stop()`
   - Provider shuts down LSP server
   - Provider exits

## Implementing Provider Interfaces

### Core Interfaces

Located in `/provider/provider.go`:

```go
// BaseClient - Provider registration and capabilities
type BaseClient interface {
    Capabilities() []Capability
    Init(context.Context, logr.Logger, InitConfig) (ServiceClient, InitConfig, error)
}

// ServiceClient - Provider operations
type ServiceClient interface {
    Prepare(ctx context.Context, conditionsByCap []ConditionsByCap) error
    Evaluate(ctx context.Context, cap string, conditionInfo []byte) (ProviderEvaluateResponse, error)
    GetDependencies(ctx context.Context) (map[uri.URI][]*Dep, error)
    GetDependenciesDAG(ctx context.Context) (map[uri.URI][]DepDAGItem, error)
    NotifyFileChanges(ctx context.Context, changes ...FileChange) error
    Stop()
}

// Client - Full provider interface
type Client interface {
    BaseClient
    ServiceClient
}
```

### Minimal Provider Example

```go
package myprovider

import (
    "context"
    "github.com/konveyor/analyzer-lsp/provider"
    "go.lsp.dev/uri"
)

type MyProvider struct {
    config provider.Config
    // Add your fields
}

// Capabilities returns what this provider can do
func (p *MyProvider) Capabilities() []provider.Capability {
    return []provider.Capability{
        {
            Name: "referenced",
            // Input/Output schemas...
        },
    }
}

// Init initializes the provider
func (p *MyProvider) Init(ctx context.Context, log logr.Logger, config provider.InitConfig) (provider.ServiceClient, provider.InitConfig, error) {
    p.config = config
    // Initialize your provider (e.g., start LSP server)
    return p, config, nil
}

// Prepare pre-processes conditions for faster evaluation
func (p *MyProvider) Prepare(ctx context.Context, conditions []provider.ConditionsByCap) error {
    // Cache/pre-process conditions
    return nil
}

// Evaluate executes a condition and returns incidents
func (p *MyProvider) Evaluate(ctx context.Context, cap string, conditionInfo []byte) (provider.ProviderEvaluateResponse, error) {
    switch cap {
    case "referenced":
        return p.evaluateReferenced(ctx, conditionInfo)
    default:
        return provider.ProviderEvaluateResponse{}, fmt.Errorf("unknown capability: %s", cap)
    }
}

// GetDependencies returns dependency information
func (p *MyProvider) GetDependencies(ctx context.Context) (map[uri.URI][]*provider.Dep, error) {
    return nil, nil  // If provider doesn't support dependencies
}

// GetDependenciesDAG returns dependency DAG
func (p *MyProvider) GetDependenciesDAG(ctx context.Context) (map[uri.URI][]provider.DepDAGItem, error) {
    return nil, nil
}

// NotifyFileChanges handles file change notifications
func (p *MyProvider) NotifyFileChanges(ctx context.Context, changes ...provider.FileChange) error {
    return nil
}

// Stop shuts down the provider
func (p *MyProvider) Stop() {
    // Cleanup (e.g., stop LSP server)
}

func (p *MyProvider) evaluateReferenced(ctx context.Context, conditionInfo []byte) (provider.ProviderEvaluateResponse, error) {
    // Parse condition
    var condition ReferencedCondition
    yaml.Unmarshal(conditionInfo, &condition)

    // Execute analysis
    incidents := []provider.IncidentContext{}
    // ... your logic here ...

    return provider.ProviderEvaluateResponse{
        Matched:   len(incidents) > 0,
        Incidents: incidents,
    }, nil
}
```

## Adding Capabilities

### 1. Define Capability

Capabilities define what your provider can do.

```go
func (p *MyProvider) Capabilities() []provider.Capability {
    return []provider.Capability{
        {
            Name: "referenced",
            Input: provider.SchemaOrRef{
                // Define input schema
            },
            Output: provider.SchemaOrRef{
                // Define output schema
            },
        },
        {
            Name: "dependency",
            Input: provider.SchemaOrRef{...},
            Output: provider.SchemaOrRef{...},
        },
    }
}
```

### 2. Implement Capability Logic

Add logic to `Evaluate()`:

```go
func (p *MyProvider) Evaluate(ctx context.Context, cap string, conditionInfo []byte) (provider.ProviderEvaluateResponse, error) {
    switch cap {
    case "referenced":
        return p.evaluateReferenced(ctx, conditionInfo)
    case "dependency":
        return p.evaluateDependency(ctx, conditionInfo)
    default:
        return provider.ProviderEvaluateResponse{}, fmt.Errorf("unsupported capability: %s", cap)
    }
}
```

### 3. Create Condition Structure

Define the input for your capability:

```go
type ReferencedCondition struct {
    Pattern      string `json:"pattern"`
    Location     string `json:"location,omitempty"`
    AnnotationPattern string `json:"annotationPattern,omitempty"`
}
```

### 4. Test the Capability

Write tests in `*_test.go`:

```go
func TestReferencedCapability(t *testing.T) {
    provider := &MyProvider{}
    provider.Init(context.Background(), logr.Discard(), initConfig)

    condition := `
capability:
  referenced:
    pattern: "my.package.*"
`

    resp, err := provider.Evaluate(context.Background(), "referenced", []byte(condition))

    assert.NoError(t, err)
    assert.True(t, resp.Matched)
    assert.Greater(t, len(resp.Incidents), 0)
}
```

## Creating an LSP-Based Provider

### Using generic-external-provider

For most languages, extend the generic provider:

#### 1. Create Service Client

Create `external-providers/generic-external-provider/pkg/server_configurations/mylang/service_client.go`:

```go
package mylang

import (
    "github.com/konveyor/analyzer-lsp/external-providers/generic-external-provider/pkg/generic"
)

type MyLangServiceClient struct {
    generic.GenericServiceClient
    // Add language-specific fields
}

func (c *MyLangServiceClient) Init(ctx context.Context, log logr.Logger, config InitConfig) error {
    // Custom initialization
    return c.GenericServiceClient.Init(ctx, log, config)
}

// Override capabilities as needed
func (c *MyLangServiceClient) Capabilities() []Capability {
    return []Capability{
        // Your capabilities
    }
}
```

#### 2. Register Service Client

In `constants.go`:

```go
var SupportedLanguages = map[string]ServiceClientConstructor{
    "generic": NewGenericServiceClient,
    "gopls":   NewGoplsServiceClient,
    "pylsp":   NewPylspServiceClient,
    "mylang":  NewMyLangServiceClient,  // Add yours
}
```

#### 3. Build and Test

```bash
# Build
make external-generic

# Create test configuration
cat > provider_settings.json <<EOF
[{
  "name": "mylang",
  "binaryPath": "./build/generic-external-provider",
  "initConfig": [{
    "location": "/path/to/code",
    "providerSpecificConfig": {
      "lspServerName": "mylang",
      "lspServerPath": "/path/to/mylang-lsp"
    }
  }]
}]
EOF

# Test
./build/generic-external-provider --port 14660 --name mylang
```

## Testing Your Provider

### Unit Tests

```go
func TestMyProviderEvaluate(t *testing.T) {
    provider := &MyProvider{}
    // Initialize provider

    tests := []struct {
        name      string
        condition string
        wantMatch bool
        wantErr   bool
    }{
        {
            name: "simple pattern match",
            condition: `referenced: {pattern: "test.*"}`,
            wantMatch: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            resp, err := provider.Evaluate(ctx, "referenced", []byte(tt.condition))
            if (err != nil) != tt.wantErr {
                t.Errorf("unexpected error: %v", err)
            }
            if resp.Matched != tt.wantMatch {
                t.Errorf("expected matched=%v, got %v", tt.wantMatch, resp.Matched)
            }
        })
    }
}
```

### E2E Tests

Create `e2e-tests/` directory:

```
external-providers/mylang-provider/
├── e2e-tests/
│   ├── rule-example.yaml
│   ├── demo-output.yaml
│   └── provider_settings.json
```

**rule-example.yaml:**
```yaml
- ruleID: mylang-test-rule
  when:
    mylang.referenced:
      pattern: "test.*"
  message: "Found test pattern"
  effort: 1
```

**demo-output.yaml:**
```yaml
- name: konveyor-analysis
  violations:
    mylang-test-rule:
      description: ""
      incidents:
        - uri: "file:///path/to/file"
          message: "Found test pattern"
          lineNumber: 42
```

Add Makefile target:

```makefile
test-mylang: run-mylang-provider-pod run-demo-mylang stop-mylang-provider-pod
```

### Integration Testing

Test with analyzer:

```bash
# Start your provider
./build/mylang-external-provider --port 14660

# Run analyzer
go run cmd/analyzer/main.go \
  --provider-settings=provider_settings.json \
  --rules=mylang-rules.yaml \
  --output-file=output.yaml
```

## Best Practices

### 1. Handle Errors Gracefully

```go
func (p *MyProvider) Evaluate(ctx context.Context, cap string, info []byte) (provider.ProviderEvaluateResponse, error) {
    var condition Condition
    if err := yaml.Unmarshal(info, &condition); err != nil {
        return provider.ProviderEvaluateResponse{}, fmt.Errorf("failed to parse condition: %w", err)
    }

    // Validate input
    if condition.Pattern == "" {
        return provider.ProviderEvaluateResponse{}, fmt.Errorf("pattern is required")
    }

    // ... rest of logic
}
```

### 2. Use Context for Cancellation

```go
func (p *MyProvider) Evaluate(ctx context.Context, cap string, info []byte) (provider.ProviderEvaluateResponse, error) {
    select {
    case <-ctx.Done():
        return provider.ProviderEvaluateResponse{}, ctx.Err()
    default:
    }

    // Your logic here

    // Check context periodically in loops
    for _, file := range files {
        select {
        case <-ctx.Done():
            return provider.ProviderEvaluateResponse{}, ctx.Err()
        default:
        }
        // Process file
    }
}
```

### 3. Implement Prepare() for Performance

```go
type MyProvider struct {
    cache map[string]interface{}
}

func (p *MyProvider) Prepare(ctx context.Context, conditions []provider.ConditionsByCap) error {
    p.cache = make(map[string]interface{})

    for _, condByCap := range conditions {
        switch condByCap.Cap {
        case "referenced":
            // Pre-compute or cache data
            for _, cond := range condByCap.Conditions {
                // Parse and cache
            }
        }
    }

    return nil
}
```

### 4. Provide Detailed Incidents

```go
incident := provider.IncidentContext{
    FileURI:    uri.File(filePath),
    LineNumber: &lineNum,
    CodeLocation: &provider.Location{
        StartPosition: provider.Position{Line: startLine, Character: startChar},
        EndPosition:   provider.Position{Line: endLine, Character: endChar},
    },
    Variables: map[string]interface{}{
        "matchedPattern": pattern,
        "symbolName":     symbolName,
    },
}
```

### 5. Document Your Capabilities

Create documentation for users:

```markdown
## MyLang Provider Capabilities

### referenced

Finds references to symbols matching a pattern.

**Input:**
- `pattern` (required): Regex pattern to match
- `location` (optional): File filter

**Example:**
yaml
mylang.referenced:
  pattern: "com.example.*"
  location: "src/**/*.mylang"
```

### 6. Version Your Provider

Follow semantic versioning and document breaking changes.

## Advanced Topics

### Custom gRPC Implementation

For full control, implement gRPC directly:

1. Define protobuf schema
2. Generate Go code
3. Implement gRPC server
4. Register with analyzer

### Dependency Analysis

Implement dependency tracking:

```go
func (p *MyProvider) GetDependencies(ctx context.Context) (map[uri.URI][]*provider.Dep, error) {
    deps := make(map[uri.URI][]*provider.Dep)

    // Parse dependency files (e.g., package.json, go.mod)
    for _, file := range dependencyFiles {
        fileDeps := parseDependencies(file)
        deps[uri.File(file)] = fileDeps
    }

    return deps, nil
}
```

### Progress Reporting

Report progress during long operations:

```go
func (p *MyProvider) Prepare(ctx context.Context, conditions []provider.ConditionsByCap) error {
    if p.progressReporter != nil {
        p.progressReporter.ReportProgress("mylang", 0, len(files))
    }

    for i, file := range files {
        // Process file

        if p.progressReporter != nil {
            p.progressReporter.ReportProgress("mylang", i+1, len(files))
        }
    }

    return nil
}
```

## Next Steps

- Study existing providers in `/external-providers/`
- Review [Architecture](architecture.md) for system design
- Read [Testing Guide](testing.md) for test strategies
- Check out the [LSP Specification](https://microsoft.github.io/language-server-protocol/)
