package dependency

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"text/template"

	"github.com/go-logr/logr"
	"github.com/hashicorp/go-version"
	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency/labels"
	"github.com/vifraa/gopom"
)

// Resolver handles downloading and decompiling dependency sources for different build systems.
// It ensures that all project dependencies have accessible source code for analysis, either by
// downloading source JARs from repositories or by decompiling binary JARs using a decompiler.
//
// The resolver is obtained from BuildTool.GetResolver() and is automatically invoked during
// provider initialization when BuildTool.ShouldResolve() returns true or when running in
// FullAnalysisMode.
type Resolver interface {
	// ResolveSources downloads dependency sources and decompiles JARs that lack source artifacts.
	// This is a critical step for enabling deep code analysis, as it ensures the language server
	// has access to all dependency source code.
	//
	// Process:
	//   1. Execute build tool command to download available source JARs
	//   2. Parse output to identify dependencies without sources
	//   3. Locate binary JARs for unresolved dependencies
	//   4. Decompile missing sources using a decompiler (parallel worker pool)
	//   5. Store decompiled sources in appropriate repository structure
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout control (typically 5-10 minute timeout)
	//
	// Returns:
	//   - sourceLocation (string): Absolute path to project source directory
	//     For source projects: Original project location
	//     For binary artifacts: Path to generated project directory
	//   - dependencyLocation (string): Absolute path to local dependency repository
	//     May be empty string if the build tool uses a different caching mechanism
	//   - error: Error if source resolution fails
	//
	// Example Usage:
	//   resolver, _ := buildTool.GetResolver("/path/to/decompiler.jar")
	//   srcPath, depPath, err := resolver.ResolveSources(ctx)
	//   if err != nil {
	//       // Handle resolution failure
	//   }
	//   // srcPath: Project directory with sources
	//   // depPath: Repository with dependency sources (may be empty)
	//
	// Performance Considerations:
	//   - Uses worker pool for parallel decompilation
	//   - Can take several minutes for large projects with many dependencies
	//   - Progress logged at various verbosity levels
	//   - Individual decompilation failures logged but don't stop overall process
	//
	// Error Handling:
	//   - Returns error if build tool command fails completely
	//   - Returns error if decompiler initialization fails
	//   - Logs individual JAR decompilation failures but continues
	//   - May cache errors to avoid repeated failures
	ResolveSources(ctx context.Context) (sourceLocation string, dependencyLocation string, err error)
}

// ResolverOptions contains configuration options for creating build tool-specific resolvers.
// Different resolvers use different subsets of these options based on their requirements.
type ResolverOptions struct {
	// Log is the logger instance for logging resolver operations.
	// Used by all resolver types for progress tracking and error reporting.
	Log logr.Logger

	// Location is the absolute path to the project directory or binary artifact.
	// Points to the root of the project or the binary file to be analyzed.
	Location string

	// DecompileTool is the absolute path to the decompiler JAR.
	// Required by all resolver types for decompiling dependencies without sources.
	DecompileTool string

	// Labeler identifies whether dependencies are open source or internal.
	// Used to determine if remote repository lookups should be attempted.
	Labeler labels.Labeler

	// LocalRepo is the path to the local dependency repository where
	// dependencies and their sources are cached.
	// May not be used by all build tools.
	LocalRepo string

	// DisableMavenSearch disables lookups to remote repositories for artifact identification.
	// When true, relies solely on embedded metadata and JAR structure analysis.
	// Useful in air-gapped environments or to reduce network traffic.
	DisableMavenSearch bool

	// BuildFile points to build tool-specific configuration file.
	// May be a settings file or build definition depending on the build tool.
	BuildFile string

	// Insecure allows insecure HTTPS connections when downloading dependencies.
	// Should only be used in development/testing environments.
	Insecure bool

	// Version is the build tool version detected from the project.
	// Used by some resolvers to determine compatibility requirements.
	Version version.Version

	// Wrapper is the absolute path to the build tool wrapper executable.
	// Used by build tools that support wrapper scripts for reproducible builds.
	Wrapper string

	// JavaHome is the path to the Java installation to use for build tool execution.
	// May be set based on build tool version requirements.
	JavaHome string

	// GradleTaskFile is the path to a custom task file for source download.
	// Optional custom task file to use instead of embedded defaults.
	GradleTaskFile string
}

func contains(artifacts []JavaArtifact, artifactToFind JavaArtifact) bool {
	if len(artifacts) == 0 {
		return false
	}

	return slices.Contains(artifacts, artifactToFind)
}

func moveFile(srcPath string, destPath string) error {
	err := CopyFile(srcPath, destPath)
	if err != nil {
		return err
	}
	err = os.Remove(srcPath)
	if err != nil {
		return err
	}
	return nil
}

func CopyFile(srcPath string, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), DirPermRWX); err != nil {
		return err
	}
	inputFile, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer inputFile.Close()
	outputFile, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer outputFile.Close()
	_, err = io.Copy(outputFile, inputFile)
	if err != nil {
		return err
	}
	return nil
}

func AppendToFile(src string, dst string) error {
	// Read the contents of the source file
	content, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("error reading source file: %s", err)
	}

	// Open the destination file in append mode
	destFile, err := os.OpenFile(dst, os.O_APPEND|os.O_WRONLY, FilePermRW)
	if err != nil {
		return fmt.Errorf("error opening destination file: %s", err)
	}
	defer destFile.Close()

	// Append the content to the destination file
	_, err = destFile.Write(content)
	if err != nil {
		return fmt.Errorf("error appending to destination file: %s", err)
	}

	return nil
}

const javaProjectPom = `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
  xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 http://maven.apache.org/xsd/maven-4.0.0.xsd">
  <modelVersion>4.0.0</modelVersion>

  <groupId>io.konveyor</groupId>
  <artifactId>java-project</artifactId>
  <version>1.0-SNAPSHOT</version>

  <name>java-project</name>
  <url>http://www.konveyor.io</url>

  <properties>
    <project.build.sourceEncoding>UTF-8</project.build.sourceEncoding>
  </properties>

  <dependencies>
{{range .}}
    <dependency>
      <groupId>{{.GroupId}}</groupId>
      <artifactId>{{.ArtifactId}}</artifactId>
      <version>{{.Version}}</version>
    </dependency>
{{end}}
  </dependencies>

  <build>
  </build>
</project>
`

func createJavaProject(_ context.Context, dir string, dependencies []JavaArtifact) error {
	tmpl := template.Must(template.New("javaProjectPom").Parse(javaProjectPom))

	err := os.MkdirAll(filepath.Join(dir, "src", "main", "java"), DirPermRWX)
	if err != nil {
		return err
	}

	if _, err = os.Stat(filepath.Join(dir, PomXmlFile)); err == nil {
		// enhance the pom.xml with any dependencies that were found
		// that don't match an existing one.
		pom, err := gopom.Parse(filepath.Join(dir, PomXmlFile))
		if err != nil {
			return err
		}
		if pom.Dependencies == nil {
			pom.Dependencies = &[]gopom.Dependency{}
		}
		var foundUpdates bool
		for _, artifact := range dependencies {
			var found bool
			if slices.ContainsFunc(*pom.Dependencies, artifact.EqualsPomDep) {
				found = true
			}
			if found {
				break
			}
			foundUpdates = true
			*pom.Dependencies = append(*pom.Dependencies, artifact.ToPomDep())
		}
		if foundUpdates {
			pomFile, err := os.OpenFile(filepath.Join(dir, PomXmlFile), os.O_TRUNC|os.O_CREATE|os.O_WRONLY, DirPermRWX)
			if err != nil {
				return err
			}
			defer pomFile.Close()
			output, err := xml.MarshalIndent(pom, "", "  ")
			if err != nil {
				return err
			}
			_, err = pomFile.Write(output)
			if err != nil {
				return err
			}
		}
		return nil
	}

	pom, err := os.OpenFile(filepath.Join(dir, PomXmlFile), os.O_CREATE|os.O_WRONLY, DirPermRWX)
	if err != nil {
		return err
	}

	err = tmpl.Execute(pom, dependencies)
	if err != nil {
		return err
	}
	return nil
}
