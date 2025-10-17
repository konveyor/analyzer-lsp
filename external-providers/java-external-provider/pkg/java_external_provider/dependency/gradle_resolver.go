package dependency

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/go-logr/logr"
	"github.com/hashicorp/go-version"
	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency/labels"
	"github.com/konveyor/analyzer-lsp/tracing"
)

type gradleResolver struct {
	log                logr.Logger
	decompileTool      string
	labeler            labels.Labeler
	gradleVersion      version.Version
	location           string
	buildFile          string
	wrapper            string
	javaHome           string
	taskFile           string
	localRepo          string
	disableMavenSearch bool
}

func GetGradleResolver(opts ResolverOptions) Resolver {
	return &gradleResolver{
		log:           opts.Log,
		gradleVersion: opts.Version,
		location:      opts.Location,
		buildFile:     opts.BuildFile,
		wrapper:       opts.Wrapper,
		javaHome:      opts.JavaHome,
		decompileTool: opts.DecompileTool,
		labeler:       opts.Labeler,
		taskFile:      opts.GradleTaskFile,
	}
}

func (g *gradleResolver) ResolveSources(ctx context.Context) (string, string, error) {
	ctx, span := tracing.StartNewSpan(ctx, "resolve-sources")
	defer span.End()

	g.log.V(5).Info("resolving dependency sources for gradle")

	// create a temporary build file to append the task for downloading sources
	taskgb := filepath.Join(filepath.Dir(g.buildFile), "tmp.gradle")
	err := CopyFile(g.buildFile, taskgb)
	if err != nil {
		return "", "", fmt.Errorf("error copying file %s to %s", g.buildFile, taskgb)
	}
	defer os.Remove(taskgb)

	// append downloader task
	if g.taskFile == "" {
		// if taskFile is empty, we are in container mode
		g.taskFile = "/usr/local/etc/task.gradle"
	}
	// if Gradle >= 9.0, use a newer script for downloading sources
	gradle9version, _ := version.NewVersion("9.0")
	if g.gradleVersion.GreaterThanOrEqual(gradle9version) {
		g.taskFile = filepath.Join(filepath.Dir(g.taskFile), "task-v9.gradle")
	}

	err = AppendToFile(g.taskFile, taskgb)
	if err != nil {
		return "", "", fmt.Errorf("error appending file %s to %s", g.taskFile, taskgb)
	}

	tmpgbname := filepath.Join(g.location, "toberenamed.gradle")
	err = os.Rename(g.buildFile, tmpgbname)
	if err != nil {
		return "", "", fmt.Errorf("error renaming file %s to %s", g.buildFile, "toberenamed.gradle")
	}
	defer os.Rename(tmpgbname, g.buildFile)

	err = os.Rename(taskgb, g.buildFile)
	if err != nil {
		return "", "", fmt.Errorf("error renaming file %s to %s", g.buildFile, "toberenamed.gradle")
	}
	defer os.Remove(g.buildFile)

	args := []string{
		"konveyorDownloadSources",
	}
	cmd := exec.CommandContext(ctx, g.wrapper, args...)
	cmd.Env = append(cmd.Env, fmt.Sprintf("JAVA_HOME=%s", g.javaHome))
	cmd.Dir = g.location
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("error trying to get sources for Gradle: %w - Gradle output: %s", err, output)
	}

	g.log.V(8).WithValues("output", output).Info("got gradle output")

	// TODO: what if all sources available
	reader := bytes.NewReader(output)
	unresolvedSources, err := g.parseUnresolvedSourcesForGradle(reader)
	if err != nil {
		return "", "", err
	}

	g.log.V(5).Info("total unresolved sources", "count", len(unresolvedSources))

	if len(unresolvedSources) > 1 {
		// Gradle cache dir structure changes over time - we need to find where the actual dependencies are stored
		cache, err := g.findGradleCache(unresolvedSources[0].GroupId)
		if err != nil {
			return "", "", err
		}
		decompiler, err := getDecompiler(DecompilerOpts{
			DecompileTool:      g.decompileTool,
			log:                g.log,
			workers:            DefaultWorkerPoolSize,
			labler:             g.labeler,
			disableMavenSearch: g.disableMavenSearch,
			m2Repo:             cache,
		})
		if err != nil {
			return "", "", err
		}

		wg := &sync.WaitGroup{}
		dependencies := []JavaArtifact{}
		returnChan := make(chan struct {
			artifact []JavaArtifact
			err      error
		})
		decompilerCtx, cancelFunc := context.WithCancel(ctx)

		go func() {
			for {
				select {
				case resp := <-returnChan:
					defer wg.Done()
					if resp.err != nil {
						g.log.Error(err, "unable to get java artifact")
						continue
					}
					dependencies = append(dependencies, resp.artifact...)
				case <-decompilerCtx.Done():
					return
				}
			}
		}()
		for _, artifact := range unresolvedSources {
			g.log.V(5).WithValues("artifact", artifact).Info("sources for artifact not found, decompiling...")

			artifactDir := filepath.Join(cache, artifact.GroupId, artifact.ArtifactId)
			jarName := fmt.Sprintf("%s-%s.jar", artifact.ArtifactId, artifact.Version)
			artifactPath, err := g.findGradleArtifact(artifactDir, jarName)
			if err != nil {
				cancelFunc()
				return "", "", err
			}
			wg.Add(1)
			go func() {
				artifact, err := decompiler.Decompile(decompilerCtx, artifactPath)
				returnChan <- struct {
					artifact []JavaArtifact
					err      error
				}{artifact: artifact, err: err}
			}()
		}

		wg.Done()
		cancelFunc()

		return g.location, cache, nil
	}
	return g.location, "", nil
}

// findGradleCache looks for the folder within the Gradle cache where the actual dependencies are stored
// by walking the cache directory looking for a directory equal to the given sample group id
func (g *gradleResolver) findGradleCache(sampleGroupId string) (string, error) {
	gradleHome := g.findGradleHome()
	cacheRoot := filepath.Join(gradleHome, "caches")
	cache := ""
	walker := func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("found error looking for cache directory: %w", err)
		}
		if d.IsDir() && d.Name() == sampleGroupId {
			cache = path
			return filepath.SkipAll
		}
		return nil
	}
	err := filepath.WalkDir(cacheRoot, walker)
	if err != nil {
		return "", err
	}
	cache = filepath.Dir(cache) // return the parent of the found directory
	return cache, nil
}

// findGradleHome tries to get the .gradle directory from several places
// 1. check $GRADLE_HOME
// 2. check $HOME/.gradle
// 3. else, set to /root/.gradle
func (g *gradleResolver) findGradleHome() string {
	gradleHome := os.Getenv("GRADLE_HOME")
	if gradleHome == "" {
		home := os.Getenv("HOME")
		if home == "" {
			home = "/root"
		}
		gradleHome = filepath.Join(home, ".gradle")
	}
	return gradleHome
}

// findGradleArtifact looks for a given artifact jar within the given root dir
func (g *gradleResolver) findGradleArtifact(root string, artifactId string) (string, error) {
	artifactPath := ""
	walker := func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("found error looking for artifact: %w", err)
		}
		if !d.IsDir() && d.Name() == artifactId {
			artifactPath = path
			return filepath.SkipAll
		}
		return nil
	}
	err := filepath.WalkDir(root, walker)
	if err != nil {
		return "", err
	}
	return artifactPath, nil
}

// parseUnresolvedSources takes the output from the download sources gradle task and returns the artifacts whose sources
// could not be found. Sample gradle output:
// Found 0 sources for :simple-jar:
// Found 1 sources for com.codevineyard:hello-world:1.0.1
// Found 1 sources for org.codehaus.groovy:groovy:3.0.21
func (g *gradleResolver) parseUnresolvedSourcesForGradle(output io.Reader) ([]JavaArtifact, error) {
	unresolvedSources := []JavaArtifact{}
	unresolvedRegex := regexp.MustCompile(`Found 0 sources for (.*)`)
	artifactRegex := regexp.MustCompile(`(.+):(.+):(.+)|:(.+):`)

	scanner := bufio.NewScanner(output)
	for scanner.Scan() {
		line := scanner.Text()

		if match := unresolvedRegex.FindStringSubmatch(line); len(match) != 0 {
			gav := artifactRegex.FindStringSubmatch(match[1])
			if gav[4] != "" { // internal library, unknown group/version
				artifact := JavaArtifact{
					ArtifactId: match[4],
				}
				unresolvedSources = append(unresolvedSources, artifact)
			} else { // external dependency
				artifact := JavaArtifact{
					GroupId:    gav[1],
					ArtifactId: gav[2],
					Version:    gav[3],
				}
				unresolvedSources = append(unresolvedSources, artifact)
			}
		}
	}

	// dedup artifacts
	result := []JavaArtifact{}
	for _, artifact := range unresolvedSources {
		if contains(result, artifact) {
			continue
		}
		result = append(result, artifact)
	}

	return result, scanner.Err()
}
