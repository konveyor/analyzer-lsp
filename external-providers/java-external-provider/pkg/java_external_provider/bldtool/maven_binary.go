package bldtool

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency"
	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency/labels"
	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
)

// mavenBinaryBuildTool implements the BuildTool interface for binary Java artifacts
// (JAR, WAR, EAR files) without source code. It decompiles binaries into a Maven project
// structure to enable analysis.
//
// This implementation supports:
//   - JAR, WAR, and EAR file analysis
//   - Binary decompilation into source code
//   - Creation of synthetic Maven project structure
//   - Recursive processing of nested archives
//   - Dependency extraction from embedded libraries
//   - Hash-based caching to avoid reprocessing unchanged binaries
//
// The tool creates a "java-project" directory containing decompiled sources
// and a generated pom.xml with discovered dependencies.
type mavenBinaryBuildTool struct {
	mavenBaseTool
	resolveSync        *sync.Mutex
	binaryLocation     string              // Absolute path to the binary artifact (JAR/WAR/EAR)
	disableMavenSearch bool                // Whether to disable Maven repository lookups
	dependencyPath     string              // Path to dependency configuration
	resolver           dependency.Resolver // Resolver for source resolution and decompilation
	mavenBldTool       *mavenBuildTool     // Optional Maven build tool if pom.xml found in binary
}

func getMavenBinaryBuildTool(opts BuildToolOptions, log logr.Logger) BuildTool {
	log = log.WithName("mvn-binary-bldtool")
	if opts.Config.Location == "" {
		return nil
	}
	if _, err := os.Stat(opts.Config.Location); err != nil {
		return nil
	}
	mavenBaseTool := mavenBaseTool{
		mvnInsecure:     opts.MvnInsecure,
		mvnSettingsFile: opts.MvnSettingsFile,
		mavenIndexPath:  opts.MavenIndexPath,
		log:             log,
		labeler:         opts.Labeler,
	}
	mvnLocalRepo := mavenBaseTool.getMavenLocalRepoPath()
	mavenBaseTool.mvnLocalRepo = mvnLocalRepo
	return &mavenBinaryBuildTool{
		binaryLocation: opts.Config.Location,
		resolveSync:    &sync.Mutex{},
		mavenBaseTool:  mavenBaseTool,
	}

}

func (m *mavenBaseTool) ShouldResolve() bool {
	return true
}

func (m *mavenBinaryBuildTool) GetResolver(decompileTool string) (dependency.Resolver, error) {
	opts := dependency.ResolverOptions{
		Log:           m.log,
		Location:      m.binaryLocation,
		BuildFile:     m.mvnSettingsFile,
		LocalRepo:     m.mvnLocalRepo,
		Insecure:      m.mvnInsecure,
		DecompileTool: decompileTool,
		Labeler:       m.labeler,
	}
	m.resolver = dependency.GetBinaryResolver(opts)
	return m, nil
}

func (m *mavenBinaryBuildTool) ResolveSources(ctx context.Context) (string, string, error) {
	m.resolveSync.Lock()
	defer m.resolveSync.Unlock()
	if m.resolver == nil {
		return "", "", errors.New("need to get the resolver")
	}
	projectPath, depPath, err := m.resolver.ResolveSources(ctx)
	if err != nil {
		return "", "", err
	}

	m.mavenBldTool = &mavenBuildTool{
		mavenBaseTool: mavenBaseTool{
			mvnInsecure:     m.mvnInsecure,
			mvnSettingsFile: m.mvnSettingsFile,
			mvnLocalRepo:    m.mvnLocalRepo,
			mavenIndexPath:  m.mavenIndexPath,
			dependencyPath:  depPath,
			log:             m.log,
			labeler:         m.labeler,
		},
		depCache: depCache{
			hashFile: filepath.Join(projectPath, dependency.PomXmlFile),
			hashSync: &sync.Mutex{},
			depLog:   m.log.WithName("dep-cache"),
		},
	}
	_, err = m.mavenBldTool.GetDependencies(ctx)
	if err != nil {
		return projectPath, depPath, err
	}
	return projectPath, depPath, nil
}

func (m *mavenBinaryBuildTool) GetSourceFileLocation(path string, jarPath string, javaFileName string) (string, error) {
	if m.mavenBldTool != nil {
		return m.mavenBldTool.GetSourceFileLocation(path, jarPath, javaFileName)
	}
	return "", fmt.Errorf("binaries should be decompiled and treated like maven repos")
}

func (m *mavenBinaryBuildTool) GetDependencies(ctx context.Context) (map[uri.URI][]provider.DepDAGItem, error) {
	m.resolveSync.Lock()
	defer m.resolveSync.Unlock()
	if m.mavenBldTool != nil {
		m.log.Info("getting dependencies from mavenBldTool for binary")
		return m.mavenBldTool.GetDependencies(ctx)
	}
	return nil, fmt.Errorf("binary is not yet resolved")
}

// discoverDepsFromJars walks given path to discover dependencies embedded as JARs
func (m *mavenBinaryBuildTool) discoverDepsFromJars(path string, ll map[uri.URI][]konveyor.DepDAGItem, mavenIndexPath string) {
	// for binaries we only find JARs embedded in archive
	w := walker{
		deps:           ll,
		labeler:        m.labeler,
		m2RepoPath:     m.mvnLocalRepo,
		seen:           map[string]bool{},
		initialPath:    path,
		log:            m.log,
		mavenIndexPath: mavenIndexPath,
	}
	filepath.WalkDir(path, w.walkDirForJar)
}

func (m *mavenBinaryBuildTool) discoverPoms(pathStart string, ll map[uri.URI][]konveyor.DepDAGItem) []string {
	w := walker{
		deps:        ll,
		labeler:     m.labeler,
		m2RepoPath:  "",
		seen:        map[string]bool{},
		initialPath: pathStart,
		pomPaths:    []string{},
		log:         m.log,
	}
	filepath.WalkDir(pathStart, w.walkDirForPom)
	return w.pomPaths
}

// walker is an internal helper type for traversing decompiled binary artifacts
// to discover dependencies and build a dependency graph. It walks the directory
// structure created by binary decompilation to find JAR files and pom.xml files.
//
// The walker performs:
//   - Recursive directory traversal
//   - JAR file discovery and metadata extraction
//   - POM file location tracking
//   - Dependency deduplication via seen map
//   - Maven repository artifact identification
type walker struct {
	deps           map[uri.URI][]provider.DepDAGItem // Accumulated dependency graph
	labeler        labels.Labeler                    // Labeler for dependency classification
	m2RepoPath     string                            // Maven local repository path
	initialPath    string                            // Starting path for traversal
	seen           map[string]bool                   // Tracks processed artifacts to prevent duplicates
	pomPaths       []string                          // Collected paths to found pom.xml files
	log            logr.Logger                       // Logger instance
	mavenIndexPath string
}

func (w *walker) walkDirForJar(path string, info fs.DirEntry, err error) error {
	if info == nil {
		return nil
	}
	if info.IsDir() {
		return filepath.WalkDir(filepath.Join(path, info.Name()), w.walkDirForJar)
	}
	if strings.HasSuffix(info.Name(), ".jar") {
		seenKey := filepath.Base(info.Name())
		if _, ok := w.seen[seenKey]; ok {
			return nil
		}
		w.seen[seenKey] = true
		d := provider.Dep{
			Name: info.Name(),
		}
		artifact, _ := dependency.ToDependency(context.TODO(), w.log, w.labeler, path, w.mavenIndexPath)
		if (artifact != dependency.JavaArtifact{}) {
			d.Name = fmt.Sprintf("%s.%s", artifact.GroupId, artifact.ArtifactId)
			d.Version = artifact.Version
			d.Labels = w.labeler.AddLabels(d.Name, artifact.FoundOnline)
			d.ResolvedIdentifier = artifact.Sha1
			// when we can successfully get javaArtifact from a jar
			// we added it to the pom and it should be in m2Repo path
			if w.m2RepoPath != "" {
				d.FileURIPrefix = fmt.Sprintf("file://%s", filepath.Join(w.m2RepoPath,
					strings.ReplaceAll(artifact.GroupId, ".", "/"), artifact.ArtifactId, artifact.Version))
			}
		}

		w.deps[uri.URI(filepath.Join(path, info.Name()))] = []provider.DepDAGItem{
			{
				Dep: d,
			},
		}
	}
	if strings.HasSuffix(info.Name(), ".class") {
		// If the class is in WEB-INF we assume this is apart of the application
		relPath, _ := filepath.Rel(w.initialPath, path)
		relPath = filepath.Dir(relPath)
		if strings.Contains(relPath, "WEB-INF") {
			return nil
		}
		if _, ok := w.seen[relPath]; ok {
			return nil
		}
		d := provider.Dep{
			Name: info.Name(),
		}
		artifact, _ := dependency.ToFilePathDependency(context.Background(), filepath.Join(relPath, info.Name()))
		if (artifact != dependency.JavaArtifact{}) {
			d.Name = fmt.Sprintf("%s.%s", artifact.GroupId, artifact.ArtifactId)
			d.Version = artifact.Version
			d.Labels = w.labeler.AddLabels(d.Name, artifact.FoundOnline)
			d.ResolvedIdentifier = artifact.Sha1
			// when we can successfully get javaArtifact from a jar
			// we added it to the pom and it should be in m2Repo path
			d.FileURIPrefix = fmt.Sprintf("file://%s", filepath.Join("java-project", "src", "main",
				strings.ReplaceAll(artifact.GroupId, ".", "/"), artifact.ArtifactId))
		}
		w.deps[uri.URI(filepath.Join(relPath))] = []provider.DepDAGItem{
			{
				Dep: d,
			},
		}
		w.seen[relPath] = true
	}
	return nil
}

func (w *walker) walkDirForPom(path string, info fs.DirEntry, err error) error {
	if info == nil {
		return nil
	}
	if info.IsDir() {
		return filepath.WalkDir(filepath.Join(path, info.Name()), w.walkDirForPom)
	}
	if strings.Contains(info.Name(), dependency.PomXmlFile) {
		w.pomPaths = append(w.pomPaths, path)
	}
	return nil
}
