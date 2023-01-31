package java

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/konveyor/analyzer-lsp/dependency/dependency"
)

func (p *javaProvider) GetDependencies(path string) (map[dependency.Dep][]dependency.Dep, error) {

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

	deps := map[dependency.Dep][]dependency.Dep{}

	err = parseMavenDepLines(lines, deps)
	if err != nil {
		return nil, err
	}

	return deps, nil
}

// parseDepString parses a java dependency string
// assumes format <group>:<name>:<type>:<version>:<scope>
func parseDepString(dep string) (dependency.Dep, error) {
	d := dependency.Dep{}
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
		return d, fmt.Errorf("unable to split depdenecy string %s", dep)
	}
	d.Name = fmt.Sprintf("%s.%s", parts[0], parts[1])
	d.Version = parts[3]
	d.Location = parts[4]
	return d, nil
}

// parseMavenDepLines recursively parses output lines from maven dependency tree
func parseMavenDepLines(lines []string, deps map[dependency.Dep][]dependency.Dep) error {
	if len(lines) > 0 {
		baseDepString := lines[0]
		baseDep, err := parseDepString(baseDepString)
		if err != nil {
			return err
		}
		if _, ok := deps[baseDep]; !ok {
			deps[baseDep] = make([]dependency.Dep, 0)
		}
		idx := 1
		// indirect deps are separated by 3 or more spaces after the direct dep
		for idx < len(lines) && strings.Count(lines[idx], " ") > 2 {
			transitiveDep, err := parseDepString(lines[idx])
			if err != nil {
				return err
			}
			deps[baseDep] = append(deps[baseDep], transitiveDep)
			idx += 1
		}
		err = parseMavenDepLines(lines[idx:], deps)
		if err != nil {
			return err
		}
	}
	return nil
}
