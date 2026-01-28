# Architecture Overview

This document provides an architectural overview of the analyzer-lsp codebase, including package organization, key interfaces, and system design.

## Table of Contents

- [High-Level Architecture](#high-level-architecture)
- [Top-Level Packages](#top-level-packages)
- [Data Flow](#data-flow)
- [Key Interfaces and Types](#key-interfaces-and-types)
- [Provider Architecture](#provider-architecture)

## High-Level Architecture

Analyzer-lsp is a rule-based code analysis engine designed for application migration and modernization. It uses Language Server Protocol (LSP) to perform semantic code analysis across multiple languages.

```
┌─────────────────────────────────────────────────────────────┐
│                    CLI (cmd/analyzer)                        │
│  Parse arguments, load rules & provider config               │
└────────────────────┬────────────────────────────────────────┘
                     │
┌────────────────────▼────────────────────────────────────────┐
│                    Parser                                    │
│  Parse YAML rules into engine structures                     │
└────────────────────┬────────────────────────────────────────┘
                     │
┌────────────────────▼────────────────────────────────────────┐
│                    Engine                                    │
│  - Execute rules with providers                              │
│  - Collect violations & generate output                      │
│  - Apply label selectors & incident filters                  │
└────────┬───────────────────────────┬────────────────────────┘
         │                           │
         │                           │
┌────────▼─────────┐        ┌────────▼─────────────────────┐
│   Builtin        │        │  External Providers           │
│   Provider       │        │   (gRPC/LSP-based)            │
│                  │        │                               │
│ - File matching  │        │ - Java (JDTLS)                │
│ - Regex          │        │ - Generic (Go/Python/Node.js) │
│ - XML/JSON       │        │ - YAML (yq)                   │
│ - Tag checking   │        │ - Dependency analysis         │
└──────────────────┘        └───────────────┬───────────────┘
                                            │
                                   ┌────────▼────────────┐
                                   │  LSP Language       │
                                   │  Servers            │
                                   │                     │
                                   │ - gopls (Go)        │
                                   │ - pylsp (Python)    │
                                   │ - typescript-ls     │
                                   │ - jdtls (Java)      │
                                   └─────────────────────┘
```

## Top-Level Packages

### Core Engine and Analysis

#### `/engine`
**Purpose:** Rule execution engine that evaluates rules against source code.

**Key Types:**
- `RuleEngine` - Main interface for running analysis rules
- `Rule` - Represents a single analysis rule with conditions and actions
- `RuleSet` - Collection of related rules
- `ConditionResponse` - Result of evaluating a condition
- `IncidentContext` - Information about a specific code issue found

**Exposed Interfaces:**
```go
type RuleEngine interface {
    RunRules(ctx context.Context, rules []RuleSet, selectors ...RuleSelector) []konveyor.RuleSet
    RunRulesWithOptions(ctx context.Context, rules []RuleSet, opts []RunOption, selectors ...RuleSelector) []konveyor.RuleSet
    Stop()
}
```

**Key Functions:**
- `CreateRuleEngine()` - Factory function to create a new rule engine with worker pool
- `processRule()` - Evaluates a single rule against its conditions

**Entry Point:** `engine/engine.go`

---

#### `/engine/labels`
**Purpose:** Label matching and selector logic for filtering rules and dependencies.

**Key Types:**
- `LabelSelector[T]` - Generic label selector for filtering based on label expressions

---

#### `/parser`
**Purpose:** Parses YAML rule files and rulesets into engine-compatible structures.

**Key Types:**
- `RuleParser` - Main parser for loading rules from filesystem

**Exposed Interfaces:**
```go
type RuleParser struct {
    ProviderNameToClient map[string]provider.InternalProviderClient
    Log                  logr.Logger
    NoDependencyRules    bool
    DepLabelSelector     *labels.LabelSelector[*provider.Dep]
}
```

**Key Functions:**
- `LoadRules(filepath string)` - Load rules from a file or directory
- `LoadRule(filepath string)` - Load rules from a single file

**Entry Point:** `parser/rule_parser.go`

---

### Provider System

#### `/provider`
**Purpose:** Provider abstraction layer and interfaces for language-specific analysis.

**Key Types:**
- `ServiceClient` - Interface for provider capabilities (Prepare, Evaluate, GetDependencies)
- `Config` - Provider configuration structure
- `InitConfig` - Initialization configuration for providers
- `ProviderEvaluateResponse` - Response from provider evaluation
- `IncidentContext` - Details about a code incident
- `Capability` - Defines a provider's capability with input/output schemas
- `Dep` - Dependency information
- `DepDAGItem` - Dependency directed acyclic graph item

**Exposed Interfaces:**
```go
type ServiceClient interface {
    Prepare(ctx context.Context, conditionsByCap []ConditionsByCap) error
    Evaluate(ctx context.Context, cap string, conditionInfo []byte) (ProviderEvaluateResponse, error)
    GetDependencies(ctx context.Context) (map[uri.URI][]*Dep, error)
    GetDependenciesDAG(ctx context.Context) (map[uri.URI][]DepDAGItem, error)
    NotifyFileChanges(ctx context.Context, changes ...FileChange) error
    Stop()
}

type Client interface {
    BaseClient
    ServiceClient
}

type BaseClient interface {
    Capabilities() []Capability
    Init(context.Context, logr.Logger, InitConfig) (ServiceClient, InitConfig, error)
}
```

**Key Functions:**
- `GetConfig(filepath string)` - Load provider configuration from file
- `HasCapability()` - Check if provider has a specific capability

**Entry Point:** `provider/provider.go`

---

#### `/provider/internal/builtin`
**Purpose:** Built-in provider for file/text pattern matching without LSP.

**Capabilities:**
- File pattern matching
- Regular expression search
- XML/JSON querying
- Tag checking

---

#### `/provider/grpc`
**Purpose:** gRPC-based provider implementation for external language providers.

---

### LSP Infrastructure

#### `/lsp/protocol`
**Purpose:** LSP 3.17 protocol types and definitions (generated from TypeScript spec).

**Key Types:**
- Full LSP 3.17 protocol types
- Request/response structures
- Notification types

**Entry Point:** `lsp/protocol/tsprotocol.go`

---

#### `/lsp/base_service_client`
**Purpose:** Base LSP client implementation with caching and common handlers.

**Key Features:**
- Symbol cache for performance
- Await cache for request/response management
- Standard LSP initialization and communication

**Entry Point:** `lsp/base_service_client/base_service_client.go`

---

#### `/jsonrpc2_v2`
**Purpose:** JSON-RPC 2.0 implementation (copied from gopls internal package).

**Key Features:**
- Bidirectional JSON-RPC communication
- Connection management
- Request/response handling

---

### External Providers

#### `/external-providers/java-external-provider`
**Purpose:** Java analysis using Eclipse JDTLS (Java Development Tools Language Server).

**Capabilities:**
- Java semantic analysis
- Reference finding
- Type hierarchy analysis
- Dependency resolution

---

#### `/external-providers/generic-external-provider`
**Purpose:** Generic LSP wrapper for Go, Python, and Node.js analysis.

**Capabilities:**
- Symbol references
- Type definitions
- Workspace symbols
- Generic LSP operations

---

#### `/external-providers/golang-dependency-provider`
**Purpose:** Go module dependency analysis.

**Capabilities:**
- Go module parsing
- Dependency graph construction
- Version resolution

---

#### `/external-providers/yq-external-provider`
**Purpose:** YAML and Kubernetes manifest analysis using yq.

**Capabilities:**
- YAML querying
- Kubernetes resource analysis
- Configuration validation

---

### Output and Reporting

#### `/output/v1/konveyor`
**Purpose:** Analysis output data structures for violations and incidents.

**Key Types:**
```go
type RuleSet struct {
    Name        string
    Description string
    Tags        []string
    Violations  map[string]Violation
    Insights    map[string]Violation
    Errors      map[string]string
    Unmatched   []string
    Skipped     []string
}

type Violation struct {
    Description string
    Category    *Category
    Labels      []string
    Incidents   []Incident
    Links       []Link
    Effort      *int
}

type Incident struct {
    URI        uri.URI
    Message    string
    CodeSnip   string
    LineNumber *int
    Variables  map[string]interface{}
}
```

**Entry Point:** `output/v1/konveyor/violations.go`

---

#### `/progress`
**Purpose:** Progress reporting infrastructure for long-running operations.

**Key Types:**
- `ProgressReporter` - Interface for reporting progress
- Multiple implementations (JSON, text, progress bar, channel-based)

---

### Utilities

#### `/tracing`
**Purpose:** OpenTelemetry tracing support for observability.

---

#### `/event`
**Purpose:** Event handling infrastructure for telemetry and logging.

---

### Entry Points

#### `/cmd/analyzer`
**Purpose:** Main analyzer CLI binary.

**Responsibilities:**
- Command-line argument parsing
- Configuration loading
- Provider initialization
- Rule loading and execution
- Output generation

**Entry Point:** `cmd/analyzer/main.go`

---

#### `/cmd/dep`
**Purpose:** Dependency analyzer CLI binary for generating dependency reports.

**Entry Point:** `cmd/dep/main.go`

---

## Data Flow

### 1. Initialization Phase

```
User invokes CLI
    ↓
CLI loads provider configuration (provider_settings.json)
    ↓
CLI loads rules (YAML files)
    ↓
Parser converts YAML → engine.Rule structures
    ↓
Engine initializes providers (builtin + external via gRPC)
    ↓
Providers initialize LSP servers
```

### 2. Analysis Phase

```
Engine receives rules from parser
    ↓
Engine calls Prepare() on all providers with conditions
    ↓
For each rule:
    Engine delegates to provider based on condition type
        ↓
    Provider queries LSP server or analyzes files
        ↓
    Provider returns ProviderEvaluateResponse
        ↓
    Matches generate IncidentContext with code snippets
    ↓
Engine aggregates incidents into Violations
    ↓
Engine groups Violations into RuleSets
```

### 3. Output Phase

```
Engine returns completed RuleSets
    ↓
CLI serializes to YAML
    ↓
Output written to file
```

## Key Interfaces and Types

### Condition System

Rules use a flexible condition system:

- **Simple Condition:** Single provider capability check (e.g., `java.referenced`)
- **AND Condition:** All conditions must have incidents
- **OR Condition:** At least one condition must have incidents
- **NOT Condition:** Condition must not have incidents
- **Chained Conditions:** Use `from` and `as` to pass results between conditions

Example:
```yaml
when:
  or:
  - java.referenced:
      pattern: "javax.servlet.*"
    from: java-files
  - builtin.file:
      pattern: "*.java"
    as: java-files
```

### Provider Capabilities

Each provider exposes capabilities, examples include:

- **referenced:** Find references to a pattern
- **dependency:** Check for specific dependencies
- **xml:** Query XML files
- **json:** Query JSON files
- **hasTags:** Check for generated tags
- **filecontent:** Search file contents

### Label System

Labels are used for:
- **Rule filtering:** Select which rules to run
- **Dependency filtering:** Filter dependencies by labels

## Provider Architecture

### Built-in Provider

The built-in provider runs in-process and handles:
- File pattern matching (glob, regex)
- Regular expression search for file content
- XML/JSON querying (XPath, JSONPath)
- Tag checking

### External Providers

External providers run as separate processes and communicate via gRPC over tcp/ip or a socket(mac/linux) or a named pipe(windows):

1. **Initialization:** Provider receives InitConfig with code location and settings
2. **Preparation:** Provider receives all conditions to optimize caching
3. **Evaluation:** Provider evaluates conditions and returns incidents
4. **Cleanup:** Provider shuts down gracefully

External providers typically wrap LSP servers:
- Start LSP server as subprocess
- Initialize LSP server with workspace
- Translate analyzer capabilities to LSP requests
- Return results in analyzer format

### Provider Communication

```
Analyzer                     External Provider              LSP Server
    |                               |                            |
    |-- Init(config) ------------->|                            |
    |                               |-- Initialize ------------>|
    |                               |<- Initialized ------------|
    |<- Ready --------------------|                            |
    |                               |                            |
    |-- Prepare(conditions) ------>|                            |
    |                               |-- Various LSP requests -->|
    |                               |<- Responses --------------|
    |<- OK -----------------------|                            |
    |                               |                            |
    |-- Evaluate(condition) ------>|                            |
    |                               |-- textDocument/references->|
    |                               |<- Locations --------------|
    |<- ProviderEvaluateResponse --|                            |
```

## Package Dependencies

```
cmd/analyzer
    ├── parser
    │   ├── engine
    │   └── provider
    ├── engine
    │   ├── labels
    │   ├── output
    │   └── provider
    └── provider
        ├── lsp
        ├── jsonrpc2_v2
        └── output

external-providers/*
    ├── provider (shared interface)
    └── lsp
```

## Testing Structure

- **Unit tests:** `*_test.go` files throughout codebase
- **E2E tests:** Provider-specific tests in `external-providers/*/e2e-tests/`
- **Benchmarks:** Performance tests `*_bench_test.go`
- **Examples:** Sample projects in `/examples` for testing

For detailed testing information, see [testing.md](testing.md).

## Configuration Files

- **provider_settings.json:** Provider configuration (addresses, ports, init settings)
- **rule-example.yaml:** Rule definitions
- **ruleset.yaml:** Ruleset metadata (name, description, labels)

## Extension Points

To extend analyzer-lsp:

1. **Add a new provider:** Create a new external-provider directory, use the shared provider server, and implement the Provider interface
2. **Add a new capability:** Define input/output schemas and implement evaluation logic

For detailed provider development information, see [provider_development.md](provider_development.md).
