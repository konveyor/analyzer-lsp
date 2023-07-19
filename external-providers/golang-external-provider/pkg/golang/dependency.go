package golang

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/konveyor/analyzer-lsp/engine/labels"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
)

const (
	// This will communicate that, the dep is downloadable and not vendored.
	golangDownloadableDepSourceLabel = "downloadable"
)

// createDepSourceLabel creates a dependency source label with the specified value.
func createDepSourceLabel(label, value string) string {
    return fmt.Sprintf("%s=%s", label, value)
}

// createDepLanguageLabel creates a dependency language label with the specified value.
func createDepLanguageLabel(label, value string) string {
    return fmt.Sprintf("%s=%s", label, value)
}


// TODO implement this for real
func (g *golangServiceClient) findGoMod() string {
	var depPath string
	if g.config.DependencyPath == "" {
		depPath = "go.mod"
	} else {
		depPath = g.config.DependencyPath
	}
	f, err := filepath.Abs(filepath.Join(g.config.Location, depPath))
	if err != nil {
		return ""
	}
	return f
}

func (g *golangServiceClient) GetDependencies() (map[uri.URI][]*provider.Dep, error) {
	ll, err := g.GetDependenciesDAG()
	if err != nil {
		return nil, err
	}
	if len(ll) == 0 {
		return nil, nil
	}

	m := map[uri.URI][]*provider.Dep{}
	for u, d := range ll {
		m[u] = provider.ConvertDagItemsToList(d)
	}

	return m, err
}

func (g *golangServiceClient) GetDependenciesDAG() (map[uri.URI][]provider.DepDAGItem, error) {
	// We are going to run the graph command, and write a parser for this.
	// This is so that we can get the tree of deps.

	path := g.findGoMod()
	file := uri.File(path)

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
	m := map[uri.URI][]provider.DepDAGItem{}
	m[file] = deps

	return m, nil
}

// parseGoDepString parses a golang dependency string
// assumes format <dependency_name>@<version>
func parseGoDepString(dep string) (provider.Dep, error) {
	d := provider.Dep{}
	v := strings.Split(dep, "@")
	if len(v) != 2 {
		return d, fmt.Errorf("failed to parse dependency string %s", dep)
	}
	d.Name = strings.TrimSpace(v[0])
	d.Version = strings.TrimSpace(strings.ReplaceAll(v[1], "@", ""))
	d.Labels = []string{
<<<<<<< HEAD
		labels.AsString(provider.DepSourceLabel,golangDownloadableDepSourceLabel),
		labels.AsString(provider.DepLanguageLabel,"go"),
=======
		createDepSourceLabel(provider.DepSourceLabel, golangDownloadableDepSourceLabel),
		createDepLanguageLabel(provider.DepLanguageLabel, "go"),
>>>>>>> 5d4e9ee20df94d06832dd0fbf058987f81105bbe
	}
	return d, nil
}

// parseGoDepLines parses go mod graph output
func parseGoDepLines(lines []string) ([]provider.DepDAGItem, error) {
	depsListed := map[string][]string{}
	var root *string
	deps := []provider.DepDAGItem{}

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
		d := provider.DepDAGItem{
			Dep: directDep,
		}

		indirect := []provider.DepDAGItem{}

		// traverse the list to find all indirect deps for current dep
		for _, indirectDepString := range depsListed[directDepString] {
			indirectDep, err := parseGoDepString(indirectDepString)
			if err != nil {
				// TODO: handle error better
				fmt.Println(err.Error())
				return deps, err
			}
			indirect = append(indirect, provider.DepDAGItem{Dep: indirectDep})
		}
		d.AddedDeps = indirect
		deps = append(deps, d)
	}
	return deps, nil
}
