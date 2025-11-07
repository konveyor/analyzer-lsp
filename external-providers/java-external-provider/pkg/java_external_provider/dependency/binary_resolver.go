package dependency

import (
	"context"
	"path/filepath"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency/labels"
)

// binaryDependencyResolver implements the Resolver interface for binary Java artifacts.
// It decompiles JAR/WAR/EAR files without source code, creating a synthetic Maven project
// structure suitable for analysis.
//
// The resolver:
//   - Decompiles the binary artifact into a "java-project" directory
//   - Extracts embedded dependencies from the binary
//   - Generates a pom.xml with discovered dependencies
//   - Stores decompiled sources in Maven repository structure
type binaryDependencyResolver struct {
	decompileTool  string         // Path to FernFlower decompiler JAR
	labeler        labels.Labeler // Labeler for dependency classification
	localRepo      string         // Path to Maven local repository
	log            logr.Logger    // Logger for resolver operations
	settingsFile   string         // Path to Maven settings file (currently unused for binary)
	insecure       bool           // Allow insecure HTTPS (currently unused for binary)
	location       string         // Absolute path to the binary artifact file
	cleanBin       bool           // Whether to clean up temporary binary files (currently unused)
	mavenIndexPath string         // Path to Maven index for artifact lookups
}

// GetBinaryResolver creates a new binary dependency resolver with the provided options.
// The resolver is used for analyzing standalone binary artifacts (JAR/WAR/EAR)
// without accompanying source code or build files.
func GetBinaryResolver(options ResolverOptions) Resolver {
	log := options.Log.WithName("binary-resolver")
	return &binaryDependencyResolver{
		localRepo:      options.LocalRepo,
		settingsFile:   options.BuildFile,
		insecure:       options.Insecure,
		location:       options.Location,
		log:            log,
		decompileTool:  options.DecompileTool,
		labeler:        options.Labeler,
		mavenIndexPath: options.MavenIndexPath,
	}
}

func (m *binaryDependencyResolver) ResolveSources(ctx context.Context) (string, string, error) {
	projectPath := filepath.Join(filepath.Dir(m.location), "java-project")
	// And whatever else we need
	decompiler, err := getDecompiler(DecompilerOpts{
		DecompileTool:  m.decompileTool,
		log:            m.log,
		workers:        DefaultWorkerPoolSize,
		labler:         m.labeler,
		m2Repo:         m.localRepo,
		mavenIndexPath: m.mavenIndexPath,
	})
	if err != nil {
		return "", "", err
	}

	dependencies, err := decompiler.DecompileIntoProject(ctx, m.location, projectPath)
	if err != nil {
		return "", "", err
	}

	//removeIncompleteDependencies(deduplicateJavaArtifacts(deps))
	err = createJavaProject(ctx, projectPath, dependencies)
	if err != nil {
		m.log.Error(err, "failed to create java project", "path", projectPath)
		return "", "", err
	}
	m.log.V(5).Info("created java project", "path", projectPath)
	return projectPath, m.localRepo, nil
}
