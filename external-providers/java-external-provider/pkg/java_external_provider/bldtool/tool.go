package bldtool

import (
	"context"
	"path"
	"strings"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency"
	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency/labels"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
)

// keys used in dep.Extras for extra information about a dep
const (
	artifactIdKey = "artifactId"
	groupIdKey    = "groupId"
	pomPathKey    = "pomPath"
	baseDepKey    = "baseDep"
)

const (
	maven  = "maven"
	gradle = "gradle"
)

const (
	gradleDepErr   = "gradleErr"
	fallbackDepErr = "fallbackDepErr"
)

type Downloader interface {
	Download(context.Context) (string, error)
}

// BuildTool provides a unified interface for interacting with different Java build systems
// and binary artifacts. It abstracts dependency extraction, source resolution, and caching
// across different build tool implementations.
type BuildTool interface {
	// GetDependencies retrieves all project dependencies as a directed acyclic graph (DAG).
	// It executes the underlying build tool to extract the complete dependency tree,
	// including both direct and transitive dependencies.
	//
	// The method caches results based on build file hash to avoid repeated expensive executions.
	// Cache is invalidated when the build file changes.
	//
	// Returns:
	//   - map[uri.URI][]provider.DepDAGItem: Map of build file URIs to dependency DAG items
	//     Key: URI of the build file (e.g., file:///path/to/pom.xml or build.gradle)
	//     Value: Slice of dependency DAG items with hierarchy information
	//   - error: Error if dependency resolution fails
	//
	// Example:
	//   deps, err := buildTool.GetDependencies(ctx)
	//   for buildFileURI, dagItems := range deps {
	//       // dagItems contains direct deps and their transitive deps
	//   }
	GetDependencies(ctx context.Context) (map[uri.URI][]provider.DepDAGItem, error)

	// GetLocalRepoPath returns the path to the local dependency repository where
	// dependency JARs and their sources are stored. May return empty string if
	// the build tool uses a different caching mechanism.
	//
	// This path is used to locate dependency JARs and their sources for decompilation
	// and source file location resolution.
	GetLocalRepoPath() string

	// GetSourceFileLocation resolves the absolute path to a decompiled Java source file
	// within a dependency JAR. This is critical for converting JDT class file URIs
	// (konveyor-jdt://) to actual file paths for incident reporting.
	//
	// Parameters:
	//   - packagePath: Package path derived from class name (e.g., "org/apache/logging/log4j/core/appender")
	//   - jarPath: Absolute path to the dependency JAR file
	//   - javaFileName: Name of the Java source file (e.g., "FileManager.java")
	//
	// Returns:
	//   - string: Absolute path to the decompiled .java file
	//   - error: Error if file cannot be located or decompiled
	//
	// Behavior:
	//   - Searches local repository structure for decompiled sources
	//   - Triggers on-demand decompilation if source doesn't exist
	//
	// Example:
	//   path, err := buildTool.GetSourceFileLocation(
	//       "org/springframework/core",
	//       "/home/user/.m2/repository/org/springframework/spring-core/5.3.21/spring-core-5.3.21.jar",
	//       "SpringApplication.java",
	//   )
	//   // Returns absolute path to the .java file
	GetSourceFileLocation(packagePath string, jarPath string, javaFileName string) (string, error)

	// GetResolver creates a dependency resolver appropriate for this build tool.
	// The resolver handles downloading dependency sources and decompiling JARs
	// that don't have source JARs available.
	//
	// Parameters:
	//   - decompileTool: Absolute path to the FernFlower decompiler JAR
	//
	// Returns:
	//   - dependency.Resolver: Build tool-specific resolver implementation
	//   - error: Error if resolver cannot be created
	//
	// The resolver will be used by the provider during initialization if ShouldResolve()
	// returns true or if running in FullAnalysisMode.
	GetResolver(decompileTool string) (dependency.Resolver, error)

	// ShouldResolve indicates whether source resolution must be performed for this build tool.
	//
	// Returns:
	//   - bool: true if resolution is required (e.g., binary artifacts that need decompilation),
	//           false if resolution can be deferred to standard build tool source download
	//
	// When true, the provider will automatically call GetResolver() and resolver.ResolveSources()
	// during initialization to ensure the project can be analyzed.
	//
	// Note: Even when false, source resolution may still occur if FullAnalysisMode is enabled
	// to ensure all dependency sources are available for deep analysis.
	ShouldResolve() bool
}

// BuildToolOptions contains configuration options for creating and initializing
// build tool instances. These options are used by GetBuildTool to detect the
// project type and create the appropriate BuildTool implementation.
//
// The options control:
//   - Project location and dependency configuration
//   - Maven-specific settings (repository, settings file, security)
//   - Gradle-specific settings (custom task files)
//   - Dependency labeling and Maven search behavior
//   - Binary cleanup preferences
type BuildToolOptions struct {
	Config          provider.InitConfig // Base provider configuration including project location
	MvnSettingsFile string              // Path to Maven settings.xml for custom repository configuration
	MvnInsecure     bool                // Allow insecure HTTPS connections to Maven repositories
	MavenIndexPath  string              // Path to Maven index for artifact metadata searches
	Labeler         labels.Labeler      // Labeler for classifying dependencies as open source or internal
	CleanBin        bool                // Whether to clean up temporary binary decompilation artifacts
	GradleTaskFile  string              // Path to custom Gradle task file for dependency resolution
}

func GetBuildTool(opts BuildToolOptions, log logr.Logger) BuildTool {
	extension := strings.ToLower(path.Ext(opts.Config.Location))
	isBinary := false
	if extension == dependency.JavaArchive || extension == dependency.EnterpriseArchive || extension == dependency.WebArchive {
		isBinary = true
	}

	if bt := getGradleBuildTool(opts, log); bt != nil {
		log.Info("getting gradle build tool")
		return bt
	} else if isBinary {
		log.Info("getting maven binary build tool")
		return getMavenBinaryBuildTool(opts, log)
	} else if bt := getMavenBuildTool(opts, log); bt != nil {
		log.Info("getting maven build tool")
		return bt
	}
	return nil
}
