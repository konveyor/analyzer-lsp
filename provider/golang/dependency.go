package golang

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/konveyor/analyzer-lsp/dependency/dependency"
)

// TODO implement this for real
func (g *golangProvider) findGoMod() string {
	var depPath string
	if g.config.DependencyPath == "" {
		depPath = "go.mod"
	} else {
		depPath = g.config.DependencyPath
	}
	return filepath.Join(g.config.Location, depPath)
}

func (g *golangProvider) GetDependencies() ([]dependency.Dep, error) {
	ll, err := g.GetDependenciesLinkedList()
	if err != nil {
		return nil, err
	}
	if len(ll) == 0 {
		return nil, nil
	}
	deps := []dependency.Dep{}
	for topLevel, transitives := range ll {
		deps = append(deps, topLevel)
		deps = append(deps, transitives...)
	}
	return deps, err
}

func (g *golangProvider) GetDependenciesLinkedList() (map[dependency.Dep][]dependency.Dep, error) {
	// We are going to run the graph command, and write a parser for this.
	// This is so that we can get the tree of deps.

	path := g.findGoMod()

	moddir := filepath.Dir(path)
	// get the graph output
	buf := bytes.Buffer{}
	cmd := exec.Command("go", "mod", "graph")
	cmd.Dir = moddir
	cmd.Stdout = &buf
	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	// use base and graph to get the deps and their deps.

	data := buf.String()
	lines := strings.Split(data, "\n")

	deps, err := parseGoDepLines(lines)
	if err != nil {
		return nil, err
	}

	return deps, nil
}

// parseGoDepString parses a golang dependency string
// assumes format <dependency_name>@<version>
func parseGoDepString(dep string) (dependency.Dep, error) {
	d := dependency.Dep{}
	v := strings.Split(dep, "@")
	if len(v) != 2 {
		return d, fmt.Errorf("failed to parse dependency string %s", dep)
	}
	d.Name = strings.TrimSpace(v[0])
	d.Version = strings.TrimSpace(strings.ReplaceAll(v[1], "@", ""))
	return d, nil
}

// parseGoDepLines parses go mod graph output
func parseGoDepLines(lines []string) (map[dependency.Dep][]dependency.Dep, error) {
	depsListed := map[string][]string{}
	var root *string
	deps := map[dependency.Dep][]dependency.Dep{}

	for _, l := range lines {
		// get all base mod deps.
		if len(l) == 0 {
			continue
		}
		values := strings.Split(l, " ")
		if len(values) < 2 {
			return deps, fmt.Errorf("failed to split dependency line '%s'", l)
		}

		baseDep, nestedDep := values[0], values[1]
		if _, ok := depsListed[baseDep]; !ok {
			depsListed[baseDep] = make([]string, 0)
		}
		if _, ok := depsListed[nestedDep]; !ok {
			depsListed[nestedDep] = make([]string, 0)
		}
		depsListed[baseDep] = append(depsListed[baseDep], nestedDep)
		if root == nil {
			root = &baseDep
		}
	}

	if root == nil {
		if len(lines) > 1 {
			return deps, fmt.Errorf("failed to parse dependencies %s", strings.Join(lines, ""))
		}
		return deps, nil
	}

	for _, directDepString := range depsListed[*root] {
		directDep, err := parseGoDepString(directDepString)
		if err != nil {
			// TODO: handle error better
			fmt.Println(err.Error())
			return deps, err
		}
		if _, ok := deps[directDep]; !ok {
			deps[directDep] = make([]dependency.Dep, 0)
		}

		// traverse the list to find all indirect deps for current dep
		for _, indirectDepString := range depsListed[directDepString] {
			indirectDep, err := parseGoDepString(indirectDepString)
			if err != nil {
				// TODO: handle error better
				fmt.Println(err.Error())
				return deps, err
			}
			deps[directDep] = append(deps[directDep], indirectDep)
		}
	}
	return deps, nil
}
