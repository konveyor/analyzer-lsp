package java

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/antchfx/xmlquery"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
)

const (
	javaDepSourceInternal                      = "internal"
	javaDepSourceOpenSource                    = "open-source"
	providerSpecificConfigOpenSourceDepListKey = "depOpenSourceLabelsFile"
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
	ll, err := p.GetDependenciesDAG()
	if err != nil {
		return p.GetDependencyFallback()
	}
	if len(ll) == 0 {
		return p.GetDependencyFallback()
	}
	m := map[uri.URI][]*provider.Dep{}
	for f, ds := range ll {
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

func (p *javaServiceClient) getLocalRepoPath() string {
	cmd := exec.Command("mvn", "help:evaluate", "-Dexpression=settings.localRepository", "-q", "-DforceStdout")
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
	dep := provider.Dep{}
	// TODO this is comedically janky
	for _, node := range list {
		if node.Data == "groupId" {
			if dep.Name != "" {
				deps = append(deps, &dep)
				dep = provider.Dep{}
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
		deps = append(deps, &dep)
	}
	m := map[uri.URI][]*provider.Dep{}
	m[file] = deps
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
	// get the graph output
	cmd := exec.Command("mvn", "dependency:tree", fmt.Sprintf("-DoutputFile=%s", f.Name()))
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
		lines = lines[1 : len(lines)-2]
	}

	pomDeps, err := p.parseMavenDepLines(lines, localRepoPath)
	if err != nil {
		return nil, err
	}

	m := map[uri.URI][]provider.DepDAGItem{}
	m[file] = pomDeps

	//Walk the dir, looking for .jar files to add to the dependency
	w := walker{
		deps: m,
	}
	filepath.WalkDir(moddir, w.walkDirForJar)

	return m, nil
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
		return d, err
	}
	d.ResolvedIdentifier = string(b)
	d.Labels = p.addDepLabels(d.Name)
	d.FileURIPrefix = fmt.Sprintf("%v://contents%v", FILE_URI_PREFIX, filepath.Dir(fp))

	return d, nil
}

func (p *javaServiceClient) addDepLabels(depName string) []string {
	m := map[string]interface{}{}

	for _, d := range p.depToLabels {
		if d.r.Match([]byte(depName)) {
			for _, l := range d.labels {
				m[l] = nil
			}
		}
	}
	s := []string{}
	for k, _ := range m {
		s = append(s, k)
	}
	if len(s) == 0 {
		s = append(s, fmt.Sprintf("%v=%v", provider.DepSourceLabel, javaDepSourceInternal))
	}
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

// depInit will allow for us to check for the list of deps file,
// If found, we will load into a map, for easy lookup.
// We will need to define file structure to read in.
func (p *javaServiceClient) depInit() error {
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

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		depName := scanner.Text()
		r, err := regexp.Compile(depName)
		if err != nil {
			return fmt.Errorf("unable to create regexp for string: %v", depName)
		}
		//Make sure that we are not adding duplicates
		found := false
		for _, d := range p.depToLabels {
			if d.r.String() == depName {
				d.labels = append(d.labels, javaDepSourceOpenSource)
				found = true
			}
		}
		if !found {
			p.depToLabels = append(p.depToLabels, depLabelItem{
				r:      r,
				labels: []string{fmt.Sprintf("%v=%v", provider.DepSourceLabel, javaDepSourceOpenSource)},
			})
		}

	}
	return nil
}
