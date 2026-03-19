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
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"github.com/hashicorp/go-version"
	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency/labels"
	"github.com/konveyor/analyzer-lsp/tracing"
)

type gradleResolver struct {
	log           logr.Logger
	decompileTool string
	labeler       labels.Labeler
	gradleVersion version.Version
	location      string
	buildFile     string
	wrapper       string
	javaHome      string
	taskFile      string
	localRepo     string
	mvnIndexPath  string
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

	taskFile := g.taskFile
	if taskFile == "" {
		taskFile = "/usr/local/etc/task.gradle"
	}
	gradle9version, _ := version.NewVersion("9.0")
	if g.gradleVersion.GreaterThanOrEqual(gradle9version) {
		taskFile = filepath.Join(filepath.Dir(taskFile), "task-v9.gradle")
	}

	// --build-file / -b was deprecated in Gradle 7.1 (https://github.com/gradle/gradle/issues/16402), still worked in 8.x,
	// and was removed in Gradle 9.0.0 (gradlew reports "Unknown command-line option '--build-file'" on 9.x).
	var args []string
	gradle9OrNewer := g.gradleVersion.GreaterThanOrEqual(gradle9version)

	if !gradle9OrNewer {
		// Gradle < 9: use a temp combined build file and --build-file so we never rename or modify the project's build.gradle.
		// This avoids cross-filesystem rename failures and leaves the project intact if the process is killed.
		buildContent, err := os.ReadFile(g.buildFile)
		if err != nil {
			return "", "", fmt.Errorf("error reading build file %s: %w", g.buildFile, err)
		}
		taskContent, err := os.ReadFile(taskFile)
		if err != nil {
			return "", "", fmt.Errorf("error reading task file %s: %w", taskFile, err)
		}
		combinedFile, err := os.CreateTemp(g.location, ".konveyor-sources-*.gradle")
		if err != nil {
			return "", "", fmt.Errorf("error creating temporary build file in %s: %w", g.location, err)
		}
		combinedPath := combinedFile.Name()
		defer os.Remove(combinedPath)
		if _, err := combinedFile.Write(buildContent); err != nil {
			combinedFile.Close()
			return "", "", fmt.Errorf("error writing to temporary build file: %w", err)
		}
		if _, err := combinedFile.Write(append([]byte("\n"), taskContent...)); err != nil {
			combinedFile.Close()
			return "", "", fmt.Errorf("error writing to temporary build file: %w", err)
		}
		if err := combinedFile.Close(); err != nil {
			return "", "", fmt.Errorf("error closing temporary build file: %w", err)
		}
		args = []string{"--build-file", combinedPath, "konveyorDownloadSources", "--no-daemon"}
	} else {
		// Gradle 9+: --build-file was removed in 9.0.0; use the original approach (temp file + rename) so Gradle sees build.gradle in the project dir.
		taskgb := filepath.Join(filepath.Dir(g.buildFile), "tmp.gradle")
		if err := CopyFile(g.buildFile, taskgb); err != nil {
			return "", "", fmt.Errorf("error copying file %s to %s: %w", g.buildFile, taskgb, err)
		}
		defer os.Remove(taskgb)
		if err := AppendToFile(taskFile, taskgb); err != nil {
			return "", "", fmt.Errorf("error appending file %s to %s: %w", taskFile, taskgb, err)
		}
		tmpgbname := filepath.Join(g.location, "toberenamed.gradle")
		if err := os.Rename(g.buildFile, tmpgbname); err != nil {
			return "", "", fmt.Errorf("error renaming file %s to %s: %w", g.buildFile, "toberenamed.gradle", err)
		}
		defer os.Rename(tmpgbname, g.buildFile)
		if err := os.Rename(taskgb, g.buildFile); err != nil {
			return "", "", fmt.Errorf("error renaming file %s to %s: %w", taskgb, g.buildFile, err)
		}
		defer os.Remove(g.buildFile)
		args = []string{"konveyorDownloadSources", "--no-daemon"}
	}

	cmd := exec.CommandContext(ctx, g.wrapper, args...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("JAVA_HOME=%s", g.javaHome))
	cmd.Dir = g.location
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("error trying to get sources for Gradle: %w - Gradle output: %s", err, output)
	}

	g.log.V(8).WithValues("output", string(output)).Info("got gradle output")

	// TODO: what if all sources available
	reader := bytes.NewReader(output)
	unresolvedSources, err := g.parseUnresolvedSourcesForGradle(reader)
	if err != nil {
		return "", "", err
	}

	g.log.V(5).Info("total unresolved sources", "count", len(unresolvedSources))
	gradleHome := g.findGradleHome()
	cacheRoot := filepath.Join(gradleHome, "caches", "modules-2")

	if len(unresolvedSources) > 1 {
		// Gradle cache dir structure changes over time - we need to find where the actual dependencies are stored
		cache, err := g.findGradleCache(unresolvedSources[0].GroupId)
		if err != nil {
			return "", "", err
		}
		decompiler, err := getDecompiler(DecompilerOpts{
			DecompileTool:  g.decompileTool,
			log:            g.log,
			workers:        DefaultWorkerPoolSize,
			labler:         g.labeler,
			mavenIndexPath: g.mvnIndexPath,
			m2Repo:         cache,
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
					if resp.err != nil {
						g.log.Error(err, "unable to get java artifact")
						wg.Done()
						continue
					}
					dependencies = append(dependencies, resp.artifact...)
					wg.Done()
				case <-decompilerCtx.Done():
					return
				}
			}
		}()
		for _, artifact := range unresolvedSources {
			g.log.V(5).WithValues("artifact", artifact).Info("sources for artifact not found, decompiling...")

			groupDirs := filepath.Join(strings.Split(artifact.GroupId, ".")...)
			artifactDir := filepath.Join(cache, groupDirs, artifact.Version, artifact.ArtifactId)
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

		wg.Wait()
		cancelFunc()

		return g.location, cache, nil
	}
	return g.location, cacheRoot, nil
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
// 1. Check GRADLE_USER_HOME: https://docs.gradle.org/current/userguide/directory_layout.html#dir:gradle_user_home
// 2. check $GRADLE_HOME
// 3. check $HOME/.gradle
// 4. else, set to /root/.gradle
func (g *gradleResolver) findGradleHome() string {
	gradleHome := os.Getenv("GRADLE_USER_HOME")
	if gradleHome != "" {
		return gradleHome
	}
	gradleHome = os.Getenv("GRADLE_HOME")
	if gradleHome != "" {
		return gradleHome
	}
	home := os.Getenv("HOME")
	if home == "" {
		home = "/root"
	}
	gradleHome = filepath.Join(home, ".gradle")
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
