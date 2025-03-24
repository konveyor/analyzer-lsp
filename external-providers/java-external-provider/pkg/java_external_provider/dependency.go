package java

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/engine/labels"
	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/vifraa/gopom"
	"go.lsp.dev/uri"
)

const (
	javaDepSourceInternal                      = "internal"
	javaDepSourceOpenSource                    = "open-source"
	providerSpecificConfigOpenSourceDepListKey = "depOpenSourceLabelsFile"
	providerSpecificConfigExcludePackagesKey   = "excludePackages"
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

func (p *javaServiceClient) GetBuildTool() string {
	bf := ""
	if bf = p.findPom(); bf != "" {
		return maven
	} else if bf = p.findGradleBuild(); bf != "" {
		return gradle
	}
	return ""
}

// TODO implement this for real
func (p *javaServiceClient) findPom() string {
	var depPath string
	if p.config.DependencyPath == "" {
		depPath = "pom.xml"
	} else {
		depPath = p.config.DependencyPath
	}
	if filepath.IsAbs(depPath) {
		return depPath
	}
	f, err := filepath.Abs(filepath.Join(p.config.Location, depPath))
	if err != nil {
		return ""
	}
	if _, err := os.Stat(f); errors.Is(err, os.ErrNotExist) {
		return ""
	}
	return f
}

func (p *javaServiceClient) findGradleBuild() string {
	if p.config.Location != "" {
		path := filepath.Join(p.config.Location, "build.gradle")
		_, err := os.Stat(path)
		if err != nil {
			return ""
		}
		f, err := filepath.Abs(path)
		if err != nil {
			return ""
		}
		return f
	}
	return ""
}

func (p *javaServiceClient) GetDependencies(ctx context.Context) (map[uri.URI][]*provider.Dep, error) {
	p.log.V(4).Info("running dependency analysis")

	// TODO: shawn-hurley does not appear that this is returning early if there is a cache.
	// We should add these to a cache with the hash of the gradle build file.
	if p.GetBuildTool() == gradle {
		p.log.V(2).Info("gradle found - retrieving dependencies")
		m := map[uri.URI][]*provider.Dep{}
		deps, err := p.getDependenciesForGradle(ctx)
		for f, ds := range deps {
			deps := []*provider.Dep{}
			for _, dep := range ds {
				d := dep.Dep
				deps = append(deps, &d)
				deps = append(deps, provider.ConvertDagItemsToList(dep.AddedDeps)...)
			}
			m[f] = deps
		}
		return m, err
	}

	var err error
	var ll map[uri.URI][]konveyor.DepDAGItem
	m := map[uri.URI][]*provider.Dep{}
	if p.isLocationBinary {
		p.depsMutex.RLock()
		val := p.depsCache
		p.depsMutex.RUnlock()
		if val != nil {
			return val, nil
		}
		ll = make(map[uri.URI][]konveyor.DepDAGItem, 0)
		// for binaries we only find JARs embedded in archive
		p.discoverDepsFromJars(p.config.DependencyPath, ll)
		if len(ll) == 0 {
			p.log.Info("unable to get dependencies from jars, looking for pom")
			pomPaths := p.discoverPoms(p.config.DependencyPath, ll)
			for _, path := range pomPaths {
				dep, err := p.GetDependenciesFallback(ctx, path)
				if err != nil {
					return m, err
				}
				maps.Copy(m, dep)
			}
			return m, nil
		}
	} else {
		// Read pom and create a hash.
		// if pom hash and depCache return cache
		hash := sha256.New()
		var file *os.File
		file, err = os.Open(p.findPom())
		if err != nil {
			p.log.Error(err, "unable to open the pom file", "pom path", file)
			return nil, err
		}
		if _, err = io.Copy(hash, file); err != nil {
			file.Close()
			p.log.Error(err, "unable to copy file to hash", "pom path", file)
			return nil, err
		}
		file.Close()
		hashString := string(hash.Sum(nil))
		if p.depsFileHash != nil && *p.depsFileHash == hashString && p.depsCache != nil {
			p.depsMutex.RLock()
			val := p.depsCache
			p.depsMutex.RUnlock()
			if val != nil {
				return val, nil
			}
		}
		p.depsFileHash = &hashString
		ll, err = p.GetDependenciesDAG(ctx)
		if err != nil {
			p.log.Info("unable to get dependencies, using fallback", "error", err)
			return p.GetDependenciesFallback(ctx, "")
		}
		if len(ll) == 0 {
			p.log.Info("unable to get dependencies (none found), using fallback")
			return p.GetDependenciesFallback(ctx, "")
		}
	}
	for f, ds := range ll {
		deps := []*provider.Dep{}
		for _, dep := range ds {
			d := dep.Dep
			deps = append(deps, &d)
			deps = append(deps, provider.ConvertDagItemsToList(dep.AddedDeps)...)
		}
		m[f] = deps
	}
	p.depsMutex.Lock()
	p.depsCache = m
	p.depsMutex.Unlock()
	return m, nil
}

func getMavenLocalRepoPath(mvnSettingsFile string) string {
	args := []string{
		"help:evaluate", "-Dexpression=settings.localRepository", "-q", "-DforceStdout",
	}
	if mvnSettingsFile != "" {
		args = append(args, "-s", mvnSettingsFile)
	}
	cmd := exec.Command("mvn", args...)
	var outb bytes.Buffer
	cmd.Stdout = &outb
	err := cmd.Run()
	if err != nil {
		return ""
	}

	// check errors
	return outb.String()
}

func (p *javaServiceClient) GetDependenciesFallback(ctx context.Context, location string) (map[uri.URI][]*provider.Dep, error) {
	deps := []*provider.Dep{}

	m2Repo := getMavenLocalRepoPath(p.mvnSettingsFile)

	path, err := filepath.Abs(p.findPom())
	if err != nil {
		return nil, err
	}

	if location != "" {
		path = location
	}
	pom, err := gopom.Parse(path)
	if err != nil {
		p.log.Error(err, "Analyzing POM")
		return nil, err
	}
	p.log.V(10).Info("Analyzing POM",
		"POM", fmt.Sprintf("%s:%s:%s", pomCoordinate(pom.GroupID), pomCoordinate(pom.ArtifactID), pomCoordinate(pom.Version)),
		"error", err)

	// If the pom object is empty then parse failed silently.
	if reflect.DeepEqual(*pom, gopom.Project{}) {
		return nil, nil
	}

	// have to get both <dependencies> and <dependencyManagement> dependencies (if present)
	var pomDeps []gopom.Dependency
	if pom.Dependencies != nil && *pom.Dependencies != nil {
		pomDeps = append(pomDeps, *pom.Dependencies...)
	}
	if pom.DependencyManagement != nil {
		if pom.DependencyManagement.Dependencies != nil {
			pomDeps = append(pomDeps, *pom.DependencyManagement.Dependencies...)
		}
	}

	// add each dependency found
	for _, d := range pomDeps {
		if d.GroupID == nil || d.Version == nil || d.ArtifactID == nil {
			continue
		}
		dep := provider.Dep{}
		dep.Name = fmt.Sprintf("%s.%s", *d.GroupID, *d.ArtifactID)
		dep.Extras = map[string]interface{}{
			groupIdKey:    *d.GroupID,
			artifactIdKey: *d.ArtifactID,
			pomPathKey:    path,
		}
		if d.Version != nil {
			if strings.Contains(*d.Version, "$") {
				version := strings.TrimSuffix(strings.TrimPrefix(*d.Version, "${"), "}")
				p.log.V(10).Info("Searching for property in properties",
					"property", version,
					"properties", pom.Properties)
				if pom.Properties == nil {
					p.log.Info("Cannot resolve version property value as POM does not have properties",
						"POM", fmt.Sprintf("%s.%s", pomCoordinate(pom.GroupID), pomCoordinate(pom.ArtifactID)),
						"property", version,
						"dependency", dep.Name)
					dep.Version = version
				} else {
					version = pom.Properties.Entries[version]
					if version != "" {
						dep.Version = version
					}
				}
			} else {
				dep.Version = *d.Version
			}
			if m2Repo != "" && d.ArtifactID != nil && d.GroupID != nil {
				dep.FileURIPrefix = fmt.Sprintf("file://%s", filepath.Join(m2Repo,
					strings.Replace(*d.GroupID, ".", "/", -1), *d.ArtifactID, dep.Version))
			}
		}
		deps = append(deps, &dep)
	}
	if len(deps) == 0 {
		p.log.V(1).Info("unable to get dependencies from pom.xml in fallback", "pom", path)
		return nil, nil
	}

	m := map[uri.URI][]*provider.Dep{}
	m[uri.File(path)] = deps
	p.depsMutex.Lock()
	p.depsCache = m
	p.depsMutex.Unlock()

	// recursively find deps in submodules
	if pom.Modules != nil {
		for _, mod := range *pom.Modules {
			mPath := fmt.Sprintf("%s/%s/pom.xml", filepath.Dir(path), mod)
			moreDeps, err := p.GetDependenciesFallback(ctx, mPath)
			if err != nil {
				return nil, err
			}

			// add found dependencies to map
			for depPath := range moreDeps {
				m[depPath] = moreDeps[depPath]
			}
		}
	}

	return m, nil
}

func pomCoordinate(value *string) string {
	if value != nil {
		return *value
	}
	return "unknown"
}

func (p *javaServiceClient) GetDependenciesDAG(ctx context.Context) (map[uri.URI][]provider.DepDAGItem, error) {
	switch p.GetBuildTool() {
	case maven:
		return p.getDependenciesForMaven(ctx)
	case gradle:
		return p.getDependenciesForGradle(ctx)
	default:
		return nil, fmt.Errorf("no build tool found")
	}
}

func (p *javaServiceClient) getDependenciesForMaven(_ context.Context) (map[uri.URI][]provider.DepDAGItem, error) {
	localRepoPath := getMavenLocalRepoPath(p.mvnSettingsFile)

	path := p.findPom()
	file := uri.File(path)

	moddir := filepath.Dir(path)

	args := []string{
		"-B",
		"dependency:tree",
		"-Djava.net.useSystemProxies=true",
	}

	if p.mvnSettingsFile != "" {
		args = append(args, "-s", p.mvnSettingsFile)
	}

	if p.mvnInsecure {
		args = append(args, "-Dmaven.wagon.http.ssl.insecure=true")
	}

	// get the graph output
	cmd := exec.Command("mvn", args...)
	cmd.Dir = moddir
	mvnOutput, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("maven dependency:tree command failed with error %w, maven output: %s", err, string(mvnOutput))
	}

	lines := strings.Split(string(mvnOutput), "\n")
	submoduleTrees := extractSubmoduleTrees(lines)

	var pomDeps []provider.DepDAGItem
	for _, tree := range submoduleTrees {
		submoduleDeps, err := p.parseMavenDepLines(tree, localRepoPath, path)
		if err != nil {
			return nil, err
		}
		pomDeps = append(pomDeps, submoduleDeps...)
	}

	m := map[uri.URI][]provider.DepDAGItem{}
	m[file] = pomDeps

	if len(m) == 0 {
		// grab the embedded deps
		p.discoverDepsFromJars(moddir, m)
	}

	return m, nil
}

// getDependenciesForGradle invokes the Gradle wrapper to get the dependency tree and returns all project dependencies
// TODO: what if no wrapper?
func (p *javaServiceClient) getDependenciesForGradle(_ context.Context) (map[uri.URI][]provider.DepDAGItem, error) {
	subprojects, err := p.getGradleSubprojects()
	if err != nil {
		return nil, err
	}

	// command syntax: ./gradlew subproject1:dependencies subproject2:dependencies ...
	args := []string{}
	if len(subprojects) > 0 {
		for _, sp := range subprojects {
			args = append(args, fmt.Sprintf("%s:dependencies", sp))
		}
	} else {
		args = append(args, "dependencies")
	}

	// get the graph output
	exe, err := filepath.Abs(filepath.Join(p.config.Location, "gradlew"))
	if err != nil {
		return nil, fmt.Errorf("error calculating gradle wrapper path")
	}
	if _, err = os.Stat(exe); errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("a gradle wrapper must be present in the project")
	}
	cmd := exec.Command(exe, args...)
	cmd.Dir = p.config.Location
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(output), "\n")
	deps := p.parseGradleDependencyOutput(lines)

	// TODO: do we need to separate by submodule somehow?

	path := p.findGradleBuild()
	file := uri.File(path)
	m := map[uri.URI][]provider.DepDAGItem{}
	m[file] = deps

	// TODO: need error?
	return m, nil
}

func (p *javaServiceClient) getGradleSubprojects() ([]string, error) {
	args := []string{
		"projects",
	}

	// Ideally we'd want to set this in gradle.properties, or as a -Dorg.gradle.java.home arg,
	// but it doesn't seem to work in older Gradle versions. This should only affect child processes in any case.
	err := os.Setenv("JAVA_HOME", os.Getenv("JAVA8_HOME"))
	if err != nil {
		return nil, err
	}

	exe, err := filepath.Abs(filepath.Join(p.config.Location, "gradlew"))
	if err != nil {
		return nil, fmt.Errorf("error calculating gradle wrapper path")
	}
	if _, err = os.Stat(exe); errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("a gradle wrapper must be present in the project")
	}
	cmd := exec.Command(exe, args...)
	cmd.Dir = p.config.Location
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	beginRegex := regexp.MustCompile(`Root project`)
	endRegex := regexp.MustCompile(`To see a list of`)
	npRegex := regexp.MustCompile(`No sub-projects`)
	pRegex := regexp.MustCompile(`.*- Project '(.*)'`)

	subprojects := []string{}

	gather := false
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if npRegex.Find([]byte(line)) != nil {
			return []string{}, nil
		}
		if beginRegex.Find([]byte(line)) != nil {
			gather = true
			continue
		}
		if gather {
			if endRegex.Find([]byte(line)) != nil {
				return subprojects, nil
			}

			if p := pRegex.FindStringSubmatch(line); p != nil {
				subprojects = append(subprojects, p[1])
			}
		}
	}

	return subprojects, fmt.Errorf("error parsing gradle dependency output")
}

// parseGradleDependencyOutput converts the relevant lines from the dependency output into actual dependencies
// See https://regex101.com/r/9Gp7dW/1 for context
func (p *javaServiceClient) parseGradleDependencyOutput(lines []string) []provider.DepDAGItem {
	deps := []provider.DepDAGItem{}

	treeDepRegex := regexp.MustCompile(`^([| ]+)?[+\\]--- (.*)`)

	// map of <anidation level> to <pointer to last found dependency for given level>
	// this is so that children can be added to their respective parents
	lastFoundWithDepth := make(map[int]*provider.DepDAGItem)

	for _, line := range lines {
		match := treeDepRegex.FindStringSubmatch(line)
		if match != nil {
			dep := parseGradleDependencyString(match[2])
			if reflect.DeepEqual(dep, provider.DepDAGItem{}) { // ignore empty dependency
				continue
			} else if match[1] != "" { // transitive dependency
				dep.Dep.Indirect = true
				depth := len(match[1]) / 5                                             // get the level of anidation of the dependency within the tree
				parent := lastFoundWithDepth[depth-1]                                  // find its parent
				parent.AddedDeps = append(parent.AddedDeps, dep)                       // add child to parent
				lastFoundWithDepth[depth] = &parent.AddedDeps[len(parent.AddedDeps)-1] // update last found with given depth
			} else { // root level (direct) dependency
				deps = append(deps, dep) // add root dependency to result list
				lastFoundWithDepth[0] = &deps[len(deps)-1]
				continue
			}
		}
	}

	return deps
}

// parseGradleDependencyString parses the lines of the gradle dependency output, for instance:
// org.codehaus.groovy:groovy:3.0.21
// org.codehaus.groovy:groovy:3.+ -> 3.0.21
// com.codevineyard:hello-world:{strictly 1.0.1} -> 1.0.1
// :simple-jar (n)
func parseGradleDependencyString(s string) provider.DepDAGItem {
	// (*) - dependencies omitted (listed previously)
	// (n) - Not resolved (configuration is not meant to be resolved)
	if strings.HasSuffix(s, "(n)") || strings.HasSuffix(s, "(*)") {
		return provider.DepDAGItem{}
	}

	depRegex := regexp.MustCompile(`(.+):(.+):((.*) -> )?(.*)`)
	libRegex := regexp.MustCompile(`:(.*)`)

	dep := provider.Dep{}
	match := depRegex.FindStringSubmatch(s)
	if match != nil {
		dep.Name = match[1] + "." + match[2]
		dep.Version = match[5]
	} else if match = libRegex.FindStringSubmatch(s); match != nil {
		dep.Name = match[1]
	}

	return provider.DepDAGItem{Dep: dep, AddedDeps: []provider.DepDAGItem{}}
}

// extractSubmoduleTrees creates an array of lines for each submodule tree found in the mvn dependency:tree output
func extractSubmoduleTrees(lines []string) [][]string {
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

// discoverDepsFromJars walks given path to discover dependencies embedded as JARs
func (p *javaServiceClient) discoverDepsFromJars(path string, ll map[uri.URI][]konveyor.DepDAGItem) {
	// for binaries we only find JARs embedded in archive
	w := walker{
		deps:        ll,
		depToLabels: p.depToLabels,
		m2RepoPath:  getMavenLocalRepoPath(p.mvnSettingsFile),
		seen:        map[string]bool{},
		initialPath: path,
		log:         p.log,
	}
	filepath.WalkDir(path, w.walkDirForJar)
}

type walker struct {
	deps        map[uri.URI][]provider.DepDAGItem
	depToLabels map[string]*depLabelItem
	m2RepoPath  string
	initialPath string
	seen        map[string]bool
	pomPaths    []string
	log         logr.Logger
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
		artifact, _ := toDependency(context.TODO(), path, w.log)
		if (artifact != javaArtifact{}) {
			d.Name = fmt.Sprintf("%s.%s", artifact.GroupId, artifact.ArtifactId)
			d.Version = artifact.Version
			d.Labels = addDepLabels(w.depToLabels, d.Name)
			d.ResolvedIdentifier = artifact.sha1
			// when we can successfully get javaArtifact from a jar
			// we added it to the pom and it should be in m2Repo path
			if w.m2RepoPath != "" {
				d.FileURIPrefix = fmt.Sprintf("file://%s", filepath.Join(w.m2RepoPath,
					strings.Replace(artifact.GroupId, ".", "/", -1), artifact.ArtifactId, artifact.Version))
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
		artifact, _ := toFilePathDependency(context.Background(), filepath.Join(relPath, info.Name()))
		if (artifact != javaArtifact{}) {
			d.Name = fmt.Sprintf("%s.%s", artifact.GroupId, artifact.ArtifactId)
			d.Version = artifact.Version
			d.Labels = addDepLabels(w.depToLabels, d.Name)
			d.ResolvedIdentifier = artifact.sha1
			// when we can successfully get javaArtifact from a jar
			// we added it to the pom and it should be in m2Repo path
			d.FileURIPrefix = fmt.Sprintf("file://%s", filepath.Join("java-project", "src", "main",
				strings.Replace(artifact.GroupId, ".", "/", -1), artifact.ArtifactId))
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

func (p *javaServiceClient) discoverPoms(pathStart string, ll map[uri.URI][]konveyor.DepDAGItem) []string {
	w := walker{
		deps:        ll,
		depToLabels: p.depToLabels,
		m2RepoPath:  "",
		seen:        map[string]bool{},
		initialPath: pathStart,
		pomPaths:    []string{},
	}
	filepath.WalkDir(pathStart, w.walkDirForPom)
	return w.pomPaths
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

// parseDepString parses a java dependency string
func (p *javaServiceClient) parseDepString(dep, localRepoPath, pomPath string) (provider.Dep, error) {
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
			p.log.Info("Cannot derive version from dependency string", "dependency", dep)
			d.Version = "Unknown"
		}
	} else {
		return d, fmt.Errorf("unable to split dependency string %s", dep)
	}

	group := parts[0]
	artifact := parts[1]
	d.Name = fmt.Sprintf("%s.%s", group, artifact)

	fp := resolveDepFilepath(&d, p, group, artifact, localRepoPath)

	// if windows home path begins with C:
	if !strings.HasPrefix(fp, "/") {
		fp = "/" + fp
	}
	d.Labels = addDepLabels(p.depToLabels, d.Name)
	d.FileURIPrefix = fmt.Sprintf("file://%v", filepath.Dir(fp))

	if runtime.GOOS == "windows" {
		d.FileURIPrefix = strings.ReplaceAll(d.FileURIPrefix, "\\", "/")
	}

	d.Extras = map[string]interface{}{
		groupIdKey:    group,
		artifactIdKey: artifact,
		pomPathKey:    pomPath,
	}

	return d, nil
}

// resolveDepFilepath tries to extract a valid filepath for the dependency with either JAR or POM packaging
func resolveDepFilepath(d *provider.Dep, p *javaServiceClient, group string, artifact string, localRepoPath string) string {
	groupPath := strings.Replace(group, ".", "/", -1)

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
		p.log.V(5).Error(err, "error reading SHA hash file for dependency", "d", d.Name)
		// Set some default or empty resolved identifier for the dependency.
		d.ResolvedIdentifier = ""
	} else {
		// sometimes sha file contains name of the jar followed by the actual sha
		sha, _, _ := strings.Cut(string(b), " ")
		d.ResolvedIdentifier = sha
	}

	return fp
}

func addDepLabels(depToLabels map[string]*depLabelItem, depName string) []string {
	m := map[string]interface{}{}
	for _, d := range depToLabels {
		if d.r.Match([]byte(depName)) {
			for label := range d.labels {
				m[label] = nil
			}
		}
	}
	s := []string{}
	for k := range m {
		s = append(s, k)
	}
	// if open source label is not found, qualify the dep as being internal by default
	if _, openSourceLabelFound :=
		m[labels.AsString(provider.DepSourceLabel, javaDepSourceOpenSource)]; !openSourceLabelFound {
		s = append(s,
			labels.AsString(provider.DepSourceLabel, javaDepSourceInternal))
	}
	s = append(s, labels.AsString(provider.DepLanguageLabel, "java"))
	return s
}

// parseMavenDepLines recursively parses output lines from maven dependency tree
func (p *javaServiceClient) parseMavenDepLines(lines []string, localRepoPath, pomPath string) ([]provider.DepDAGItem, error) {
	if len(lines) > 0 {
		baseDepString := lines[0]
		baseDep, err := p.parseDepString(baseDepString, localRepoPath, pomPath)
		if err != nil {
			return nil, err
		}
		item := provider.DepDAGItem{}
		item.Dep = baseDep
		item.AddedDeps = []provider.DepDAGItem{}
		idx := 1
		// indirect deps are separated by 3 or more spaces after the direct dep
		for idx < len(lines) && strings.Count(lines[idx], " ") > 2 {
			transitiveDep, err := p.parseDepString(lines[idx], localRepoPath, pomPath)
			if err != nil {
				return nil, err
			}
			dm := map[string]interface{}{
				"name":    baseDep.Name,
				"version": baseDep.Version,
				"extras":  baseDep.Extras,
			}
			transitiveDep.Indirect = true
			transitiveDep.Extras[baseDepKey] = dm // Minimum needed set of attributes for GetLocation
			item.AddedDeps = append(item.AddedDeps, provider.DepDAGItem{Dep: transitiveDep})
			idx += 1
		}
		ds, err := p.parseMavenDepLines(lines[idx:], localRepoPath, pomPath)
		if err != nil {
			return nil, err
		}
		ds = append(ds, item)
		return ds, nil
	}
	return []provider.DepDAGItem{}, nil
}

// depInit loads a map of package patterns and their associated labels for easy lookup
func (p *javaServiceClient) depInit() error {
	err := p.initOpenSourceDepLabels()
	if err != nil {
		p.log.V(5).Error(err, "failed to initialize dep labels lookup for open source packages")
		return err
	}

	err = p.initExcludeDepLabels()
	if err != nil {
		p.log.V(5).Error(err, "failed to initialize dep labels lookup for excluded packages")
		return err
	}

	return nil
}

// initOpenSourceDepLabels reads user provided file that has a list of open source
// packages (supports regex) and loads a map of patterns -> labels for easy lookup
func (p *javaServiceClient) initOpenSourceDepLabels() error {
	var ok bool
	var v interface{}
	if v, ok = p.config.ProviderSpecificConfig[providerSpecificConfigOpenSourceDepListKey]; !ok {
		p.log.V(7).Info("Did not find open source dep list.")
		return nil
	}

	var filePath string
	if filePath, ok = v.(string); !ok {
		return fmt.Errorf("unable to determine filePath from open source dep list")
	}

	fileInfo, err := os.Stat(filePath)
	if err != nil {
		//TODO(shawn-hurley): consider wrapping error with value
		return err
	}

	if fileInfo.IsDir() {
		return fmt.Errorf("open source dep list must be a file, not a directory")
	}

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	return loadDepLabelItems(file, p.depToLabels,
		labels.AsString(provider.DepSourceLabel, javaDepSourceOpenSource))
}

// initExcludeDepLabels reads user provided list of excluded packages
// and initiates label lookup for them
func (p *javaServiceClient) initExcludeDepLabels() error {
	var ok bool
	var v interface{}
	if v, ok = p.config.ProviderSpecificConfig[providerSpecificConfigExcludePackagesKey]; !ok {
		p.log.V(7).Info("did not find exclude packages list")
		return nil
	}
	var excludePackages []string
	if excludePackages, ok = v.([]string); !ok {
		return fmt.Errorf("%s config must be a list of packages to exclude", providerSpecificConfigExcludePackagesKey)
	}
	return loadDepLabelItems(strings.NewReader(
		strings.Join(excludePackages, "\n")), p.depToLabels, provider.DepExcludeLabel)
}

// loadDepLabelItems reads list of patterns from reader and appends given
// label to the list of labels for the associated pattern
func loadDepLabelItems(r io.Reader, depToLabels map[string]*depLabelItem, label string) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		pattern := scanner.Text()
		r, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("unable to create regexp for string: %v", pattern)
		}
		//Make sure that we are not adding duplicates
		if _, found := depToLabels[pattern]; !found {
			depToLabels[pattern] = &depLabelItem{
				r: r,
				labels: map[string]interface{}{
					label: nil,
				},
			}
		} else {
			if depToLabels[pattern].labels == nil {
				depToLabels[pattern].labels = map[string]interface{}{}
			}
			depToLabels[pattern].labels[label] = nil
		}
	}
	return nil
}
