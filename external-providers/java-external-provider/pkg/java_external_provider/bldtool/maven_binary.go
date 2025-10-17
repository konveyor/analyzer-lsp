package bldtool

import (
	"context"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency"
	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency/labels"
	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
)

type mavenBinaryBuildTool struct {
	mavenBaseTool
	binaryLocation     string
	binaryLocationHash *string
	disableMavenSearch bool
	dependencyPath     string
	log                logr.Logger
}

func getMavenBinaryBuildTool(opts BuildToolOptions, log logr.Logger) BuildTool {
	if opts.Config.Location == "" {
		return nil
	}
	if _, err := os.Stat(opts.Config.Location); err != nil {
		return nil
	}
	mavenBaseTool := mavenBaseTool{
		mvnInsecure:     opts.MvnInsecure,
		mvnSettingsFile: opts.MvnSettingsFile,
		mvnIndexPath:    opts.MvnIndexPath,
		log:             log,
		labeler:         opts.Labeler,
	}
	mvnLocalRepo := mavenBaseTool.getMavenLocalRepoPath()
	mavenBaseTool.mvnLocalRepo = mvnLocalRepo
	return &mavenBinaryBuildTool{
		binaryLocation: opts.Config.Location,
		mavenBaseTool:  mavenBaseTool,
	}

}

func (m *mavenBaseTool) ShouldResolve() bool {
	return true
}

func (m *mavenBinaryBuildTool) GetResolver(decompileTool string) (dependency.Resolver, error) {
	opts := dependency.ResolverOptions{
		Log:           logr.Logger{},
		Location:      filepath.Dir(m.binaryLocation),
		BuildFile:     m.mvnSettingsFile,
		LocalRepo:     m.mvnLocalRepo,
		Insecure:      m.mvnInsecure,
		DecompileTool: decompileTool,
		Labeler:       m.labeler,
	}
	return dependency.GetBinaryResolver(opts), nil
}

func (m *mavenBinaryBuildTool) GetSourceFileLocation(string, string, string) (string, error) {
	return "", fmt.Errorf("Binaries should be decompled and treated like maven repos")
}

func (m *mavenBinaryBuildTool) UseCache() (bool, error) {
	hashString, err := getHash(m.binaryLocation)
	if err != nil {
		m.log.Error(err, "unable to generate hash from pom file")
		return false, err
	}
	if m.binaryLocationHash != nil && *m.binaryLocationHash == hashString {
		return true, nil
	}
	return false, nil
}

func (m *mavenBinaryBuildTool) GetCachedDepError(errorCached map[string]error) (error, bool) {
	fallbackErr, hasFallbackErr := errorCached[fallbackDepErr]
	return fallbackErr, hasFallbackErr
}

func (m *mavenBinaryBuildTool) GetDependencies(ctx context.Context) (map[uri.URI][]provider.DepDAGItem, error) {
	hash, err := getHash(m.binaryLocation)
	if err != nil {
		return nil, fmt.Errorf("unable to generate hash")
	}
	m.binaryLocationHash = &hash
	depMap := map[uri.URI][]provider.DepDAGItem{}
	m.discoverDepsFromJars(m.dependencyPath, depMap, m.disableMavenSearch)
	if len(depMap) == 0 {
		m.log.Info("unable to get dependencies from jars, looking for pom")
		pomPaths := m.discoverPoms(m.dependencyPath, depMap)
		for _, path := range pomPaths {
			dep, err := m.GetDependenciesFallback(ctx, path)
			if err != nil {
				return nil, err
			}
			maps.Copy(depMap, dep)
		}
	}
	if len(depMap) == 0 {
		m.log.Error(fmt.Errorf("unable to get dependencies for binary"), "unable to find dependencies for binary")
		return nil, fmt.Errorf("unable to get dependnecies for binary")
	}
	return depMap, nil

}

// discoverDepsFromJars walks given path to discover dependencies embedded as JARs
func (m *mavenBinaryBuildTool) discoverDepsFromJars(path string, ll map[uri.URI][]konveyor.DepDAGItem, disableMavenSearch bool) {
	// for binaries we only find JARs embedded in archive
	w := walker{
		deps:               ll,
		labeler:            m.labeler,
		m2RepoPath:         m.mvnLocalRepo,
		seen:               map[string]bool{},
		initialPath:        path,
		log:                m.log,
		disableMavenSearch: disableMavenSearch,
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

type walker struct {
	deps               map[uri.URI][]provider.DepDAGItem
	labeler            labels.Labeler
	m2RepoPath         string
	initialPath        string
	seen               map[string]bool
	pomPaths           []string
	log                logr.Logger
	disableMavenSearch bool
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
		artifact, _ := dependency.ToDependency(context.TODO(), w.log, w.labeler, path, w.disableMavenSearch)
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
	if strings.Contains(info.Name(), "pom.xml") {
		w.pomPaths = append(w.pomPaths, path)
	}
	return nil
}
