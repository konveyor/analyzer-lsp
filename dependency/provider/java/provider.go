package java

import (
	"fmt"
	"os"
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

	deps := []dependency.Dep{}
	for i, l := range lines {
		if i == 0 || i == len(lines)-1 {
			// the first value is the name of the jar for this pom
			// we will ignore this for now.
			continue
		}
		// remove all the pretty print characters.
		l = strings.TrimFunc(l, func(r rune) bool {
			if r == '+' || r == '-' || r == '\\' || r == '|' || r == ' ' || r == '"' {
				return true
			}
			return false

		})
		// Split string on ":" must have 5 parts.
		// For now we ignore Type as it appears most everything is a jar
		// GroupID:Name:Type:Version:Location
		parts := strings.Split(l, ":")
		if len(parts) != 5 {
			return nil, fmt.Errorf("unable to split depdenecy string correctly")
		}

		deps = append(deps, dependency.Dep{
			Name:     fmt.Sprintf("%s.%s", parts[0], parts[1]),
			Version:  parts[3],
			Location: parts[4],
		})

	}

	return deps, nil

}
