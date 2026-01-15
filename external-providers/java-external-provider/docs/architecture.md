# Java External Provider Architecture Documentation

## Overview

The Java External Provider is responsible for analyzing Java applications and their dependencies. It consists of several key modules:

1. **bldtool** - Build tool abstraction for extracting dependency information
2. **dependency** - Dependency resolution, source code management, and binary artifact handling
3. **Symbol filtering** - LSP symbol to incident context conversion
4. **Code snippet extraction** - Contextual code extraction for incidents

This document describes the architecture, usage, and relationships between these modules and the broader provider system. It covers dependency analysis, source resolution, decompilation, binary explosion (JAR/WAR/EAR), Maven artifact downloading, incident reporting, and integration with the Eclipse JDT Language Server (JDTLS).

## Table of Contents

- [High-Level Architecture](#high-level-architecture)
- [Bldtool Module](#bldtool-module)
- [Dependency Module](#dependency-module)
- [Symbol Filtering and Incident Conversion](#symbol-filtering-and-incident-conversion)
- [Code Snippet Extraction](#code-snippet-extraction)
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

The main interface is `BuildTool` defined in `bldtool/tool.go`:

```go
type BuildTool interface {
    GetDependencies(context.Context) (map[uri.URI][]provider.DepDAGItem, error)
    GetLocalRepoPath() string
    GetSourceFileLocation(string, string, string) (string, error)
    GetResolver(string) (dependency.Resolver, error)
    ShouldResolve() bool
}
```

**Note**: Caching logic has been refactored and is now handled internally by each BuildTool implementation using the shared `depCache` struct.

### Key Components

#### 0. Shared Dependency Cache (`bldtool/dep_cache.go`)

**Responsibility**: Provide thread-safe dependency caching for all build tool implementations

The `depCache` struct is embedded in each BuildTool implementation to provide consistent, thread-safe caching behavior.

**Structure**:
```go
type depCache struct {
    hashFile string                         // Path to build file (pom.xml, build.gradle)
    hash     *string                        // SHA256 hash of build file for cache validation
    hashSync sync.Mutex                     // Mutex for thread-safe cache access
    deps     map[uri.URI][]provider.DepDAGItem // Cached dependency DAG
    depLog   logr.Logger                    // Logger for cache operations
}
```

**Key Methods**:

- `useCache() (bool, error)` - Check if cached dependencies are valid
  - Computes SHA256 hash of build file
  - **Acquires lock immediately** to ensure thread-safe cache access
  - Compares with cached hash
  - **Releases lock on cache hit** and returns true
  - **Keeps lock on cache miss** to prevent concurrent dependency resolution
  - Returns false if dependencies need to be re-fetched (lock remains held)

- `getCachedDeps() map[uri.URI][]provider.DepDAGItem` - Retrieve cached dependencies
  - Returns the cached dependency DAG
  - Called when `useCache()` returns true

- `setCachedDeps(deps, err) error` - Update cache with new dependencies
  - Stores new dependency results
  - Updates cached hash
  - **Releases lock** acquired by `useCache()`
  - Ensures thread-safe cache updates

**Cache Invalidation Strategy**:
```
┌──────────────────────────────────────────────┐
│ GetDependencies() called                     │
└──────────────┬───────────────────────────────┘
               │
               v
┌──────────────────────────────────────────────┐
│ useCache() - acquire lock immediately        │
│  - Lock acquired (hashSync)                  │
│  - Compute SHA256 of build file              │
│  - Compare with cached hash                  │
└──────────────┬───────────────────────────────┘
               │
         ┌─────┴─────┐
         │           │
    Cache Hit    Cache Miss
         │           │
         v           │
┌─────────────┐      │
│Release lock │      │
│Return cached│      │
│dependencies │      │
│             │      │
└─────────────┘      │
                     v
                ┌──────────────────────────────┐
                │ Lock remains held            │
                │ Run build tool command       │
                │  - mvn dependency:tree       │
                │  - gradlew dependencies      │
                └──────────────────────────────┘
                         │
                         v
                ┌──────────────────────────────┐
                │ setCachedDeps()              │
                │  - Update cache              │
                │  - Update hash               │
                │  - Release lock              │
                └──────────────────────────────┘
```

**Thread Safety Features**:
- **Early lock acquisition**: Lock is acquired at the start of `useCache()` before hash computation
- **Lock release on cache hit**: Lock is immediately released when cached data is valid
- **Lock hold on cache miss**: Lock remains held through build execution to prevent concurrent builds
- **Single build execution**: Multiple concurrent requests will wait for the first to complete
- **No deadlocks**: Lock is always released via either `useCache()` on cache hit or `setCachedDeps()` on cache miss

**Benefits**:
- Eliminates duplicate Maven/Gradle command execution
- Reduces service client complexity (caching moved from client to build tool)
- Thread-safe without requiring external synchronization
- Automatic invalidation on build file changes
- Consistent behavior across all build tool types

#### 1. BuildTool Factory (`bldtool/tool.go`)

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

**Structure**:
```go
type mavenBuildTool struct {
    mavenBaseTool
    *depCache  // Embedded dependency cache (pointer)
}
```

**Key Methods**:
- `GetDependencies(ctx)` - Retrieves Maven dependencies with caching
  - Calls `depCache.useCache()` to check cache validity
  - Returns cached results on cache hit
  - Calls `getDependenciesForMaven()` on cache miss
  - Updates cache via `depCache.setCachedDeps()`
  - **Thread-safe**: Lock managed by depCache
- `getDependenciesForMaven()` - Runs `mvn dependency:tree` and parses output
- `parseMavenDepLines()` - Recursively parses Maven dependency tree output
- `GetDependenciesFallback()` - Directly parses pom.xml when Maven command fails (inherited from mavenBaseTool)

**Dependency Resolution Flow**:
```
┌──────────────────────────────────────────────┐
│  1. GetDependencies() called                 │
└──────────────┬───────────────────────────────┘
               │
               v
┌──────────────────────────────────────────────┐
│  2. depCache.useCache() checks pom.xml hash  │
│     - Cache hit? Return cached dependencies  │
│     - Cache miss? Acquire lock & continue    │
└──────────────┬───────────────────────────────┘
               │
               v
┌──────────────────────────────────────────────┐
│  3. getDependenciesForMaven()                │
│     - Run: mvn dependency:tree               │
└──────────────┬───────────────────────────────┘
               │
               v
┌──────────────────────────────────────────────┐
│  4. Parse output using regex patterns        │
│     - Extract submodule trees                │
│     - Parse dependency strings               │
└──────────────┬───────────────────────────────┘
               │
               v
┌──────────────────────────────────────────────┐
│  5. Build DepDAGItem hierarchy               │
│     - Direct dependencies                    │
│     - Transitive dependencies (indirect)     │
└──────────────┬───────────────────────────────┘
               │
               v
┌──────────────────────────────────────────────┐
│  6. depCache.setCachedDeps()                 │
│     - Update cache with results              │
│     - Update hash                            │
│     - Release lock                           │
└──────────────────────────────────────────────┘
```

**Note**: The fallback pom.xml parser is available through `GetDependenciesFallback()` but is now called separately, not automatically on failure.

**Maven Shared Base** (`bldtool/maven_shared.go`):

The `mavenBaseTool` struct provides common functionality shared between Maven and Maven Binary build tools:

```go
type mavenBaseTool struct {
    mvnInsecure     bool           // Whether to allow insecure HTTPS connections
    mvnSettingsFile string         // Path to Maven settings.xml file
    mvnLocalRepo    string         // Path to local Maven repository (.m2/repository)
    mavenIndexPath  string         // Path to Maven index for artifact searches
    dependencyPath  string         // Path to dependency configuration file
    log             logr.Logger    // Logger instance for this build tool
    labeler         labels.Labeler // Labeler for identifying dependency types
}
```

**Key Methods**:
- `GetDependenciesFallback()` - Parses pom.xml directly using gopom library
- `getMavenLocalRepoPath()` - Determines local Maven repository location
- `GetLocalRepoPath()` - Returns the local Maven repository path

#### 3. Gradle Build Tool (`bldtool/gradle.go`)

**Responsibility**: Handle Gradle-based projects

**Structure**:
```go
type gradleBuildTool struct {
    *depCache                      // Embedded dependency cache
    taskFile       string          // Path to custom Gradle task file for dependency resolution
    mavenIndexPath string          // Path to Maven index for artifact searches
    log            logr.Logger     // Logger instance for this build tool
    labeler        labels.Labeler  // Labeler for identifying open source vs internal dependencies
}
```

**Key Methods**:
- `GetDependencies(ctx)` - Retrieves Gradle dependencies with caching
  - Calls `depCache.useCache()` to check build.gradle hash
  - Returns cached results on cache hit
  - Calls internal dependency resolution on cache miss
  - Updates cache via `depCache.setCachedDeps()`
  - **Thread-safe**: Lock managed by depCache
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

**Structure**:
```go
type mavenBinaryBuildTool struct {
    mavenBaseTool
    resolveSync        *sync.Mutex          // Protects resolution and mavenBldTool access
    binaryLocation     string               // Absolute path to the binary artifact (JAR/WAR/EAR)
    disableMavenSearch bool                 // Whether to disable Maven repository lookups
    dependencyPath     string               // Path to dependency configuration
    resolver           dependency.Resolver  // Resolver for source resolution and decompilation
    mavenBldTool       *mavenBuildTool      // Created after resolution completes
}
```

**Key Methods**:
- Extends `mavenBaseTool`
- `ShouldResolve()` returns `true` - binary artifacts always need resolution
- `GetResolver()` - Returns a binary resolver for decompiling
- `ResolveSources(ctx)` - Decompiles binary and creates Maven project structure
  - **Acquires resolveSync lock** to prevent concurrent resolution
  - Calls resolver to decompile binary
  - Creates `mavenBuildTool` instance with generated pom.xml
  - Calls `mavenBuildTool.GetDependencies()` to analyze decompiled project
  - **Releases lock** after resolution completes
- `GetDependencies(ctx)` - Returns dependencies from decompiled project
  - **Acquires resolveSync lock** for thread-safe access
  - Returns error if resolution hasn't completed yet
  - Delegates to `mavenBldTool.GetDependencies()` after resolution
- `discoverDepsFromJars()` - Walks decompiled path to discover embedded JARs
- `discoverPoms()` - Finds pom.xml files in decompiled structure

**Internal Helper: walker struct**

The `walker` type is an internal helper for traversing decompiled binary artifacts to discover dependencies:

```go
type walker struct {
    deps           map[uri.URI][]provider.DepDAGItem // Accumulated dependency graph
    labeler        labels.Labeler                    // Labeler for dependency classification
    m2RepoPath     string                            // Maven local repository path
    initialPath    string                            // Starting path for traversal
    seen           map[string]bool                   // Tracks processed artifacts to prevent duplicates
    pomPaths       []string                          // Collected paths to found pom.xml files
    log            logr.Logger                       // Logger instance
    mavenIndexPath string                            // Path to Maven index for lookups
}
```

**Key Methods**:
- `walkDirForJar()` - Traverses directories to find JAR files and .class files
  - Identifies Maven coordinates using `dependency.ToDependency()`
  - Adds discovered JARs to dependency graph
  - Deduplicates using `seen` map
  - Handles WEB-INF class files as application code (not dependencies)
- `walkDirForPom()` - Traverses directories to find pom.xml files
  - Collects paths to all discovered POM files

**Binary Resolution and Synchronization Flow**:
```
┌──────────────────────────────────────────────┐
│ Binary artifact analysis starts              │
└──────────────┬───────────────────────────────┘
               │
               v
┌──────────────────────────────────────────────┐
│ ResolveSources() called (from provider init) │
│  - Acquires resolveSync lock                 │
└──────────────┬───────────────────────────────┘
               │
               v
┌──────────────────────────────────────────────┐
│ Binary resolver decompiles artifact          │
│  - Explodes JAR/WAR/EAR structure            │
│  - Decompiles .class files                   │
│  - Generates pom.xml with dependencies       │
└──────────────┬───────────────────────────────┘
               │
               v
┌──────────────────────────────────────────────┐
│ Create mavenBuildTool instance               │
│  - Points to generated pom.xml               │
│  - Includes depCache for dependency caching  │
└──────────────┬───────────────────────────────┘
               │
               v
┌──────────────────────────────────────────────┐
│ Call mavenBuildTool.GetDependencies()        │
│  - Analyzes generated pom.xml                │
│  - Populates depCache                        │
└──────────────┬───────────────────────────────┘
               │
               v
┌──────────────────────────────────────────────┐
│ Release resolveSync lock                     │
│ - Binary is now ready for analysis           │
└──────────────────────────────────────────────┘
               │
               v
┌──────────────────────────────────────────────┐
│ Subsequent GetDependencies() calls           │
│  - Acquire resolveSync lock                  │
│  - Delegate to mavenBldTool (uses depCache)  │
│  - Release lock and return results           │
└──────────────────────────────────────────────┘
```

**Thread Safety Features**:
- **resolveSync mutex**: Ensures only one thread performs binary resolution
- **Wait-for-resolution**: `GetDependencies()` waits if resolution is in progress
- **Lazy mavenBuildTool creation**: Only created after successful decompilation
- **Delegation to depCache**: Once resolved, uses standard Maven caching via mavenBuildTool

#### 5. Maven Downloader (`bldtool/maven_downloader.go`)

**Responsibility**: Download Maven artifacts using Maven coordinates

**Key Features**:
- Supports `mvn://` URI scheme for artifact locations
- URI format: `mvn://<group>:<artifact>:<version>:<classifier>@<path>`
- Uses `mvn dependency:copy` command to download artifacts
- Supports custom Maven settings files and insecure HTTPS mode

**Example Usage**:
```go
location := "mvn://org.springframework:spring-core:5.3.21@/tmp/downloads"
downloader, ok := bldtool.GetDownloader(location, settingsFile, insecure, logger)
if ok {
    downloadedPath, err := downloader.Download(ctx)
    // downloadedPath points to the downloaded JAR file
}
```

**Download Process**:
```
┌──────────────────────────────────────────────┐
│  1. Parse mvn:// URI                         │
│     - Extract GAV coordinates                │
│     - Extract destination path               │
└──────────────┬───────────────────────────────┘
               │
               v
┌──────────────────────────────────────────────┐
│  2. Build mvn dependency:copy command        │
│     - Add artifact coordinates               │
│     - Add output directory                   │
│     - Add settings file (if specified)       │
│     - Add insecure flag (if specified)       │
└──────────────┬───────────────────────────────┘
               │
               v
┌──────────────────────────────────────────────┐
│  3. Execute Maven command                    │
│     - Parse output for download path         │
│     - Verify file exists                     │
└──────────────┬───────────────────────────────┘
               │
               v
┌──────────────────────────────────────────────┐
│  4. Return path to downloaded artifact       │
└──────────────────────────────────────────────┘
```

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

#### 6. Dependency Labeling (`dependency/labels/`)

**Responsibility**: Label dependencies for classification and filtering

The labels subpackage provides sophisticated labeling functionality to classify Java dependencies, enabling analysis filtering and dependency categorization.

**Location**: `dependency/labels/labels.go`

**Key Features**:
- **Open-Source Detection**: Identifies open-source dependencies vs internal/proprietary code
- **Pattern Matching**: Uses regex patterns to match dependency names
- **Exclusion Lists**: Supports excluding specific packages from analysis
- **Label Types**:
  - `konveyor.io/dep-source`: Source classification (open-source, internal)
  - `konveyor.io/exclude`: Marks dependencies to exclude from analysis
  - `konveyor.io/language`: Language classification (java)

**Labeler Interface**:
```go
type Labeler interface {
    AddLabels(string, bool) []string  // Add labels to a dependency
    HasLabel(string) bool              // Check if pattern exists
}
```

**Configuration**:
- `depOpenSourceLabelsFile`: Path to file containing regex patterns for open-source packages
- `excludePackages`: List of package patterns to exclude from analysis

**Labeling Process**:
```
┌────────────────────────────────────────────────────┐
│ Initialize Labeler                                 │
│  - Load open-source patterns from file             │
│  - Load exclude patterns from config               │
│  - Compile regex patterns                          │
└─────────────────┬──────────────────────────────────┘
                  │
                  v
┌────────────────────────────────────────────────────┐
│ For each dependency (e.g., "org.springframework:...")│
└─────────────────┬──────────────────────────────────┘
                  │
                  v
┌────────────────────────────────────────────────────┐
│ Match against all regex patterns                  │
│  - Check if matches open-source patterns           │
│  - Check if matches exclude patterns               │
└─────────────────┬──────────────────────────────────┘
                  │
                  v
┌────────────────────────────────────────────────────┐
│ Apply labels based on matches                      │
│  - Default: konveyor.io/dep-source=internal        │
│  - If matched or openSource=true:                  │
│    konveyor.io/dep-source=open-source              │
│  - Always: konveyor.io/language=java               │
└────────────────────────────────────────────────────┘
```

**Example Open-Source Patterns File**:
```
^org\.springframework.*
^org\.apache.*
^com\.google.*
^io\.netty.*
```

**Usage in Artifact Identification**:
When identifying JAR coordinates, the labeler is used to determine if a dependency is open-source based on its groupId pattern. This affects how dependencies are classified in analysis results.

#### 7. Platform-Specific Constants

**Files**:
- `dependency/constants.go` (Unix/Linux/macOS)
- `dependency/constants_windows.go` (Windows)

**Responsibility**: Provide platform-specific path constants

These files use Go build tags to define platform-specific path constants:

**Unix/Linux/macOS** (`constants.go`):
```go
const (
    JAVA   = "src/main/java"
    WEBAPP = "src/main/webapp"
)
```

**Windows** (`constants_windows.go`):
```go
const (
    JAVA   = `src\main\java`
    WEBAPP = `src\main\webapp`
)
```

This ensures proper path handling across different operating systems when creating Maven project structures during decompilation.

#### 8. Binary Explosion Utilities

The dependency module includes specialized handlers for exploding (extracting) different types of Java archive files. These utilities are critical for analyzing binary artifacts.

**Base Explosion** (`dependency/explosion.go`):
- `exploadArtifact` - Base type for archive explosion
- Uses `jar -xvf` command to extract archives
- Creates temporary directories for explosion
- Provides foundation for specialized handlers

**JAR Artifact Handler** (`dependency/jar.go`):
- Handles standard JAR files
- Identifies Maven coordinates using `ToDependency()`
- Decompiles JARs without sources using FernFlower
- Creates Maven-style directory structure in local repository
- Copies JAR to `~/.m2/repository` with proper GAV path

**JAR Explosion Handler** (`dependency/jar_explode.go`):
- Handles nested JAR files (e.g., within EAR/WAR archives)
- Walks exploded directory structure
- Identifies and processes embedded JARs in `lib/` directories
- Decompiles class directories using FernFlower
- Creates `src/main/java` structure for decompiled code

**WAR Artifact Handler** (`dependency/war.go`):
- Handles Web Application Archive (.war) files
- Understands WAR structure:
  - `WEB-INF/classes/` → decompiled to `src/main/java/`
  - `WEB-INF/lib/` → treated as dependencies
  - Static resources (css, js, images, html) → moved to `src/main/webapp/`
  - `WEB-INF/web.xml` and other config → preserved in `src/main/webapp/WEB-INF/`
- Automatically decompiles embedded JARs in `WEB-INF/lib/`

**EAR Artifact Handler** (`dependency/ear.go`):
- Handles Enterprise Application Archive (.ear) files
- Complex multi-module structure support
- Smart module detection:
  - Top-level JARs/WARs → decompiled into project (application modules)
  - Nested JARs/WARs → treated as dependencies
- Processes each module independently
- Handles both WAR and JAR modules within EAR

**Explosion Flow for Binary Artifacts**:
```
┌────────────────────────────────────────────────────┐
│ Binary Artifact (JAR/WAR/EAR)                      │
└─────────────────┬──────────────────────────────────┘
                  │
                  v
┌────────────────────────────────────────────────────┐
│ Identify Artifact Type                             │
│  - .jar → jarArtifact                              │
│  - .war → warArtifact                              │
│  - .ear → earArtifact                              │
└─────────────────┬──────────────────────────────────┘
                  │
                  v
┌────────────────────────────────────────────────────┐
│ Explode Archive to Temp Directory                  │
│  - Run: jar -xvf <artifact>                        │
│  - Extract to: /tmp/expload-<name>-<random>        │
└─────────────────┬──────────────────────────────────┘
                  │
        ┌─────────┼─────────┐
        │         │         │
    JAR │     WAR │     EAR │
        v         v         v
┌─────────┐ ┌─────────┐ ┌─────────────────┐
│ Process │ │ Process │ │ Collect modules │
│ classes │ │ WEB-INF │ │ (JARs/WARs)     │
│         │ │         │ │                 │
│ Process │ │ Process │ │ For each module:│
│ META-INF│ │ webapp  │ │  - Top level:   │
│ POM     │ │ content │ │    Decompile    │
└────┬────┘ └────┬────┘ │    into project │
     │           │      │  - Nested:      │
     │           │      │    Decompile as │
     │           │      │    dependency   │
     │           │      └────────┬────────┘
     │           │               │
     └───────────┼───────────────┘
                 │
                 v
┌────────────────────────────────────────────────────┐
│ Decompile .class files using FernFlower            │
│  - Submit jobs to worker pool                      │
│  - Concurrent decompilation (10 workers)           │
└─────────────────┬──────────────────────────────────┘
                  │
                  v
┌────────────────────────────────────────────────────┐
│ Create Maven Project Structure                     │
│  - src/main/java/ (decompiled sources)             │
│  - src/main/webapp/ (web content, WAR only)        │
│  - pom.xml (extracted from META-INF)               │
└─────────────────┬──────────────────────────────────┘
                  │
                  v
┌────────────────────────────────────────────────────┐
│ Copy Dependencies to Local Repository              │
│  - Embedded JARs → ~/.m2/repository/...            │
│  - Maintain Maven GAV structure                    │
└────────────────────────────────────────────────────┘
```

**Key Considerations**:
- All archive types use the worker pool for parallel decompilation
- Embedded dependencies are automatically identified and processed
- Proper Maven directory structure is maintained for JDTLS compatibility
- Temporary explosion directories are cleaned up after processing
- Each artifact type has specific knowledge of its internal structure

---

## Symbol Filtering and Incident Conversion

The Java provider includes sophisticated symbol filtering capabilities to convert Language Server Protocol (LSP) workspace symbols into incident contexts for rule evaluation.

### Location
`external-providers/java-external-provider/pkg/java_external_provider/filter.go`

### Core Functionality

The filtering system bridges the gap between JDTLS symbol search results and the analyzer's incident reporting format.

**Key Constants**:
```go
const (
    LINE_NUMBER_EXTRA_KEY = "lineNumber"
    KIND_EXTRA_KEY        = "kind"
    SYMBOL_NAME_KEY       = "name"
    FILE_KEY              = "file"
)
```

### Filter Functions

Different filter functions handle different types of code locations:

**1. `filterVariableDeclaration()`**:
- Filters symbols related to variable declarations
- Converts each symbol to an incident context
- Useful for finding variable usage patterns

**2. `filterModulesImports()`**:
- Filters for module import symbols (`protocol.Module` kind)
- Identifies dependency import statements
- Helps detect deprecated or prohibited imports

**3. `filterTypesInheritance()`**:
- Filters symbols based on type inheritance
- Identifies class/interface hierarchies
- Useful for detecting extension of specific base classes

**4. `filterMethodSymbols()`**:
- Filters method-related symbols
- Captures method calls and declarations
- Currently returns all methods (filtration concept for future enhancement)

**5. `filterDefault()`**:
- Generic filter for unspecified location types
- Converts all symbols to incident contexts

### Symbol to Incident Conversion

**`convertToIncidentContext()`** - Core conversion function:

**Process**:
```
┌─────────────────────────────────────────────────────┐
│ Input: protocol.WorkspaceSymbol from JDTLS          │
└──────────────────┬──────────────────────────────────┘
                   │
                   v
┌─────────────────────────────────────────────────────┐
│ Extract Location Information                        │
│  - Document URI (file path or class file URI)       │
│  - Range (start/end line and character positions)   │
└──────────────────┬──────────────────────────────────┘
                   │
                   v
┌─────────────────────────────────────────────────────┐
│ Process URI with getURI()                           │
│  - Application source: Parse file for package name  │
│  - Dependency (konveyor-jdt://): Parse class file   │
│    URI and locate decompiled source                 │
└──────────────────┬──────────────────────────────────┘
                   │
                   v
┌─────────────────────────────────────────────────────┐
│ Create IncidentContext                              │
│  - FileURI: Resolved file path                      │
│  - LineNumber: Incident line number                 │
│  - CodeLocation: Start/end positions                │
│  - Variables: Metadata (kind, name, package, file)  │
│  - IsDependencyIncident: true if from decompiled    │
└─────────────────────────────────────────────────────┘
```

**Special URI Handling** (`getURI()`):

The function handles two types of URIs:

1. **Regular File URIs** (`file://...`):
   - Parses the Java file to extract package name
   - Reads file and searches for `package` declaration
   - Handles edge cases (comments, licenses with word "package")

2. **JDT Class File URIs** (`konveyor-jdt://...`):
   - Special format for decompiled dependency classes
   - URI parameters include:
     - `source-range`: Whether sources JAR exists
     - `packageName`: Fully qualified class name
   - Resolves to actual decompiled `.java` file location
   - Uses `buildTool.GetSourceFileLocation()` to find file
   - Handles inner classes (e.g., `OuterClass$InnerClass`)

**Example JDT URI**:
```
konveyor-jdt://contents/.m2/repository/org/apache/logging/log4j/log4j-core/2.14.1/log4j-core-2.14.1.jar?source-range=true&packageName=org.apache.logging.log4j.core.appender.FileManager.class
```

### Incident Context Output

**IncidentContext Structure**:
```go
type IncidentContext struct {
    FileURI              uri.URI
    LineNumber           *int
    CodeLocation         *Location  // Start/end positions
    IsDependencyIncident bool       // True for decompiled deps
    Variables            map[string]interface{} {
        "kind":    "Class|Method|Field|..."
        "name":    "symbolName"
        "file":    "/path/to/file.java"
        "package": "com.example.package"
    }
}
```

**Usage in Rule Evaluation**:
```
┌────────────────────────────────────────┐
│ Rule Condition evaluated via JDTLS     │
└─────────────────┬──────────────────────┘
                  │
                  v
┌────────────────────────────────────────┐
│ JDTLS returns WorkspaceSymbol[]        │
└─────────────────┬──────────────────────┘
                  │
                  v
┌────────────────────────────────────────┐
│ Apply appropriate filter function      │
│  - Based on rule location type         │
│  - e.g., "inheritance" → filterTypes   │
└─────────────────┬──────────────────────┘
                  │
                  v
┌────────────────────────────────────────┐
│ Convert each symbol to IncidentContext │
│  - Extract file location               │
│  - Add metadata                        │
│  - Mark dependency incidents           │
└─────────────────┬──────────────────────┘
                  │
                  v
┌────────────────────────────────────────┐
│ Return IncidentContext[] to analyzer   │
│  - Ready for violation reporting       │
└────────────────────────────────────────┘
```

**Key Features**:
- Handles both application code and dependency code uniformly
- Automatically resolves decompiled source locations
- Extracts package information for better context
- Marks incidents from dependencies separately
- Provides rich metadata for rule evaluation
- Supports all LSP symbol kinds (Class, Method, Field, etc.)

---

## Code Snippet Extraction

The Java provider implements code snippet extraction to provide contextual code around incidents for better understanding and reporting.

### Location
`external-providers/java-external-provider/pkg/java_external_provider/snipper.go`

### Core Interface

Implements the `engine.CodeSnip` interface from the analyzer engine.

### Key Components

**`GetCodeSnip(u uri.URI, loc engine.Location)`**:
- Main entry point for snippet extraction
- Validates URI is a file URI
- Delegates to `scanFile()` for actual extraction

**`scanFile(path string, loc engine.Location)`**:
- Opens and scans the Java source file
- Extracts code around the specified location
- Includes configurable context lines before and after

### Snippet Extraction Process

```
┌─────────────────────────────────────────────────┐
│ Input: File URI + Location (line range)        │
└──────────────────┬──────────────────────────────┘
                   │
                   v
┌─────────────────────────────────────────────────┐
│ Validate URI                                    │
│  - Must be file:// scheme                      │
└──────────────────┬──────────────────────────────┘
                   │
                   v
┌─────────────────────────────────────────────────┐
│ Open Source File                                │
│  - Use URI.Filename() to get path              │
└──────────────────┬──────────────────────────────┘
                   │
                   v
┌─────────────────────────────────────────────────┐
│ Scan File Line by Line                          │
│  - Track current line number                   │
│  - Determine snippet boundaries:               │
│    * Start: loc.StartLine - contextLines       │
│    * End: loc.EndLine + contextLines           │
└──────────────────┬──────────────────────────────┘
                   │
                   v
┌─────────────────────────────────────────────────┐
│ Build Formatted Snippet                         │
│  - Prefix each line with line number           │
│  - Right-align line numbers with padding       │
│  - Format: "  <lineNum>  <code>"               │
└──────────────────┬──────────────────────────────┘
                   │
                   v
┌─────────────────────────────────────────────────┐
│ Return Code Snippet String                      │
└─────────────────────────────────────────────────┘
```

### Configuration

**Context Lines**: Controlled by `p.contextLines` field
- Configurable number of lines before and after the incident location
- Provides surrounding code for better context
- Default value can be set during provider initialization

### Example Output

For an incident at line 42 with 2 context lines:

```
  40  public class Example {
  41      private String name;
  42      public void problematicMethod() {  // <- Incident here
  43          // method body
  44      }
```

### Features

- **Line Number Formatting**:
  - Right-aligned for clean presentation
  - Padding calculated based on max line number in snippet
  - Makes it easy to identify exact incident location

- **Context Awareness**:
  - Includes surrounding code for understanding
  - Helps developers see incident in context
  - Configurable context size

- **File URI Support**:
  - Works with standard file:// URIs
  - Compatible with decompiled source files
  - Validates URI scheme before processing

### Integration with Incident Reporting

```
┌────────────────────────────────────┐
│ Incident detected in Java code     │
└─────────────────┬──────────────────┘
                  │
                  v
┌────────────────────────────────────┐
│ IncidentContext has FileURI and    │
│ CodeLocation with line range       │
└─────────────────┬──────────────────┘
                  │
                  v
┌────────────────────────────────────┐
│ Analyzer calls GetCodeSnip()       │
│  - Passes FileURI and Location     │
└─────────────────┬──────────────────┘
                  │
                  v
┌────────────────────────────────────┐
│ Provider extracts code snippet     │
│  - Opens file at URI               │
│  - Reads lines around location     │
│  - Formats with line numbers       │
└─────────────────┬──────────────────┘
                  │
                  v
┌────────────────────────────────────┐
│ Snippet included in incident report│
│  - Displayed to user               │
│  - Written to output file          │
└────────────────────────────────────┘
```

**Error Handling**:
- Returns error if URI is not a file URI
- Returns error if file cannot be opened
- Logs errors at appropriate verbosity levels

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

The `javaServiceClient` (`service_client.go:27-45`) is the main interface for analyzing Java code:

**Key Components**:
```go
type javaServiceClient struct {
    rpc                provider.RPCClient    // JSON-RPC to JDTLS
    cancelFunc         context.CancelFunc    // Cancel function for cleanup
    config             provider.InitConfig   // Configuration
    log                logr.Logger           // Logger instance
    cmd                *exec.Cmd             // JDTLS process
    bundles            []string              // OSGi bundles for JDTLS
    workspace          string                // Workspace directory
    isLocationBinary   bool                  // Whether analyzing binary artifact
    globalSettings     string                // Global settings file path
    includedPaths      []string              // Paths to include in analysis
    cleanExplodedBins  []string              // Binary explosion dirs to clean up
    disableMavenSearch bool                  // Whether to disable Maven lookups
    activeRPCCalls     sync.WaitGroup        // Tracks active RPC calls
    depsLocationCache  map[string]int        // Cache for dependency locations
    buildTool          bldtool.BuildTool     // Reference to build tool
    mvnIndexPath       string                // Maven index for labeling
    mvnSettingsFile    string                // Maven settings file
}
```

**Note**: As of commit 7b864b5, `depsCache`, `depsMutex`, and `depsErrCache` fields have been removed. Dependency caching is now handled by the BuildTool implementations.

**Key Methods**:

1. **Evaluate()** (`service_client.go:49+`)
   - Evaluates rule conditions using JDTLS
   - Calls `GetAllSymbols()` to query code
   - Filters results based on location type (inheritance, method calls, etc.)

2. **GetDependencies()** (via BuildTool)
   - Returns dependency DAG for the project
   - Delegates to BuildTool which handles caching internally

3. **GetAllSymbols()** (`service_client.go:111+`)
   - Sends workspace/executeCommand to JDTLS
   - Command: `io.konveyor.tackle.ruleEntry`
   - Returns matching symbols from codebase

### Dependency Caching and Retrieval

The service client in `dependency.go` provides a simplified interface for dependency retrieval. Caching is now handled internally by the BuildTool implementations.

**Key Methods**:

1. **`GetDependencies(ctx context.Context)`** - Returns flattened dependency list
   - Calls `GetDependenciesDAG()` internally
   - Converts DAG structure to flat list
   - Uses `provider.ConvertDagItemsToList()` for transitive dependencies

2. **`GetDependenciesDAG(ctx context.Context)`** - Returns dependency DAG
   - **Simplified**: No longer manages cache or synchronization at this level
   - Directly delegates to `buildTool.GetDependencies(ctx)`
   - BuildTool handles all caching and thread-safety internally

**Simplified Retrieval Flow**:
```
┌──────────────────────────────────────────────┐
│ User requests dependency analysis            │
└──────────────┬───────────────────────────────┘
               │
               v
┌──────────────────────────────────────────────┐
│ Service Client: GetDependenciesDAG()         │
│  - No locks needed at this level             │
└──────────────┬───────────────────────────────┘
               │
               v
┌──────────────────────────────────────────────┐
│ BuildTool: GetDependencies(ctx)              │
│  - BuildTool manages its own cache/locks     │
│  - For Maven/Gradle: uses depCache           │
│  - For Binary: uses resolveSync              │
└──────────────┬───────────────────────────────┘
               │
               v
┌──────────────────────────────────────────────┐
│ Return map[uri.URI][]provider.DepDAGItem    │
│  - Key: Build file URI                       │
│  - Value: List of dependencies with DAG      │
└──────────────────────────────────────────────┘
```

**Architectural Benefits of Refactoring**:
- **Separation of concerns**: Service client no longer manages caching
- **Encapsulation**: Each BuildTool controls its own synchronization strategy
- **Reduced complexity**: Eliminated `depsMutex`, `depsCache`, and `depsErrCache` from service client
- **Consistent behavior**: All build tools use same depCache pattern
- **Thread-safety**: Moved from service client level to BuildTool level where it belongs

**Dual Interface**:
- `GetDependencies()`: Returns flat list (`[]*provider.Dep`)
- `GetDependenciesDAG()`: Returns DAG structure (`[]provider.DepDAGItem`)
- Both delegate to BuildTool's internal implementation
- DAG structure preserves transitive dependency relationships

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

**Build Tool Module**:
- **bldtool/tool.go**: Main BuildTool interface and factory
- **bldtool/dep_cache.go**: Shared dependency caching mechanism for all build tools
- **bldtool/maven.go**: Maven build tool implementation
- **bldtool/gradle.go**: Gradle build tool implementation
- **bldtool/maven_binary.go**: Binary artifact handling with resolution synchronization
- **bldtool/maven_shared.go**: Shared Maven functionality
- **bldtool/maven_downloader.go**: Maven artifact downloader with mvn:// URI support

**Dependency Module**:
- **dependency/resolver.go**: Resolver interface
- **dependency/maven_resolver.go**: Maven source resolution
- **dependency/gradle_resolver.go**: Gradle source resolution
- **dependency/binary_resolver.go**: Binary artifact resolution
- **dependency/decompile.go**: Decompilation engine with worker pool
- **dependency/artifact.go**: JAR artifact identification
- **dependency/explosion.go**: Base archive explosion utilities
- **dependency/jar.go**: JAR artifact handler
- **dependency/jar_explode.go**: JAR explosion handler for nested archives
- **dependency/war.go**: WAR artifact handler with web structure support
- **dependency/ear.go**: EAR artifact handler for enterprise applications
- **dependency/labels/labels.go**: Dependency labeling and classification
- **dependency/constants.go**: Platform-specific path constants (Unix/Linux/macOS)
- **dependency/constants_windows.go**: Platform-specific path constants (Windows)

**Provider Core**:
- **provider.go**: Java provider initialization and lifecycle
- **service_client.go**: Service client for analysis operations
- **dependency.go**: Dependency caching and retrieval layer
- **filter.go**: Symbol filtering and incident conversion
- **snipper.go**: Code snippet extraction for incidents

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
