package bldtool

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/hashicorp/go-version"
	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency"
	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency/labels"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
)

// gradleBuildTool implements the BuildTool interface for Gradle-based Java projects.
// It handles projects with build.gradle files, extracting dependencies using Gradle
// dependency resolution tasks and custom Gradle scripts.
//
// This implementation supports:
//   - Standard Gradle projects with build.gradle
//   - Gradle wrapper execution for reproducible builds
//   - Custom dependency resolution tasks (task.gradle, task-v9.gradle for Gradle >= 9.0)
//   - Caching based on build.gradle hash to avoid redundant processing
//   - Maven repository searches for artifact metadata (unless disabled)
type gradleBuildTool struct {
	*depCache
	taskFile       string // Path to custom Gradle task file for dependency resolution
	mavenIndexPath string
	log            logr.Logger    // Logger instance for this build tool
	labeler        labels.Labeler // Labeler for identifying open source vs internal dependencies
}

func getGradleBuildTool(opts BuildToolOptions, log logr.Logger) BuildTool {
	log = log.WithName("gradle-bldtool")
	if opts.Config.Location != "" {
		path := filepath.Join(opts.Config.Location, "build.gradle")
		_, err := os.Stat(path)
		if err != nil {
			return nil
		}
		f, err := filepath.Abs(path)
		if err != nil {
			return nil
		}
		return &gradleBuildTool{
			depCache: &depCache{
				hashFile: f,
				hashSync: sync.Mutex{},
				depLog:   log.WithName("dep-cache"),
			},
			taskFile:       opts.GradleTaskFile,
			mavenIndexPath: opts.MavenIndexPath,
			log:            log,
			labeler:        opts.Labeler,
		}
	}
	return nil
}

func (g *gradleBuildTool) ShouldResolve() bool {
	return false
}

func (g *gradleBuildTool) GetResolver(decompileTool string) (dependency.Resolver, error) {
	gradleVersion, err := g.GetGradleVersion(context.TODO())
	if err != nil {
		return nil, err
	}
	gradleWrapper, err := g.GetGradleWrapper()
	if err != nil {
		return nil, err
	}
	javaHome, err := g.GetJavaHomeForGradle(context.TODO())
	if err != nil {
		return nil, err
	}

	opts := dependency.ResolverOptions{
		Log:            g.log,
		Location:       filepath.Dir(g.hashFile),
		BuildFile:      g.hashFile,
		Version:        gradleVersion,
		Wrapper:        gradleWrapper,
		JavaHome:       javaHome,
		DecompileTool:  decompileTool,
		Labeler:        g.labeler,
		GradleTaskFile: g.taskFile,
		MavenIndexPath: g.mavenIndexPath,
	}
	return dependency.GetGradleResolver(opts), nil
}

func (g *gradleBuildTool) GetSourceFileLocation(path string, jarPath string, javaFileName string) (string, error) {
	sourcesFile := ""
	jarFile := filepath.Base(jarPath)
	walker := func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("found error traversing files: %w", err)
		}
		if !d.IsDir() && d.Name() == jarFile {
			sourcesFile = path
			return nil
		}
		return nil
	}
	root := filepath.Join(jarPath, "..", "..")
	err := filepath.WalkDir(root, walker)
	if err != nil {
		return "", err
	}
	javaFileAbsolutePath := filepath.Join(filepath.Dir(sourcesFile), filepath.Dir(path), javaFileName)

	if _, err := os.Stat(filepath.Dir(javaFileAbsolutePath)); err != nil {
		cmd := exec.Command("jar", "xf", filepath.Base(sourcesFile))
		cmd.Dir = filepath.Dir(sourcesFile)
		err = cmd.Run()
		if err != nil {
			g.log.Error(err, "error unpacking java archive")
			return "", err
		}
	}
	return javaFileAbsolutePath, nil
}

func (g *gradleBuildTool) GetLocalRepoPath() string {
	return ""
}

// getDependenciesForGradle invokes the Gradle wrapper to get the dependency tree and returns all project dependencies
func (g *gradleBuildTool) GetDependencies(ctx context.Context) (map[uri.URI][]provider.DepDAGItem, error) {
	g.log.V(3).Info("getting deps")
	ok, err := g.depCache.useCache()
	if err != nil {
		return nil, err
	}
	if ok {
		return g.depCache.getCachedDeps(), nil
	}
	subprojects, err := g.getGradleSubprojects(ctx)
	if err != nil {
		return nil, err
	}

	// command syntax: ./gradlew subproject1:dependencies subproject2:dependencies ...
	args := []string{}
	if len(subprojects) > 0 {
		for _, sp := range subprojects {
			args = append(args, fmt.Sprintf("%s:dependencies", sp))
		}
	} else {
		args = append(args, "dependencies")
	}

	// get the graph output
	exe, err := filepath.Abs(filepath.Join(filepath.Dir(g.hashFile), "gradlew"))
	if err != nil {
		return nil, fmt.Errorf("error calculating gradle wrapper path")
	}
	if _, err = os.Stat(exe); errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("a gradle wrapper must be present in the project")
	}

	timeout, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	javaHome, err := g.GetJavaHomeForGradle(ctx)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(timeout, exe, args...)
	cmd.Dir = filepath.Dir(g.hashFile)
	cmd.Env = append(cmd.Env, fmt.Sprintf("JAVA_HOME=%s", javaHome))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("error trying to get Gradle dependencies: %w - Gradle output: %s", err, string(output))
	}

	lines := strings.Split(string(output), "\n")
	deps := g.parseGradleDependencyOutput(lines)

	file := uri.File(g.hashFile)
	m := map[uri.URI][]provider.DepDAGItem{}
	m[file] = deps
	g.depCache.setCachedDeps(m, err)
	return m, nil
}

func (g *gradleBuildTool) getGradleSubprojects(ctx context.Context) ([]string, error) {
	args := []string{
		"projects",
	}

	javaHome, err := g.GetJavaHomeForGradle(ctx)
	if err != nil {
		return nil, err
	}

	exe, err := filepath.Abs(filepath.Join(filepath.Dir(g.hashFile), "gradlew"))
	if err != nil {
		return nil, fmt.Errorf("error calculating gradle wrapper path")
	}
	if _, err = os.Stat(exe); errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("a gradle wrapper must be present in the project")
	}
	cmd := exec.Command(exe, args...)
	cmd.Dir = filepath.Dir(g.hashFile)
	cmd.Env = append(cmd.Env, fmt.Sprintf("JAVA_HOME=%s", javaHome))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("error getting gradle subprojects: %w - Gradle output: %s", err, string(output))
	}

	beginRegex := regexp.MustCompile(`Root project`)
	endRegex := regexp.MustCompile(`To see a list of`)
	npRegex := regexp.MustCompile(`No sub-projects`)
	pRegex := regexp.MustCompile(`.*- Project '(.*)'`)

	subprojects := []string{}

	gather := false
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if npRegex.Find([]byte(line)) != nil {
			return []string{}, nil
		}
		if beginRegex.Find([]byte(line)) != nil {
			gather = true
			continue
		}
		if gather {
			if endRegex.Find([]byte(line)) != nil {
				return subprojects, nil
			}

			if p := pRegex.FindStringSubmatch(line); p != nil {
				subprojects = append(subprojects, p[1])
			}
		}
	}

	return subprojects, fmt.Errorf("error parsing gradle dependency output")
}

// parseGradleDependencyOutput converts the relevant lines from the dependency output into actual dependencies
// See https://regex101.com/r/9Gp7dW/1 for context
func (g *gradleBuildTool) parseGradleDependencyOutput(lines []string) []provider.DepDAGItem {
	deps := []provider.DepDAGItem{}

	treeDepRegex := regexp.MustCompile(`^([| ]+)?[+\\]--- (.*)`)

	// map of <anidation level> to <pointer to last found dependency for given level>
	// this is so that children can be added to their respective parents
	lastFoundWithDepth := make(map[int]*provider.DepDAGItem)

	for _, line := range lines {
		match := treeDepRegex.FindStringSubmatch(line)
		if match != nil {
			dep := g.parseGradleDependencyString(match[2])
			if reflect.DeepEqual(dep, provider.DepDAGItem{}) { // ignore empty dependency
				continue
			} else if match[1] != "" { // transitive dependency
				dep.Dep.Indirect = true
				depth := len(match[1]) / 5                                             // get the level of anidation of the dependency within the tree
				parent := lastFoundWithDepth[depth-1]                                  // find its parent
				parent.AddedDeps = append(parent.AddedDeps, dep)                       // add child to parent
				lastFoundWithDepth[depth] = &parent.AddedDeps[len(parent.AddedDeps)-1] // update last found with given depth
			} else { // root level (direct) dependency
				deps = append(deps, dep) // add root dependency to result list
				lastFoundWithDepth[0] = &deps[len(deps)-1]
				continue
			}
		}
	}

	return deps
}

// parseGradleDependencyString parses the lines of the gradle dependency output, for instance:
// org.codehaus.groovy:groovy:3.0.21 (c)
// org.codehaus.groovy:groovy:3.+ -> 3.0.21
// com.codevineyard:hello-world:{strictly 1.0.1} -> 1.0.1
// :simple-jar (n)
func (g *gradleBuildTool) parseGradleDependencyString(s string) provider.DepDAGItem {
	// (*) - dependencies omitted (listed previously)
	// (n) - Not resolved (configuration is not meant to be resolved)
	// (c) - A dependency constraint (not a dependency, to be ignored)
	if strings.HasSuffix(s, "(n)") || strings.HasSuffix(s, "(*)") || strings.HasSuffix(s, "(c)") {
		return provider.DepDAGItem{}
	}

	depRegex := regexp.MustCompile(`(.+):(.+)(:| -> )((.*) -> )?(.*)`)
	libRegex := regexp.MustCompile(`:(.*)`)

	dep := provider.Dep{}
	match := depRegex.FindStringSubmatch(s)
	if match != nil {
		dep.Name = match[1] + "." + match[2]
		dep.Version = match[6]
	} else if match = libRegex.FindStringSubmatch(s); match != nil {
		dep.Name = match[1]
	}

	return provider.DepDAGItem{Dep: dep, AddedDeps: []provider.DepDAGItem{}}
}

func (g *gradleBuildTool) GetGradleWrapper() (string, error) {
	wrapper := "gradlew"
	if runtime.GOOS == "windows" {
		wrapper = "gradlew.bat"
	}
	exe, err := filepath.Abs(filepath.Join(filepath.Dir(g.hashFile), wrapper))
	if err != nil {
		return "", fmt.Errorf("error calculating gradle wrapper path")
	}
	if _, err = os.Stat(exe); errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("a gradle wrapper is not present in the project")
	}
	return exe, err
}

func (g *gradleBuildTool) GetGradleVersion(ctx context.Context) (version.Version, error) {
	exe, err := g.GetGradleWrapper()
	if err != nil {
		return version.Version{}, err
	}

	// getting the Gradle version is the first step for guessing compatibility
	// up to 8.14 is compatible with Java 8, so let's first try to run with that
	args := []string{
		"--version",
	}
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = filepath.Dir(g.hashFile)
	cmd.Env = append(cmd.Env, fmt.Sprintf("JAVA_HOME=%s", os.Getenv("JAVA8_HOME")))
	output, err := cmd.CombinedOutput()
	if err != nil {
		// if executing with 8 we get an error, try with 17
		cmd = exec.CommandContext(ctx, exe, args...)
		cmd.Dir = filepath.Dir(g.hashFile)
		cmd.Env = append(cmd.Env, fmt.Sprintf("JAVA_HOME=%s", os.Getenv("JAVA_HOME")))
		output, err = cmd.CombinedOutput()
		if err != nil {
			return version.Version{}, fmt.Errorf("error trying to get Gradle version: %w - Gradle output: %s", err, string(output))
		}
	}

	vRegex := regexp.MustCompile(`Gradle (\d+(\.\d+)*)`)
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if match := vRegex.FindStringSubmatch(line); len(match) != 0 {
			v, err := version.NewVersion(match[1])
			if err != nil {
				return version.Version{}, err
			}
			return *v, err
		}
	}
	return version.Version{}, nil
}

func (g *gradleBuildTool) GetJavaHomeForGradle(ctx context.Context) (string, error) {
	v, err := g.GetGradleVersion(ctx)
	if err != nil {
		return "", err
	}
	lastVersionForJava8, _ := version.NewVersion("8.14")
	if v.LessThanOrEqual(lastVersionForJava8) {
		java8home := os.Getenv("JAVA8_HOME")
		if java8home == "" {
			return "", fmt.Errorf("couldn't get JAVA8_HOME environment variable")
		}
		return java8home, nil
	}
	return os.Getenv("JAVA_HOME"), nil
}
