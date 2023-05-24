package java

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/antchfx/xmlquery"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
)

// TODO implement this for real
func (p *javaProvider) findPom() string {
	var depPath string
	if p.config.InitConfig.DependencyPath == "" {
		depPath = "pom.xml"
	} else {
		depPath = p.config.InitConfig.DependencyPath
	}
	f, err := filepath.Abs(filepath.Join(p.config.InitConfig.Location, depPath))
	if err != nil {
		return ""
	}
	return f
}

func (p *javaProvider) GetDependencies() ([]provider.Dep, uri.URI, error) {
	ll, file, err := p.GetDependenciesDAG()
	if err != nil {
		return p.GetDependencyFallback()
	}
	if len(ll) == 0 {
		return p.GetDependencyFallback()
	}
	deps := []provider.Dep{}
	for _, transitives := range ll {
		deps = append(deps, transitives.Dep)
		deps = append(deps, provider.ConvertDagItemsToList(transitives.AddedDeps)...)
	}
	return deps, file, err
}

func (p *javaProvider) getLocalRepoPath() string {
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

func (p *javaProvider) GetDependencyFallback() ([]provider.Dep, uri.URI, error) {
	pomDependencyQuery := "//dependencies/dependency/*"
	path := p.findPom()
	file := uri.File(path)

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, file, err
	}

	f, err := os.Open(absPath)
	if err != nil {
		return nil, file, err
	}
	doc, err := xmlquery.Parse(f)
	if err != nil {
		return nil, file, err
	}
	list, err := xmlquery.QueryAll(doc, pomDependencyQuery)
	if err != nil {
		return nil, file, err
	}
	deps := []provider.Dep{}
	dep := provider.Dep{}
	// TODO this is comedically janky
	for _, node := range list {
		if node.Data == "groupId" {
			if dep.Name != "" {
				deps = append(deps, dep)
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
		deps = append(deps, dep)
	}
	return deps, file, nil
}

func (p *javaProvider) GetDependenciesDAG() ([]provider.DepDAGItem, uri.URI, error) {
	localRepoPath := p.getLocalRepoPath()

	path := p.findPom()
	file := uri.File(path)

	//Create temp file to use
	f, err := os.CreateTemp("", "*")
	if err != nil {
		return nil, file, err
	}
	defer os.Remove(f.Name())

	moddir := filepath.Dir(path)
	// get the graph output
	cmd := exec.Command("mvn", "dependency:tree", fmt.Sprintf("-DoutputFile=%s", f.Name()))
	cmd.Dir = moddir
	err = cmd.Run()
	if err != nil {
		return nil, file, err
	}

	b, err := os.ReadFile(f.Name())
	if err != nil {
		return nil, file, err
	}

	lines := strings.Split(string(b), "\n")

	// strip first and last line of the output
	// first line is the base package, last line empty
	if len(lines) > 2 {
		lines = lines[1 : len(lines)-2]
	}

	deps := []provider.DepDAGItem{}

	deps, err = parseMavenDepLines(lines, localRepoPath)
	if err != nil {
		return nil, file, err
	}

	//Walk the dir, looking for .jar files to add to the dependency
	w := walker{
		deps: deps,
	}
	filepath.WalkDir(moddir, w.walkDirForJar)

	return deps, file, nil
}

type walker struct {
	deps []provider.DepDAGItem
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
		w.deps = append(w.deps, provider.DepDAGItem{
			Dep: d,
		})
	}
	return nil
}

// parseDepString parses a java dependency string
// assumes format <group>:<name>:<type>:<version>:<scope>
func parseDepString(dep, localRepoPath string) (provider.Dep, error) {
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

	return d, nil
}

// parseMavenDepLines recursively parses output lines from maven dependency tree
func parseMavenDepLines(lines []string, localRepoPath string) ([]provider.DepDAGItem, error) {
	if len(lines) > 0 {
		baseDepString := lines[0]
		baseDep, err := parseDepString(baseDepString, localRepoPath)
		if err != nil {
			return nil, err
		}
		item := provider.DepDAGItem{}
		item.Dep = baseDep
		item.AddedDeps = []provider.DepDAGItem{}
		idx := 1
		// indirect deps are separated by 3 or more spaces after the direct dep
		for idx < len(lines) && strings.Count(lines[idx], " ") > 2 {
			transitiveDep, err := parseDepString(lines[idx], localRepoPath)
			if err != nil {
				return nil, err
			}
			transitiveDep.Indirect = true
			item.AddedDeps = append(item.AddedDeps, provider.DepDAGItem{Dep: transitiveDep})
			idx += 1
		}
		ds, err := parseMavenDepLines(lines[idx:], localRepoPath)
		if err != nil {
			return nil, err
		}
		ds = append(ds, item)
		return ds, nil
	}
	return []provider.DepDAGItem{}, nil
}
