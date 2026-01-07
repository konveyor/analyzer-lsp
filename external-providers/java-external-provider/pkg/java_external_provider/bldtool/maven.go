package bldtool

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
)

const (
	mavenDepErr = "mvnErr"
)

// mavenBuildTool implements the BuildTool interface for Maven-based Java projects.
// It handles projects with a pom.xml file, extracting dependencies and source locations
// using Maven commands and parsing the POM structure.
//
// This implementation supports:
//   - Standard Maven projects with pom.xml
//   - Multi-module Maven projects
//   - Dependency resolution via mvn dependency:tree
//   - Caching based on pom.xml hash to avoid redundant processing
//   - Fallback parsing when Maven commands fail
type mavenBuildTool struct {
	mavenBaseTool
	*depCache
}

func getMavenBuildTool(opts BuildToolOptions, log logr.Logger) BuildTool {
	log = log.WithName("mvn-bldtool")
	var depPath string
	if opts.Config.DependencyPath == "" {
		depPath = dependency.PomXmlFile
	} else {
		depPath = opts.Config.DependencyPath
	}
	f, err := filepath.Abs(filepath.Join(opts.Config.Location, depPath))
	if err != nil {
		return nil
	}
	if _, err := os.Stat(f); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	mavenBaseTool := mavenBaseTool{
		mvnInsecure:     opts.MvnInsecure,
		mvnSettingsFile: opts.MvnSettingsFile,
		mavenIndexPath:  opts.MavenIndexPath,
		log:             log,
		labeler:         opts.Labeler,
	}
	mvnLocalRepo := mavenBaseTool.getMavenLocalRepoPath(log)
	mavenBaseTool.mvnLocalRepo = mvnLocalRepo
	return &mavenBuildTool{
		depCache: &depCache{
			hashFile: f,
			hashSync: sync.Mutex{},
			depLog:   log.WithName("dep-cache"),
		},
		mavenBaseTool: mavenBaseTool,
	}
}

func (m *mavenBuildTool) ShouldResolve() bool {
	return false
}

func (m *mavenBuildTool) GetSourceFileLocation(packagePath string, jarPath string, javaFileName string) (string, error) {
	javaFileAbsolutePath := filepath.Join(filepath.Dir(jarPath), filepath.Dir(packagePath), javaFileName)

	// attempt to decompile when directory for the expected java file doesn't exist
	// if directory exists, assume .java file is present within, this avoids decompiling every Jar
	if _, err := os.Stat(filepath.Dir(javaFileAbsolutePath)); err != nil {
		cmd := exec.Command("jar", "xf", filepath.Base(jarPath))
		cmd.Dir = filepath.Dir(jarPath)
		err := cmd.Run()
		if err != nil {
			m.log.Error(err, "error unpacking java archive")
			return "", err
		}
	}
	return javaFileAbsolutePath, nil
}

func (m *mavenBuildTool) GetDependencies(ctx context.Context) (map[uri.URI][]provider.DepDAGItem, error) {
	m.log.V(3).Info("getting deps")
	ok, err := m.depCache.useCache()
	if err != nil {
		return nil, err
	}
	if ok {
		ll := m.depCache.getCachedDeps()
		return ll, nil
	}
	ll, err := m.getDependenciesForMaven(ctx)
	m.depCache.setCachedDeps(ll, err)
	if err != nil {
		return nil, err
	}
	return ll, nil
}

func (m *mavenBuildTool) GetResolver(decompileTool string) (dependency.Resolver, error) {
	opts := dependency.ResolverOptions{
		Log:           m.log,
		Location:      filepath.Dir(m.depCache.hashFile),
		BuildFile:     m.mvnSettingsFile,
		LocalRepo:     m.mvnLocalRepo,
		Insecure:      m.mvnInsecure,
		DecompileTool: decompileTool,
		Labeler:       m.labeler,
	}
	return dependency.GetMavenResolver(opts), nil
}

func (m *mavenBuildTool) getDependenciesForMaven(ctx context.Context) (map[uri.URI][]provider.DepDAGItem, error) {
	file := uri.File(m.hashFile)

	moddir := filepath.Dir(m.hashFile)

	args := []string{
		"-B",
		"dependency:tree",
		"-Djava.net.useSystemProxies=true",
	}

	if m.mvnSettingsFile != "" {
		args = append(args, "-s", m.mvnSettingsFile)
	}

	if m.mvnInsecure {
		args = append(args, "-Dmaven.wagon.http.ssl.insecure=true")
	}

	// get the graph output
	timeout, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(timeout, "mvn", args...)
	cmd.Dir = moddir
	mvnOutput, err := cmd.CombinedOutput()
	m.log.V(8).Info("ran mvn command for dependency tree", "output", string(mvnOutput))
	if err != nil {
		return nil, fmt.Errorf("maven dependency:tree command failed with error %w, maven output: %s", err, string(mvnOutput))
	}

	lines := strings.Split(string(mvnOutput), "\n")
	submoduleTrees := m.extractSubmoduleTrees(lines)

	var pomDeps []provider.DepDAGItem
	for _, tree := range submoduleTrees {
		submoduleDeps, err := m.parseMavenDepLines(tree, m.mvnLocalRepo, m.hashFile)
		if err != nil {
			return nil, err
		}
		pomDeps = append(pomDeps, submoduleDeps...)
	}

	deps := map[uri.URI][]provider.DepDAGItem{}
	deps[file] = pomDeps

	return deps, nil
}

// extractSubmoduleTrees creates an array of lines for each submodule tree found in the mvn dependency:tree output
func (m *mavenBuildTool) extractSubmoduleTrees(lines []string) [][]string {
	submoduleTrees := [][]string{}

	beginRegex := regexp.MustCompile(`(maven-)*dependency(-plugin)*:[\d\.]+:tree`)
	endRegex := regexp.MustCompile(`\[INFO\] -*$`)

	submod := 0
	gather, skipmod := false, true
	for _, line := range lines {
		if beginRegex.Find([]byte(line)) != nil {
			gather = true
			submoduleTrees = append(submoduleTrees, []string{})
			continue
		}

		if gather {
			if endRegex.Find([]byte(line)) != nil {
				gather, skipmod = false, true
				submod++
				continue
			}
			if skipmod { // we ignore the first module (base module)
				skipmod = false
				continue
			}

			line = strings.TrimPrefix(line, "[INFO] ")
			line = strings.Trim(line, " ")

			// output contains progress report lines that are not deps, skip those
			if !(strings.HasPrefix(line, "+") || strings.HasPrefix(line, "|") || strings.HasPrefix(line, "\\")) {
				continue
			}

			submoduleTrees[submod] = append(submoduleTrees[submod], line)
		}
	}

	return submoduleTrees
}

// parseDepString parses a java dependency string
func (m *mavenBuildTool) parseDepString(dep, localRepoPath, pomPath string) (provider.Dep, error) {
	d := provider.Dep{}
	// remove all the pretty print characters.
	dep = strings.TrimFunc(dep, func(r rune) bool {
		if r == '+' || r == '-' || r == '\\' || r == '|' || r == ' ' || r == '"' || r == '\t' {
			return true
		}
		return false

	})
	// Split string on ":" must have 5 parts.
	// For now we ignore Type as it appears most everything is a jar
	parts := strings.Split(dep, ":")
	if len(parts) >= 3 {
		// Its always <groupId>:<artifactId>:<Packaging>: ... then
		if len(parts) == 6 {
			d.Classifier = parts[3]
			d.Version = parts[4]
			d.Type = parts[5]
		} else if len(parts) == 5 {
			d.Version = parts[3]
			d.Type = parts[4]
		} else {
			m.log.Info("Cannot derive version from dependency string", "dependency", dep)
			d.Version = "Unknown"
		}
	} else {
		return d, fmt.Errorf("unable to split dependency string %s", dep)
	}

	group := parts[0]
	artifact := parts[1]
	d.Name = fmt.Sprintf("%s.%s", group, artifact)

	fp := m.resolveDepFilepath(&d, group, artifact, localRepoPath)

	// if windows home path begins with C:
	if !strings.HasPrefix(fp, "/") {
		fp = "/" + fp
	}
	d.Labels = m.labeler.AddLabels(d.Name, false)
	d.FileURIPrefix = fmt.Sprintf("file://%v", filepath.Dir(fp))

	if runtime.GOOS == "windows" {
		d.FileURIPrefix = strings.ReplaceAll(d.FileURIPrefix, "\\", "/")
	}

	d.Extras = map[string]any{
		groupIdKey:    group,
		artifactIdKey: artifact,
		pomPathKey:    pomPath,
	}

	return d, nil
}

// parseMavenDepLines recursively parses output lines from maven dependency tree
func (m *mavenBuildTool) parseMavenDepLines(lines []string, localRepoPath, pomPath string) ([]provider.DepDAGItem, error) {
	if len(lines) > 0 {
		baseDepString := lines[0]
		baseDep, err := m.parseDepString(baseDepString, localRepoPath, pomPath)
		if err != nil {
			return nil, err
		}
		item := provider.DepDAGItem{}
		item.Dep = baseDep
		item.AddedDeps = []provider.DepDAGItem{}
		idx := 1
		// indirect deps are separated by 3 or more spaces after the direct dep
		for idx < len(lines) && strings.Count(lines[idx], " ") > 2 {
			transitiveDep, err := m.parseDepString(lines[idx], localRepoPath, pomPath)
			if err != nil {
				return nil, err
			}
			dm := map[string]any{
				"name":    baseDep.Name,
				"version": baseDep.Version,
				"extras":  baseDep.Extras,
			}
			transitiveDep.Indirect = true
			transitiveDep.Extras[baseDepKey] = dm // Minimum needed set of attributes for GetLocation
			item.AddedDeps = append(item.AddedDeps, provider.DepDAGItem{Dep: transitiveDep})
			idx += 1
		}
		ds, err := m.parseMavenDepLines(lines[idx:], localRepoPath, pomPath)
		if err != nil {
			return nil, err
		}
		ds = append(ds, item)
		return ds, nil
	}
	return []provider.DepDAGItem{}, nil
}

// resolveDepFilepath tries to extract a valid filepath for the dependency with either JAR or POM packaging
func (m *mavenBuildTool) resolveDepFilepath(d *provider.Dep, group string, artifact string, localRepoPath string) string {
	groupPath := strings.ReplaceAll(group, ".", "/")

	// Try pom packaging (see https://www.baeldung.com/maven-packaging-types#4-pom)
	var fp string
	if d.Classifier == "" {
		fp = filepath.Join(localRepoPath, groupPath, artifact, d.Version, fmt.Sprintf("%v-%v.%v.sha1", artifact, d.Version, "pom"))
	} else {
		fp = filepath.Join(localRepoPath, groupPath, artifact, d.Version, fmt.Sprintf("%v-%v-%v.%v.sha1", artifact, d.Version, d.Classifier, "pom"))
	}
	b, err := os.ReadFile(fp)
	if err != nil {
		// Try jar packaging
		if d.Classifier == "" {
			fp = filepath.Join(localRepoPath, groupPath, artifact, d.Version, fmt.Sprintf("%v-%v.%v.sha1", artifact, d.Version, "jar"))
		} else {
			fp = filepath.Join(localRepoPath, groupPath, artifact, d.Version, fmt.Sprintf("%v-%v-%v.%v.sha1", artifact, d.Version, d.Classifier, "jar"))
		}
		b, err = os.ReadFile(fp)
	}

	if err != nil {
		// Log the error and continue with the next dependency.
		m.log.V(5).Error(err, "error reading SHA hash file for dependency", "d", d.Name)
		// Set some default or empty resolved identifier for the dependency.
		d.ResolvedIdentifier = ""
	} else {
		// sometimes sha file contains name of the jar followed by the actual sha
		sha, _, _ := strings.Cut(string(b), " ")
		d.ResolvedIdentifier = sha
	}

	return fp
}
