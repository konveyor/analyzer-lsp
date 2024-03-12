package java

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

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
)

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
	return f
}

func (p *javaServiceClient) GetDependencies(ctx context.Context) (map[uri.URI][]*provider.Dep, error) {
	p.depsMutex.RLock()
	val := p.depsCache
	p.depsMutex.RUnlock()
	if val != nil {
		return val, nil
	}

	var err error
	var ll map[uri.URI][]konveyor.DepDAGItem
	m := map[uri.URI][]*provider.Dep{}
	if p.isLocationBinary {
		ll = make(map[uri.URI][]konveyor.DepDAGItem, 0)
		// for binaries we only find JARs embedded in archive
		p.discoverDepsFromJars(p.config.DependencyPath, ll)
	} else {
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
	return string(outb.String())
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
		if *d.Version != "" {
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
				dep.FileURIPrefix = filepath.Join(m2Repo,
					strings.Replace(*d.GroupID, ".", "/", -1), *d.ArtifactID, dep.Version)
			}
		}
		deps = append(deps, &dep)
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

	// get the graph output
	cmd := exec.Command("mvn", args...)
	cmd.Dir = moddir
	mvnOutput, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
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
	}
	filepath.WalkDir(path, w.walkDirForJar)
}

type walker struct {
	deps        map[uri.URI][]provider.DepDAGItem
	depToLabels map[string]*depLabelItem
	m2RepoPath  string
	seen        map[string]bool
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
		artifact, _ := toDependency(context.TODO(), path)
		if (artifact != javaArtifact{}) {
			d.Name = fmt.Sprintf("%s.%s", artifact.GroupId, artifact.ArtifactId)
			d.Version = artifact.Version
			d.Labels = addDepLabels(w.depToLabels, d.Name)
			d.ResolvedIdentifier = artifact.sha1
			// when we can successfully get javaArtifact from a jar
			// we added it to the pom and it should be in m2Repo path
			if w.m2RepoPath != "" {
				d.FileURIPrefix = filepath.Join(w.m2RepoPath,
					strings.Replace(artifact.GroupId, ".", "/", -1), artifact.ArtifactId, artifact.Version)
			}
		}

		w.deps[uri.URI(filepath.Join(path, info.Name()))] = []provider.DepDAGItem{
			{
				Dep: d,
			},
		}
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

	d.Labels = addDepLabels(p.depToLabels, d.Name)
	d.FileURIPrefix = fmt.Sprintf("file://%v", filepath.Dir(fp))

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

	// Try jar packaging
	var fp string
	if d.Classifier == "" {
		fp = filepath.Join(localRepoPath, groupPath, artifact, d.Version, fmt.Sprintf("%v-%v.%v.sha1", artifact, d.Version, "jar"))
	} else {
		fp = filepath.Join(localRepoPath, groupPath, artifact, d.Version, fmt.Sprintf("%v-%v-%v.%v.sha1", artifact, d.Version, d.Classifier, "jar"))
	}
	b, err := os.ReadFile(fp)
	if err != nil {
		// Try pom packaging (see https://www.baeldung.com/maven-packaging-types#4-pom)
		if d.Classifier == "" {
			fp = filepath.Join(localRepoPath, groupPath, artifact, d.Version, fmt.Sprintf("%v-%v.%v.sha1", artifact, d.Version, "pom"))
		} else {
			fp = filepath.Join(localRepoPath, groupPath, artifact, d.Version, fmt.Sprintf("%v-%v-%v.%v.sha1", artifact, d.Version, d.Classifier, "pom"))
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
			transitiveDep.Indirect = true
			transitiveDep.Extras["baseDep"] = baseDep
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
