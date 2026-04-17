package go_external_provider

import (
	"bytes"
	"fmt"
	"os"
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
	// envDependencyProviderModuleDir can be set to specify the module directory
	envDependencyProviderModuleDir = "DEPENDENCY_PROVIDER_MODULE_DIR"
)

// getDependenciesDAG runs go mod graph in the workspace and parses the output
// to build a dependency DAG.
func getDependenciesDAG(workspaceFolder string) (map[uri.URI][]provider.DepDAGItem, error) {
	// Remove file:// prefix if present
	workspacePath := workspaceFolder
	if strings.HasPrefix(workspaceFolder, "file://") {
		workspacePath = workspaceFolder[7:]
	}

	// If workspace folder is provided and has DEPENDENCY_PROVIDER_MODULE_DIR env var not set,
	// temporarily set it for resolveModuleRoot to use
	if workspacePath != "" && os.Getenv(envDependencyProviderModuleDir) == "" {
		defer func() {
			os.Unsetenv(envDependencyProviderModuleDir)
		}()
		os.Setenv(envDependencyProviderModuleDir, workspacePath)
	}

	modRoot, err := resolveModuleRoot()
	if err != nil {
		return nil, err
	}
	goModPath := filepath.Join(modRoot, "go.mod")
	file := uri.File(goModPath)

	// get the graph output
	buf := bytes.Buffer{}
	cmd := exec.Command("go", "mod", "graph")
	cmd.Dir = modRoot
	cmd.Stdout = &buf
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("go mod graph in %q: %w: %s", modRoot, err, strings.TrimSpace(stderr.String()))
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
		labels.AsString(provider.DepSourceLabel, golangDownloadableDepSourceLabel),
		labels.AsString(provider.DepLanguageLabel, "go"),
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

// resolveModuleRoot determines the Go module root directory.
// It first checks the DEPENDENCY_PROVIDER_MODULE_DIR environment variable.
// If not set, it uses go env GOMOD to find the module.
// If that fails, it searches for go.mod starting from the current directory.
func resolveModuleRoot() (string, error) {
	if d := os.Getenv(envDependencyProviderModuleDir); d != "" {
		abs, err := filepath.Abs(filepath.Clean(d))
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(filepath.Join(abs, "go.mod")); err != nil {
			return "", err
		}
		return abs, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	wd = filepath.Clean(wd)

	gomod, err := goEnvGOMOD(wd)
	if err != nil {
		return "", err
	}
	if !isNoModuleGOMOD(gomod) {
		return filepath.Clean(filepath.Dir(gomod)), nil
	}

	root, err := findGoModPath(wd)
	if err != nil {
		return "", err
	}
	return root, nil
}

// goEnvGOMOD runs "go env GOMOD" to get the path to the go.mod file.
func goEnvGOMOD(dir string) (string, error) {
	cmd := exec.Command("go", "env", "GOMOD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// isNoModuleGOMOD checks if the GOMOD output indicates no module is active.
func isNoModuleGOMOD(gomod string) bool {
	if gomod == "" {
		return true
	}
	clean := filepath.Clean(gomod)
	if clean == filepath.Clean(os.DevNull) {
		return true
	}
	base := strings.ToLower(filepath.Base(clean))
	return base == "nul" || base == "null"
}

// findGoModPath searches for a go.mod file starting from root directory.
// It performs a breadth-first search, skipping common directories that
// typically don't contain the main go.mod (vendor, .git, node_modules, .cache).
func findGoModPath(root string) (string, error) {
	rootAbs, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	skipDir := map[string]bool{
		"vendor":       true,
		".git":         true,
		"node_modules": true,
		".cache":       true,
	}
	seen := map[string]bool{rootAbs: true}
	queue := []string{rootAbs}
	for len(queue) > 0 {
		dir := queue[0]
		queue = queue[1:]
		goModPath := filepath.Join(dir, "go.mod")
		if st, err := os.Stat(goModPath); err == nil && !st.IsDir() {
			return dir, nil
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if skipDir[e.Name()] {
				continue
			}
			subAbs, err := filepath.Abs(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			if seen[subAbs] {
				continue
			}
			seen[subAbs] = true
			queue = append(queue, subAbs)
		}
	}
	return "", fmt.Errorf("no go.mod found under %q", rootAbs)
}
