package java

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/konveyor/analyzer-lsp/dependency/dependency"
)

// TODO implement this for real
func (p *javaProvider) findPom() string {
	var depPath string
	if p.config.DependencyPath == "" {
		depPath = "pom.xml"
	} else {
		depPath = p.config.DependencyPath
	}
	return filepath.Join(p.config.Location, depPath)
}

func (p *javaProvider) GetDependencies() ([]dependency.Dep, error) {
	ll, err := p.GetDependenciesLinkedList()
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

func (p *javaProvider) getLocalRepoPath() string {
	cmd := exec.Command("mvn", "help:evaluate", "-Dexpression=settings.localRepository", "-q", "-DforceStdout")
	var outb bytes.Buffer
	cmd.Stdout = &outb
	err := cmd.Run()
	if err != nil {
		fmt.Printf("here: %v", err)
		return ""
	}

	// check errors
	return string(outb.String())
}

func (p *javaProvider) GetDependenciesLinkedList() (map[dependency.Dep][]dependency.Dep, error) {

	localRepoPath := p.getLocalRepoPath()

	path := p.findPom()

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

	err = parseMavenDepLines(lines, deps, localRepoPath)
	if err != nil {
		return nil, err
	}

	return deps, nil
}

// parseDepString parses a java dependency string
// assumes format <group>:<name>:<type>:<version>:<scope>
func parseDepString(dep, localRepoPath string) (dependency.Dep, error) {
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
		return d, fmt.Errorf("unable to split dependency string %s", dep)
	}
	d.Name = fmt.Sprintf("%s.%s", parts[0], parts[1])
	d.Version = parts[3]
	d.Type = parts[4]

	fp := filepath.Join(localRepoPath, strings.Replace(d.Name, ".", "/", -1), d.Version, fmt.Sprintf("%v-%v.jar.sha1", parts[1], d.Version))
	b, err := os.ReadFile(fp)
	if err != nil {
		return d, err
	}
	d.SHA = string(b)

	return d, nil
}

// parseMavenDepLines recursively parses output lines from maven dependency tree
func parseMavenDepLines(lines []string, deps map[dependency.Dep][]dependency.Dep, localRepoPath string) error {
	if len(lines) > 0 {
		baseDepString := lines[0]
		baseDep, err := parseDepString(baseDepString, localRepoPath)
		if err != nil {
			return err
		}
		if _, ok := deps[baseDep]; !ok {
			deps[baseDep] = make([]dependency.Dep, 0)
		}
		idx := 1
		// indirect deps are separated by 3 or more spaces after the direct dep
		for idx < len(lines) && strings.Count(lines[idx], " ") > 2 {
			transitiveDep, err := parseDepString(lines[idx], localRepoPath)
			if err != nil {
				return err
			}
			transitiveDep.Indirect = true
			deps[baseDep] = append(deps[baseDep], transitiveDep)
			idx += 1
		}
		err = parseMavenDepLines(lines[idx:], deps, localRepoPath)
		if err != nil {
			return err
		}
	}
	return nil
}
