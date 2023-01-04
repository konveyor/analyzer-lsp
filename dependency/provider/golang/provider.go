package golang

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/konveyor/analyzer-lsp/dependency/dependency"
	"github.com/konveyor/analyzer-lsp/dependency/provider"
)

type depProvider struct {
}

func GetDepProvider() provider.DependencyProvider {
	return &depProvider{}
}

func (d *depProvider) GetDependencies(path string) ([]dependency.Dep, error) {
	// We are going to run the graph command, and write a parser for this.
	// This is so that we can get the tree of deps.

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

	depsListed := []string{}
	for _, l := range lines {
		// get all base mod deps.
		if len(l) == 0 {
			continue
		}
		values := strings.Split(l, " ")
		if len(values) != 2 {
			fmt.Printf("\nthis is odd and deal with it: %v", l)
		}
		depsListed = append(depsListed, values[1])
	}

	deps := []dependency.Dep{}
	for _, k := range depsListed {
		v := strings.Split(k, "@")
		if len(v) != 2 {
			fmt.Printf("something went wrong")

		}
		d := dependency.Dep{
			Name:    strings.TrimSpace(v[0]),
			Version: strings.ReplaceAll(v[1], "@", ""),
		}
		deps = append(deps, d)
	}
	return deps, nil
}
