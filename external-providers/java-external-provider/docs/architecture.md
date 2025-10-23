# Java External Provider Architecture Documentation

## Overview

The Java External Provider is responsible for analyzing Java applications and their dependencies. It consists of two main modules:

1. **bldtool** - Build tool abstraction for extracting dependency information
2. **dependency** - Dependency resolution and source code management

This document describes the architecture, usage, and relationships between these modules and the broader provider system.

## Table of Contents

- [High-Level Architecture](#high-level-architecture)
- [Bldtool Module](#bldtool-module)
- [Dependency Module](#dependency-module)
- [Integration with Provider and Service Client](#integration-with-provider-and-service-client)
- [Usage Guide](#usage-guide)
- [Flow Diagrams](#flow-diagrams)

---

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                      Java Provider                               │
│  ┌────────────────┐                                             │
│  │ Provider Init  │                                             │
│  └───────┬────────┘                                             │
│          │                                                       │
│          v                                                       │
│  ┌──────────────────────────────────────────────────────┐      │
│  │             Service Client                            │      │
│  │  ┌──────────────┐        ┌──────────────────┐       │      │
│  │  │ LSP Client   │        │  BuildTool       │       │      │
│  │  │ (JDTLS)      │        │  Interface       │       │      │
│  │  └──────────────┘        └────────┬─────────┘       │      │
│  └────────────────────────────────────┼──────────────────┘      │
│                                       │                         │
└───────────────────────────────────────┼─────────────────────────┘
                                        │
                ┌───────────────────────┼───────────────────────┐
                │                       │                       │
                v                       v                       v
        ┌───────────────┐     ┌─────────────────┐    ┌──────────────────┐
        │ Maven Build   │     │ Gradle Build    │    │ Binary Build     │
        │ Tool          │     │ Tool            │    │ Tool (Maven)     │
        └───────┬───────┘     └────────┬────────┘    └────────┬─────────┘
                │                      │                      │
                └──────────────────────┼──────────────────────┘
                                       │
                                       v
                            ┌──────────────────────┐
                            │  Dependency Module   │
                            │  ┌────────────────┐  │
                            │  │   Resolvers    │  │
                            │  │  - Maven       │  │
                            │  │  - Gradle      │  │
                            │  │  - Binary      │  │
                            │  └────────┬───────┘  │
                            │           │          │
                            │  ┌────────v───────┐  │
                            │  │  Decompiler    │  │
                            │  └────────────────┘  │
                            └──────────────────────┘
```

---

## Bldtool Module

The `bldtool` module provides an abstraction layer over different Java build systems (Maven, Gradle, and binary artifacts).

### Location
`external-providers/java-external-provider/pkg/java_external_provider/bldtool/`

### Core Interface

The main interface is `BuildTool` defined in `bldtool/tool.go:44-52`:

```go
type BuildTool interface {
    GetDependencies(context.Context) (map[uri.URI][]provider.DepDAGItem, error)
    UseCache() (bool, error)
    GetCachedDepError(errorCached map[string]error) (error, bool)
    GetLocalRepoPath() string
    GetSourceFileLocation(string, string, string) (string, error)
    GetResolver(string) (dependency.Resolver, error)
    ShouldResolve() bool
}
```

### Key Components

#### 1. BuildTool Factory (`bldtool/tool.go:65-83`)

The factory function `GetBuildTool()` determines which build tool to use based on:

```
┌─────────────────────────────────────┐
│   GetBuildTool()                    │
│                                     │
│   1. Check for Gradle build.gradle │
│      ├─ Found? → GradleBuildTool   │
│      └─ Not found? → Continue      │
│                                     │
│   2. Check if binary (.jar/.war)   │
│      ├─ Yes? → MavenBinaryTool     │
│      └─ No? → Continue             │
│                                     │
│   3. Check for Maven pom.xml       │
│      ├─ Found? → MavenBuildTool    │
│      └─ Not found? → nil           │
└─────────────────────────────────────┘
```

#### 2. Maven Build Tool (`bldtool/maven.go`)

**Responsibility**: Handle Maven-based projects

**Key Methods**:
- `GetDependencies()` - Runs `mvn dependency:tree` to extract dependency graph
- `getDependenciesForMaven()` - Parses Maven output to build dependency DAG
- `parseMavenDepLines()` - Recursively parses Maven dependency tree output
- `GetDependenciesFallback()` - Directly parses pom.xml when Maven command fails

**Dependency Resolution Flow**:
```
┌──────────────────────────────────────────────┐
│  1. Run mvn dependency:tree                  │
└──────────────┬───────────────────────────────┘
               │
               v
┌──────────────────────────────────────────────┐
│  2. Parse output using regex patterns        │
│     - Extract submodule trees                │
│     - Parse dependency strings               │
└──────────────┬───────────────────────────────┘
               │
               v
┌──────────────────────────────────────────────┐
│  3. Build DepDAGItem hierarchy               │
│     - Direct dependencies                    │
│     - Transitive dependencies (indirect)     │
└──────────────┬───────────────────────────────┘
               │
               v
┌──────────────────────────────────────────────┐
│  4. On failure → Use fallback pom.xml parser │
│     (gopom library)                          │
└──────────────────────────────────────────────┘
```

**Maven Shared Base** (`bldtool/maven_shared.go`):
- `mavenBaseTool` struct provides common functionality
- `GetDependenciesFallback()` - Parses pom.xml directly using gopom library
- `getMavenLocalRepoPath()` - Determines local Maven repository location

#### 3. Gradle Build Tool (`bldtool/gradle.go`)

**Responsibility**: Handle Gradle-based projects

**Key Methods**:
- `GetDependencies()` - Runs `gradlew dependencies` to extract dependency tree
- `getGradleSubprojects()` - Identifies subprojects in multi-module builds
- `parseGradleDependencyOutput()` - Parses Gradle dependency tree
- `GetGradleVersion()` - Determines Gradle version for Java compatibility
- `GetJavaHomeForGradle()` - Selects appropriate Java version (Java 8 for Gradle ≤8.14, Java 17+ otherwise)

**Gradle Dependency Parsing**:
```
Input Line Examples:
  +--- org.codehaus.groovy:groovy:3.0.21 (c)
  |    \--- com.example:lib:1.0.0
  \--- io.konveyor:analyzer:2.0.0 -> 2.0.1

Parsing Strategy:
  1. Use regex to match tree structure: `^([| ]+)?[+\\]--- (.*)`
  2. Extract dependency info from matched string
  3. Calculate nesting level from prefix length
  4. Build parent-child relationships using depth tracking
  5. Mark transitive dependencies as indirect
```

#### 4. Maven Binary Tool (`bldtool/maven_binary.go`)

**Responsibility**: Handle binary artifacts (JAR/WAR/EAR files) without build files

**Key Methods**:
- Extends `mavenBaseTool`
- `ShouldResolve()` returns `true` - binary artifacts always need resolution
- `GetResolver()` - Returns a binary resolver for decompiling

---

## Dependency Module

The `dependency` module handles dependency source resolution and decompilation.

### Location
`external-providers/java-external-provider/pkg/java_external_provider/dependency/`

### Core Interface

```go
type Resolver interface {
    ResolveSources(ctx context.Context) (string, string, error)
}
```

Returns:
1. Source location (project location)
2. Dependency location (local repository path)
3. Error (if any)

### Key Components

#### 1. Maven Resolver (`dependency/maven_resolver.go`)

**Responsibility**: Download and resolve sources for Maven dependencies

**Resolution Process**:

```
┌────────────────────────────────────────────────────┐
│ 1. Run Maven plugin to download sources            │
│    mvn de.qaware.maven:go-offline-maven-plugin:    │
│        resolve-dependencies -DdownloadSources       │
└─────────────────┬──────────────────────────────────┘
                  │
                  v
┌────────────────────────────────────────────────────┐
│ 2. Parse output for unresolved sources             │
│    - Look for WARNING messages                     │
│    - Extract GAV coordinates using regex           │
└─────────────────┬──────────────────────────────────┘
                  │
                  v
┌────────────────────────────────────────────────────┐
│ 3. For each unresolved dependency:                 │
│    - Create decompilation job                      │
│    - Locate JAR in local Maven repository          │
│    - Decompile using FernFlower                    │
└────────────────────────────────────────────────────┘
```

#### 2. Gradle Resolver (`dependency/gradle_resolver.go`)

**Responsibility**: Download and resolve sources for Gradle dependencies

**Resolution Process**:

```
┌────────────────────────────────────────────────────┐
│ 1. Create temporary build.gradle with task         │
│    - Copy original build.gradle                    │
│    - Append task.gradle (konveyorDownloadSources)  │
│    - Swap files temporarily                        │
└─────────────────┬──────────────────────────────────┘
                  │
                  v
┌────────────────────────────────────────────────────┐
│ 2. Run Gradle task: gradlew konveyorDownloadSources│
└─────────────────┬──────────────────────────────────┘
                  │
                  v
┌────────────────────────────────────────────────────┐
│ 3. Parse output for unresolved sources             │
│    - Pattern: "Found 0 sources for <coords>"       │
│    - Extract GAV coordinates                       │
└─────────────────┬──────────────────────────────────┘
                  │
                  v
┌────────────────────────────────────────────────────┐
│ 4. Locate Gradle cache directory                   │
│    - Search ~/.gradle/caches                       │
│    - Find artifacts by group ID                    │
└─────────────────┬──────────────────────────────────┘
                  │
                  v
┌────────────────────────────────────────────────────┐
│ 5. Decompile unresolved dependencies               │
└────────────────────────────────────────────────────┘
```

#### 3. Binary Resolver (`dependency/binary_resolver.go`)

**Responsibility**: Handle JAR/WAR/EAR files without source projects

Not explicitly shown in files read, but referenced in the architecture.

#### 4. Decompiler (`dependency/decompile.go`)

**Responsibility**: Decompile Java bytecode to source using FernFlower

**Architecture**:

```
┌─────────────────────────────────────────────────┐
│            Decompiler                           │
│                                                 │
│  ┌──────────────────────────────────┐          │
│  │  Worker Pool (10 workers)         │          │
│  │  - Concurrent decompilation       │          │
│  │  - Job queue (channel-based)      │          │
│  └──────────────┬───────────────────┘          │
│                 │                               │
│  ┌──────────────v───────────────────┐          │
│  │  Decompile Jobs:                 │          │
│  │  - jarArtifact                   │          │
│  │  - jarExloadArtifact             │          │
│  │  - warArtifact                   │          │
│  │  - earArtifact                   │          │
│  └──────────────────────────────────┘          │
└─────────────────────────────────────────────────┘
```

**Key Methods**:
- `Decompile()` - Decompile JAR as dependency (creates Maven artifact structure)
- `DecompileIntoProject()` - Decompile into project structure (for source analysis)
- `decompileWorker()` - Background worker processing decompilation jobs

**Decompilation Command**:
```bash
java -jar /path/to/fernflower.jar -mpm=30 <input.jar> <output-dir>
```

#### 5. Artifact Identification (`dependency/artifact.go`)

**Responsibility**: Identify Maven coordinates for JAR files

**Identification Strategies** (in order of preference):

```
┌─────────────────────────────────────────────────────┐
│ Strategy 1: SHA1 Lookup via Maven Central          │
│ ├─ Calculate SHA1 hash of JAR                      │
│ ├─ Query search.maven.org API                      │
│ ├─ Parse JSON response for GAV coordinates         │
│ └─ Cache errors to avoid repeated failures         │
└──────────────────┬──────────────────────────────────┘
                   │ (on failure)
                   v
┌─────────────────────────────────────────────────────┐
│ Strategy 2: Extract from embedded POM               │
│ ├─ Open JAR as ZIP archive                         │
│ ├─ Look for META-INF/maven/*/*/pom.properties      │
│ ├─ Parse properties file for groupId, artifactId   │
│ └─ Cross-reference with open source labeler        │
└──────────────────┬──────────────────────────────────┘
                   │ (on failure)
                   v
┌─────────────────────────────────────────────────────┐
│ Strategy 3: Infer from JAR structure                │
│ ├─ Extract package structure from .class files     │
│ ├─ Find longest common package prefix              │
│ ├─ Match against known open source group IDs       │
│ └─ Use JAR filename as artifact ID                 │
└─────────────────────────────────────────────────────┘
```

**JavaArtifact Structure**:
```go
type JavaArtifact struct {
    FoundOnline bool    // Whether found in Maven Central or known OSS
    Packaging   string  // .jar, .war, .ear
    GroupId     string  // e.g., "org.springframework"
    ArtifactId  string  // e.g., "spring-core"
    Version     string  // e.g., "5.3.21"
    Sha1        string  // SHA1 hash for verification
}
```

---

## Integration with Provider and Service Client

### Provider Initialization Flow

```
┌───────────────────────────────────────────────────────────────┐
│ javaProvider.Init()                                           │
│  (provider.go:214)                                            │
└────────────────┬──────────────────────────────────────────────┘
                 │
                 v
┌───────────────────────────────────────────────────────────────┐
│ 1. Determine Analysis Mode                                    │
│    - FullAnalysisMode: Download deps + sources                │
│    - SourceOnlyAnalysisMode: Only use existing sources        │
└────────────────┬──────────────────────────────────────────────┘
                 │
                 v
┌───────────────────────────────────────────────────────────────┐
│ 2. Initialize Open Source Labeler                             │
│    - Used to identify open source vs internal dependencies    │
└────────────────┬──────────────────────────────────────────────┘
                 │
                 v
┌───────────────────────────────────────────────────────────────┐
│ 3. Get Build Tool (bldtool.GetBuildTool)                     │
│    - Detects: Maven, Gradle, or Binary                        │
└────────────────┬──────────────────────────────────────────────┘
                 │
                 v
┌───────────────────────────────────────────────────────────────┐
│ 4. Resolve Sources (if needed)                                │
│    - buildTool.ShouldResolve() → true for binaries            │
│    - FullAnalysisMode → always resolve                        │
│    - buildTool.GetResolver() → dependency.Resolver            │
│    - resolver.ResolveSources() → download/decompile           │
└────────────────┬──────────────────────────────────────────────┘
                 │
                 v
┌───────────────────────────────────────────────────────────────┐
│ 5. Start JDTLS (Eclipse JDT Language Server)                 │
│    - Create JVM process with appropriate settings             │
│    - Initialize JSON-RPC connection                           │
└────────────────┬──────────────────────────────────────────────┘
                 │
                 v
┌───────────────────────────────────────────────────────────────┐
│ 6. Create and Return Service Client                           │
│    - Contains: RPC client, BuildTool reference                │
└───────────────────────────────────────────────────────────────┘
```

### Service Client Responsibilities

The `javaServiceClient` (`service_client.go:27-48`) is the main interface for analyzing Java code:

**Key Components**:
```go
type javaServiceClient struct {
    rpc                provider.RPCClient      // JSON-RPC to JDTLS
    buildTool          bldtool.BuildTool       // Reference to build tool
    config             provider.InitConfig      // Configuration
    depsCache          map[uri.URI][]provider.DepDAGItem  // Cached deps
    mvnSettingsFile    string                  // Maven settings
    mvnIndexPath       string                  // Maven index for labeling
}
```

**Key Methods**:

1. **Evaluate()** (`service_client.go:52-112`)
   - Evaluates rule conditions using JDTLS
   - Calls `GetAllSymbols()` to query code
   - Filters results based on location type (inheritance, method calls, etc.)

2. **GetDependencies()** (via BuildTool)
   - Returns dependency DAG for the project
   - Uses caching to avoid repeated Maven/Gradle executions

3. **GetAllSymbols()** (`service_client.go:114-198`)
   - Sends workspace/executeCommand to JDTLS
   - Command: `io.konveyor.tackle.ruleEntry`
   - Returns matching symbols from codebase

### Dependency Flow in Service Client

```
┌──────────────────────────────────────────────┐
│ User requests dependency analysis            │
└──────────────┬───────────────────────────────┘
               │
               v
┌──────────────────────────────────────────────┐
│ Service Client: GetDependencies()            │
└──────────────┬───────────────────────────────┘
               │
               v
┌──────────────────────────────────────────────┐
│ BuildTool: UseCache()                        │
│  - Check if pom.xml/build.gradle changed     │
│  - Compare SHA256 hash                       │
└──────────────┬───────────────────────────────┘
               │
         ┌─────┴─────┐
         │           │
    Cache Hit    Cache Miss
         │           │
         │           v
         │     ┌──────────────────────────────┐
         │     │ BuildTool: GetDependencies() │
         │     │  - Run Maven/Gradle          │
         │     │  - Parse dependency tree     │
         │     │  - Store in cache            │
         │     └──────────────────────────────┘
         │           │
         └─────┬─────┘
               │
               v
┌──────────────────────────────────────────────┐
│ Return map[uri.URI][]provider.DepDAGItem    │
│  - Key: Build file URI                       │
│  - Value: List of dependencies with DAG      │
└──────────────────────────────────────────────┘
```

### Relationship Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                     javaProvider                            │
│  ┌───────────────────────────────────────────────────┐     │
│  │  Init()                                            │     │
│  │    ├─ Creates BuildTool via bldtool.GetBuildTool()│     │
│  │    ├─ Gets Resolver via buildTool.GetResolver()   │     │
│  │    ├─ Calls resolver.ResolveSources()             │     │
│  │    └─ Creates javaServiceClient                   │     │
│  └───────────────────────────────────────────────────┘     │
└─────────────────────────────────────────────────────────────┘
                            │
                            │ creates
                            v
┌─────────────────────────────────────────────────────────────┐
│                  javaServiceClient                          │
│  ┌───────────────────────────────────────────────────┐     │
│  │  - Holds reference to BuildTool                    │     │
│  │  - Communicates with JDTLS via JSON-RPC           │     │
│  │  - Uses BuildTool for dependency info             │     │
│  └───────────────────────────────────────────────────┘     │
└─────────────────────────────────────────────────────────────┘
         │                                │
         │ uses                           │ calls
         v                                v
┌────────────────────┐         ┌─────────────────────┐
│   BuildTool        │         │  JDTLS (External)   │
│   - Maven          │         │  - Code analysis    │
│   - Gradle         │         │  - Symbol search    │
│   - Binary         │         │  - References       │
└────────┬───────────┘         └─────────────────────┘
         │
         │ uses
         v
┌────────────────────┐
│  Resolver          │
│  - Maven           │
│  - Gradle          │
│  - Binary          │
└────────┬───────────┘
         │
         │ uses
         v
┌────────────────────┐
│  Decompiler        │
│  - FernFlower      │
│  - Worker pool     │
└────────────────────┘
```

---

## Usage Guide

### How to Use BuildTool

#### Example: Getting Dependencies

```go
import (
    "context"
    "github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/bldtool"
    "github.com/konveyor/analyzer-lsp/provider"
)

// Create BuildTool options
opts := bldtool.BuildToolOptions{
    Config: provider.InitConfig{
        Location: "/path/to/project",  // Project directory
    },
    MvnSettingsFile:    "/path/to/settings.xml",  // Optional
    MvnInsecure:        false,
    DisableMavenSearch: false,
    Labeler:            myLabeler,  // For identifying open source deps
}

// Get appropriate build tool
buildTool := bldtool.GetBuildTool(opts, logger)
if buildTool == nil {
    // No build file found (no pom.xml or build.gradle)
    return
}

// Get dependencies
ctx := context.Background()
deps, err := buildTool.GetDependencies(ctx)
if err != nil {
    // Handle error
}

// deps is a map[uri.URI][]provider.DepDAGItem
// Key: URI of build file (pom.xml or build.gradle)
// Value: Dependency DAG with direct and transitive dependencies
```

#### Example: Resolving Sources

```go
// Check if we need to resolve sources
if buildTool.ShouldResolve() {
    // Get resolver
    resolver, err := buildTool.GetResolver("/path/to/fernflower.jar")
    if err != nil {
        // Handle error
    }

    // Resolve sources (download + decompile as needed)
    srcLocation, depLocation, err := resolver.ResolveSources(ctx)
    if err != nil {
        // Handle error
    }

    // srcLocation: Where source code is located
    // depLocation: Where dependency JARs are located (e.g., ~/.m2/repository)
}
```

### How to Use Dependency Module

#### Example: Decompiling a JAR

```go
import (
    "context"
    "github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency"
)

opts := dependency.DecompilerOpts{
    DecompileTool:      "/path/to/fernflower.jar",
    log:                logger,
    workers:            10,  // Number of concurrent workers
    labler:             myLabeler,
    disableMavenSearch: false,
    m2Repo:             "/home/user/.m2/repository",
}

decompiler, err := dependency.getDecompiler(opts)
if err != nil {
    // Handle error
}

// Decompile a JAR as a dependency (creates Maven structure)
artifacts, err := decompiler.Decompile(ctx, "/path/to/library.jar")
if err != nil {
    // Handle error
}

// artifacts: []JavaArtifact with Maven coordinates
```

#### Example: Identifying JAR Coordinates

```go
import (
    "context"
    "github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency"
)

// Try to identify Maven coordinates for a JAR
artifact, err := dependency.ToDependency(ctx, logger, labeler, "/path/to/unknown.jar", false)
if err != nil {
    // Could not identify
}

// artifact.GroupId: "org.example"
// artifact.ArtifactId: "my-library"
// artifact.Version: "1.2.3"
// artifact.FoundOnline: true/false
```

### Configuration Options

#### BuildToolOptions

| Field | Type | Description |
|-------|------|-------------|
| `Config` | `provider.InitConfig` | Base configuration with Location |
| `MvnSettingsFile` | `string` | Path to Maven settings.xml |
| `MvnInsecure` | `bool` | Allow insecure HTTPS for Maven |
| `MvnIndexPath` | `string` | Path to Maven index for labeling |
| `DisableMavenSearch` | `bool` | Disable Maven Central lookups |
| `Labeler` | `labels.Labeler` | For identifying OSS dependencies |
| `CleanBin` | `bool` | Clean exploded binaries after analysis |
| `GradleTaskFile` | `string` | Custom Gradle task file |

#### ResolverOptions

| Field | Type | Description |
|-------|------|-------------|
| `Log` | `logr.Logger` | Logger instance |
| `Location` | `string` | Project root directory |
| `DecompileTool` | `string` | Path to FernFlower JAR |
| `Labeler` | `labels.Labeler` | Dependency labeler |
| `LocalRepo` | `string` | Local Maven repository path |
| `DisableMavenSearch` | `bool` | Disable Maven Central API |
| `BuildFile` | `string` | Maven settings or Gradle build file |
| `Insecure` | `bool` | Allow insecure HTTPS (Maven only) |
| `Version` | `version.Version` | Gradle version (Gradle only) |
| `Wrapper` | `string` | Gradle wrapper path (Gradle only) |
| `JavaHome` | `string` | Java home directory (Gradle only) |
| `GradleTaskFile` | `string` | Custom task file (Gradle only) |

---

## Flow Diagrams

### Complete Analysis Flow

```
┌─────────────────────────────────────────────────────────────────┐
│ 1. User initiates Java project analysis                        │
└──────────────────────┬──────────────────────────────────────────┘
                       │
                       v
┌─────────────────────────────────────────────────────────────────┐
│ 2. javaProvider.Init()                                          │
│    - Read configuration                                         │
│    - Initialize labeler for OSS detection                       │
└──────────────────────┬──────────────────────────────────────────┘
                       │
                       v
┌─────────────────────────────────────────────────────────────────┐
│ 3. bldtool.GetBuildTool()                                       │
│    - Scan project directory                                     │
│    - Detect: Gradle → Maven → Binary                           │
└──────────────────────┬──────────────────────────────────────────┘
                       │
                       v
┌─────────────────────────────────────────────────────────────────┐
│ 4. BuildTool.GetDependencies()                                  │
│    Maven:  mvn dependency:tree                                  │
│    Gradle: gradlew dependencies                                 │
│    Binary: Skip (no build file)                                 │
└──────────────────────┬──────────────────────────────────────────┘
                       │
                       v
┌─────────────────────────────────────────────────────────────────┐
│ 5. Check if source resolution needed                            │
│    - buildTool.ShouldResolve() == true?                         │
│    - Analysis mode == FullAnalysisMode?                         │
└──────────────────────┬──────────────────────────────────────────┘
                       │
                 Yes   │   No
              ┌────────┴────────┐
              │                 │
              v                 v
┌──────────────────────┐   ┌──────────────────────┐
│ 6a. Resolve Sources  │   │ 6b. Skip resolution  │
│                      │   │                      │
│ buildTool.GetResolver│   │ Use existing sources │
│ resolver.Resolve     │   │                      │
│  - Download sources  │   └──────────┬───────────┘
│  - Decompile JARs    │              │
└──────────┬───────────┘              │
           │                          │
           └────────────┬─────────────┘
                        │
                        v
┌─────────────────────────────────────────────────────────────────┐
│ 7. Start JDTLS (Eclipse JDT Language Server)                    │
│    - Point to source location                                   │
│    - Point to dependency location                               │
│    - Initialize workspace                                       │
└──────────────────────┬──────────────────────────────────────────┘
                       │
                       v
┌─────────────────────────────────────────────────────────────────┐
│ 8. Create javaServiceClient                                     │
│    - Store buildTool reference                                  │
│    - Store JDTLS RPC connection                                 │
│    - Initialize dependency cache                                │
└──────────────────────┬──────────────────────────────────────────┘
                       │
                       v
┌─────────────────────────────────────────────────────────────────┐
│ 9. Service Client Ready                                         │
│    - Can evaluate rules via JDTLS                               │
│    - Can query dependencies via BuildTool                       │
│    - Can resolve dependency locations                           │
└─────────────────────────────────────────────────────────────────┘
```

### Dependency Resolution Detail

```
┌─────────────────────────────────────────────────────────────────┐
│ resolver.ResolveSources(ctx)                                    │
└──────────────────────┬──────────────────────────────────────────┘
                       │
        ┌──────────────┴──────────────┐
        │                             │
    Maven Resolver              Gradle Resolver
        │                             │
        v                             v
┌──────────────────┐          ┌──────────────────┐
│ mvn plugin       │          │ gradlew task     │
│ downloadSources  │          │ download sources │
└────────┬─────────┘          └────────┬─────────┘
         │                              │
         v                              v
┌──────────────────┐          ┌──────────────────┐
│ Parse output for │          │ Parse output for │
│ missing sources  │          │ missing sources  │
└────────┬─────────┘          └────────┬─────────┘
         │                              │
         │                              v
         │                    ┌──────────────────┐
         │                    │ Find Gradle cache│
         │                    │ directory        │
         │                    └────────┬─────────┘
         │                              │
         └──────────────┬───────────────┘
                        │
                        v
        ┌───────────────────────────────────┐
        │ For each missing source JAR:      │
        │  1. Locate JAR file               │
        │  2. Create decompile job          │
        │  3. Submit to worker pool         │
        └───────────────┬───────────────────┘
                        │
                        v
        ┌───────────────────────────────────┐
        │ Decompiler Workers (10 threads)   │
        │  - Pick up jobs from queue        │
        │  - Run FernFlower                 │
        │  - Extract sources to directory   │
        └───────────────┬───────────────────┘
                        │
                        v
        ┌───────────────────────────────────┐
        │ Wait for all jobs to complete     │
        │ Return: (srcPath, depPath, error) │
        └───────────────────────────────────┘
```

### Artifact Identification Flow

```
┌─────────────────────────────────────────────────────────────────┐
│ dependency.ToDependency(ctx, logger, labeler, jarPath, disable) │
└──────────────────────┬──────────────────────────────────────────┘
                       │
                       v
        ┌──────────────────────────────┐
        │ disableMavenSearch == true?  │
        └──────────┬───────────────────┘
                   │
            No     │     Yes
          ┌────────┴────────┐
          │                 │
          v                 v
┌──────────────────┐   ┌──────────────────┐
│ Try Maven Central│   │ Skip to POM      │
│ SHA1 lookup      │   │ extraction       │
└────────┬─────────┘   └────────┬─────────┘
         │                      │
    Success? No                 │
         │                      │
         v                      │
┌──────────────────┐            │
│ Try extracting   │◄───────────┘
│ from POM props   │
└────────┬─────────┘
         │
    Success? No
         │
         v
┌──────────────────┐
│ Try inferring    │
│ from JAR struct  │
└────────┬─────────┘
         │
    Success? No
         │
         v
┌──────────────────┐
│ Return error     │
└──────────────────┘
```

---

## Key Design Patterns

### 1. Strategy Pattern (Build Tools)

The `BuildTool` interface allows different build systems to be used interchangeably:
- `mavenBuildTool` for Maven projects
- `gradleBuildTool` for Gradle projects
- `mavenBinaryBuildTool` for binary artifacts

### 2. Factory Pattern

`GetBuildTool()` acts as a factory, automatically selecting the correct build tool implementation based on project structure.

### 3. Worker Pool Pattern

The decompiler uses a worker pool (10 workers by default) to parallelize decompilation:
- Jobs submitted to a channel
- Workers pull jobs and process concurrently
- Results collected via response channel

### 4. Fallback Pattern

Multiple fallback mechanisms ensure robustness:
- Maven dependency resolution → fallback to pom.xml parsing
- Maven Central lookup → fallback to embedded POM → fallback to structure inference
- Plugin execution → fallback to direct command parsing

### 5. Caching Pattern

Build tools cache results to avoid expensive re-execution:
- Hash build files (pom.xml, build.gradle)
- Compare hashes on subsequent calls
- Return cached results if unchanged

---

## Common Use Cases

### 1. Analyzing a Maven Project

```
User provides: /path/to/maven-project

Flow:
1. GetBuildTool() finds pom.xml → creates mavenBuildTool
2. GetDependencies() runs mvn dependency:tree
3. Parses Maven output into DepDAGItem hierarchy
4. Returns dependencies to service client
5. JDTLS uses dependencies for code analysis
```

### 2. Analyzing a Gradle Project

```
User provides: /path/to/gradle-project

Flow:
1. GetBuildTool() finds build.gradle → creates gradleBuildTool
2. Determines Gradle version and Java compatibility
3. GetDependencies() runs gradlew dependencies
4. Parses Gradle tree output
5. Returns dependencies to service client
```

### 3. Analyzing a Binary (JAR/WAR/EAR)

```
User provides: /path/to/application.war

Flow:
1. GetBuildTool() detects binary → creates mavenBinaryBuildTool
2. ShouldResolve() returns true
3. GetResolver() creates binary resolver
4. ResolveSources() decompiles WAR:
   - Explodes WAR structure
   - Finds embedded JARs
   - Decompiles each JAR
   - Creates project structure
5. Returns decompiled source location
6. JDTLS analyzes decompiled sources
```

### 4. Full Analysis Mode with Missing Sources

```
User provides: /path/to/maven-project
Mode: FullAnalysisMode

Flow:
1. GetBuildTool() → mavenBuildTool
2. GetDependencies() → finds all dependencies
3. GetResolver() → mavenDependencyResolver
4. ResolveSources():
   - Runs mvn download sources plugin
   - Identifies JARs without sources
   - Decompiles missing JARs in parallel
   - Stores sources in ~/.m2/repository
5. JDTLS has access to all dependency sources
```

---

## Error Handling

### BuildTool Errors

- **No build file found**: Returns `nil` from `GetBuildTool()`
- **Maven command fails**: Falls back to pom.xml parsing with gopom
- **Gradle wrapper missing**: Returns error (Gradle wrapper required)
- **Dependency tree parsing fails**: Returns partial results or error

### Resolver Errors

- **Plugin execution fails**: Returns error (can't proceed without sources)
- **Decompilation fails**: Individual failures logged, continues with others
- **Maven Central unavailable**: Cached error prevents repeated attempts

### Artifact Identification Errors

- **All strategies fail**: Returns artifact with partial info (e.g., just artifactId)
- **Maven Central rate limit**: Falls back to local strategies
- **Malformed JAR**: Returns error

---

## Performance Considerations

### 1. Caching

- BuildTool caches dependency results using SHA256 hash of build files
- Maven Central errors cached to prevent repeated API calls
- Dependency location cache prevents repeated grep operations

### 2. Parallelization

- Decompiler uses 10 workers by default (configurable)
- Maven/Gradle execution timeouts (5 minutes default)
- Concurrent decompilation of multiple JARs

### 3. Lazy Resolution

- Sources only resolved when `ShouldResolve()` is true or FullAnalysisMode
- Binary projects skip dependency tree execution
- Gradle subproject analysis only when subprojects exist

---

## Extension Points

To add support for a new build system:

1. Implement the `BuildTool` interface
2. Add detection logic to `GetBuildTool()`
3. Implement a corresponding `Resolver` if sources need resolution
4. Update this documentation

Example skeleton:

```go
type antBuildTool struct {
    buildFile string
    // ... other fields
}

func (a *antBuildTool) GetDependencies(ctx context.Context) (map[uri.URI][]provider.DepDAGItem, error) {
    // Parse build.xml
    // Extract dependencies
    // Return DAG structure
}

func (a *antBuildTool) GetResolver(decompileTool string) (dependency.Resolver, error) {
    // Return Ant-specific resolver
}

// Implement other interface methods...
```

---

## Troubleshooting

### "No build tool found"

**Cause**: No pom.xml, build.gradle, or binary file detected

**Solution**: Ensure project has proper build file or provide binary artifact

### "Maven dependency resolution failed"

**Cause**: Maven command failed (network issues, invalid pom.xml, etc.)

**Solution**: Check Maven installation, network connectivity, pom.xml validity. Fallback parser may still work.

### "Gradle wrapper not found"

**Cause**: Gradle project without wrapper

**Solution**: Generate Gradle wrapper: `gradle wrapper`

### "Decompilation failed"

**Cause**: FernFlower error, invalid JAR, or missing JAVA_HOME

**Solution**: Verify FernFlower path, JAR validity, and JAVA_HOME environment variable

### "Java version incompatible with Gradle"

**Cause**: Gradle ≤8.14 requires Java 8, but only Java 17+ available

**Solution**: Set JAVA8_HOME environment variable to Java 8 installation

---

## References

### Key Files

- **bldtool/tool.go**: Main BuildTool interface and factory
- **bldtool/maven.go**: Maven build tool implementation
- **bldtool/gradle.go**: Gradle build tool implementation
- **bldtool/maven_binary.go**: Binary artifact handling
- **bldtool/maven_shared.go**: Shared Maven functionality
- **dependency/resolver.go**: Resolver interface
- **dependency/maven_resolver.go**: Maven source resolution
- **dependency/gradle_resolver.go**: Gradle source resolution
- **dependency/decompile.go**: Decompilation engine
- **dependency/artifact.go**: JAR artifact identification
- **provider.go**: Java provider initialization
- **service_client.go**: Service client for analysis operations

### External Tools

- **JDTLS**: Eclipse JDT Language Server for Java code analysis
- **FernFlower**: Java decompiler
- **Maven**: Build and dependency management
- **Gradle**: Build and dependency management

---

## Conclusion

The bldtool and dependency modules work together to provide a comprehensive solution for analyzing Java projects:

1. **bldtool** detects the build system and extracts dependency information
2. **dependency** resolves missing sources through download or decompilation
3. **provider/service client** coordinates these modules with the language server

This architecture enables the Java External Provider to analyze:
- Source projects (Maven/Gradle)
- Binary artifacts (JAR/WAR/EAR)
- Projects with or without dependency sources
- Multi-module projects

The modular design allows easy extension for new build systems while maintaining a consistent interface for the provider layer.
