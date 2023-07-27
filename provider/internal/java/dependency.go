package java

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/antchfx/xmlquery"
	"github.com/konveyor/analyzer-lsp/engine/labels"
	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
)

const (
	javaDepSourceInternal                      = "internal"
	javaDepSourceOpenSource                    = "open-source"
	providerSpecificConfigOpenSourceDepListKey = "depOpenSourceLabelsFile"
	providerSpecificConfigExcludePackagesKey   = "excludePackages"
)

// TODO implement this for real
func (p *javaServiceClient) findPom() string {
	var depPath string
	if p.config.DependencyPath == "" {
		depPath = "pom.xml"
	} else {
		depPath = p.config.DependencyPath
	}
	f, err := filepath.Abs(filepath.Join(p.config.Location, depPath))
	if err != nil {
		return ""
	}
	return f
}

func (p *javaServiceClient) GetDependencies() (map[uri.URI][]*provider.Dep, error) {
	if p.depsCache != nil {
		return p.depsCache, nil
	}
	var err error
	var ll map[uri.URI][]konveyor.DepDAGItem
	m := map[uri.URI][]*provider.Dep{}
	if p.isLocationBinary {
		ll = make(map[uri.URI][]konveyor.DepDAGItem, 0)
		// for binaries we only find JARs embedded in archive
		p.discoverDepsFromJars(p.config.DependencyPath, ll)
	} else {
		ll, err = p.GetDependenciesDAG()
		if err != nil {
			return p.GetDependencyFallback()
		}
		if len(ll) == 0 {
			return p.GetDependencyFallback()
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
	p.depsCache = m
	return m, nil
}

func (p *javaServiceClient) getLocalRepoPath() string {
	args := []string{
		"help:evaluate", "-Dexpression=settings.localRepository", "-q", "-DforceStdout",
	}
	if p.mvnSettingsFile != "" {
		args = append(args, "-s", p.mvnSettingsFile)
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

func (p *javaServiceClient) GetDependencyFallback() (map[uri.URI][]*provider.Dep, error) {
	pomDependencyQuery := "//dependencies/dependency/*"
	path := p.findPom()
	file := uri.File(path)

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(absPath)
	if err != nil {
		return nil, err
	}
	doc, err := xmlquery.Parse(f)
	if err != nil {
		return nil, err
	}
	list, err := xmlquery.QueryAll(doc, pomDependencyQuery)
	if err != nil {
		return nil, err
	}
	deps := []*provider.Dep{}
	dep := &provider.Dep{}
	// TODO this is comedically janky
	for _, node := range list {
		if node.Data == "groupId" {
			if dep.Name != "" {
				deps = append(deps, dep)
				dep = &provider.Dep{}
			}
			dep.Name = node.InnerText()
		} else if node.Data == "artifactId" {
			dep.Name += "." + node.InnerText()
		} else if node.Data == "version" {
			dep.Version = node.InnerText()
		}
		// Ignore the others
	}
	if !reflect.DeepEqual(dep, provider.Dep{}) {
		dep.Labels = []string{fmt.Sprintf("%v=%v", provider.DepSourceLabel, javaDepSourceInternal)}
		deps = append(deps, dep)
	}
	m := map[uri.URI][]*provider.Dep{}
	m[file] = deps
	p.depsCache = m
	return m, nil
}

func (p *javaServiceClient) GetDependenciesDAG() (map[uri.URI][]provider.DepDAGItem, error) {
	localRepoPath := p.getLocalRepoPath()

	path := p.findPom()
	file := uri.File(path)

	//Create temp file to use
	f, err := os.CreateTemp("", "*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(f.Name())

	moddir := filepath.Dir(path)
	args := []string{
		"dependency:tree",
		"-Djava.net.useSystemProxies=true",
		fmt.Sprintf("-DoutputFile=%s", f.Name()),
	}
	if p.mvnSettingsFile != "" {
		args = append(args, "-s", p.mvnSettingsFile)
	}
	// get the graph output
	cmd := exec.Command("mvn", args...)
	cmd.Dir = moddir
	err = cmd.Run()
	if err != nil {
		return nil, err
	}

	b, err := os.ReadFile(f.Name())
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(b), "\n")

	// strip first and last line of the output
	// first line is the base package, last line empty
	if len(lines) > 2 {
		lines = lines[1 : len(lines)-1]
	}

	pomDeps, err := p.parseMavenDepLines(lines, localRepoPath)
	if err != nil {
		return nil, err
	}

	m := map[uri.URI][]provider.DepDAGItem{}
	m[file] = pomDeps

	// also grab the embedded deps
	p.discoverDepsFromJars(moddir, m)

	return m, nil
}

// discoverDepsFromJars walks given path to discover dependencies embedded as JARs
func (p *javaServiceClient) discoverDepsFromJars(path string, ll map[uri.URI][]konveyor.DepDAGItem) {
	// for binaries we only find JARs embedded in archive
	w := walker{
		deps: ll,
	}
	filepath.WalkDir(path, w.walkDirForJar)
}

type walker struct {
	deps map[uri.URI][]provider.DepDAGItem
}

func (w *walker) walkDirForJar(path string, info fs.DirEntry, err error) error {
	if info == nil {
		return nil
	}
	if info.IsDir() {
		return filepath.WalkDir(filepath.Join(path, info.Name()), w.walkDirForJar)
	}
	if strings.HasSuffix(info.Name(), ".jar") {
		d := provider.Dep{
			Name: info.Name(),
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
// assumes format <group>:<name>:<type>:<version>:<scope>
func (p *javaServiceClient) parseDepString(dep, localRepoPath string) (provider.Dep, error) {
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
	if len(parts) != 5 {
		return d, fmt.Errorf("unable to split dependency string %s", dep)
	}
	d.Name = fmt.Sprintf("%s.%s", parts[0], parts[1])
	d.Version = parts[3]
	d.Type = parts[4]

	fp := filepath.Join(localRepoPath, strings.Replace(parts[0], ".", "/", -1), parts[1], d.Version, fmt.Sprintf("%v-%v.jar.sha1", parts[1], d.Version))
	b, err := os.ReadFile(fp)
	if err != nil {
		// Log the error and continue with the next dependency.
		p.log.V(5).Error(err, "error reading SHA hash file for dependency", "dep", d.Name)
		// Set some default or empty resolved identifier for the dependency.
		d.ResolvedIdentifier = ""
	} else {
		d.ResolvedIdentifier = string(b)
	}

	d.Labels = p.addDepLabels(d.Name)
	d.FileURIPrefix = fmt.Sprintf("%v://contents%v", FILE_URI_PREFIX, filepath.Dir(fp))

	return d, nil
}

func (p *javaServiceClient) addDepLabels(depName string) []string {
	m := map[string]interface{}{}
	for _, d := range p.depToLabels {
		if d.r.Match([]byte(depName)) {
			for label, _ := range d.labels {
				m[label] = nil
			}
		}
	}
	s := []string{}
	for k, _ := range m {
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
func (p *javaServiceClient) parseMavenDepLines(lines []string, localRepoPath string) ([]provider.DepDAGItem, error) {
	if len(lines) > 0 {
		baseDepString := lines[0]
		baseDep, err := p.parseDepString(baseDepString, localRepoPath)
		if err != nil {
			return nil, err
		}
		item := provider.DepDAGItem{}
		item.Dep = baseDep
		item.AddedDeps = []provider.DepDAGItem{}
		idx := 1
		// indirect deps are separated by 3 or more spaces after the direct dep
		for idx < len(lines) && strings.Count(lines[idx], " ") > 2 {
			transitiveDep, err := p.parseDepString(lines[idx], localRepoPath)
			if err != nil {
				return nil, err
			}
			transitiveDep.Indirect = true
			item.AddedDeps = append(item.AddedDeps, provider.DepDAGItem{Dep: transitiveDep})
			idx += 1
		}
		ds, err := p.parseMavenDepLines(lines[idx:], localRepoPath)
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
