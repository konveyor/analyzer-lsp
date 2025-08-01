package java

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/tracing"
)

const (
	maven  = "maven"
	gradle = "gradle"
)

type SourceResolver interface {
	Resolve(context.Context) error
	BuildTool() string
}

type sourceResolver struct {
	fernflower         string
	disableMavenSearch bool
	mvnSettingsFile    string
	mvnLocalRepo       string
	mvnInsecure        bool
	location           string
	log                logr.Logger
	depToLabels        map[string]*depLabelItem
}

type mavenSourceResolver struct {
	sourceResolver
}

var _ SourceResolver = &mavenSourceResolver{}

func (s *mavenSourceResolver) BuildTool() string {
	return maven
}

// resolveSourcesJarsForMaven for a given source code location, runs maven to find
// deps that don't have sources attached and decompiles them
func (s *mavenSourceResolver) Resolve(ctx context.Context) error {
	// TODO (pgaikwad): when we move to external provider, inherit context from parent
	ctx, span := tracing.StartNewSpan(ctx, "resolve-sources")
	defer span.End()

	if s.mvnLocalRepo == "" {
		s.log.V(5).Info("unable to discover dependency sources as maven local repo path is unknown")
		return nil
	}

	decompileJobs := []decompileJob{}

	s.log.Info("resolving dependency sources")

	args := []string{
		"-B",
		"de.qaware.maven:go-offline-maven-plugin:resolve-dependencies",
		"-DdownloadSources",
		"-Djava.net.useSystemProxies=true",
	}
	if s.mvnSettingsFile != "" {
		args = append(args, "-s", s.mvnSettingsFile)
	}
	if s.mvnInsecure {
		args = append(args, "-Dmaven.wagon.http.ssl.insecure=true")
	}
	cmd := exec.CommandContext(ctx, "mvn", args...)
	cmd.Dir = s.location
	mvnOutput, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("maven downloadSources command failed with error %w, maven output: %s", err, string(mvnOutput))
	}

	reader := bytes.NewReader(mvnOutput)
	artifacts, err := s.parseUnresolvedSources(reader)
	if err != nil {
		return err
	}

	for _, artifact := range artifacts {
		s.log.WithValues("artifact", artifact).Info("sources for artifact not found, decompiling...")

		groupDirs := filepath.Join(strings.Split(artifact.GroupId, ".")...)
		artifactDirs := filepath.Join(strings.Split(artifact.ArtifactId, ".")...)
		jarName := fmt.Sprintf("%s-%s.jar", artifact.ArtifactId, artifact.Version)
		decompileJobs = append(decompileJobs, decompileJob{
			artifact: artifact,
			inputPath: filepath.Join(
				s.mvnLocalRepo, groupDirs, artifactDirs, artifact.Version, jarName),
			outputPath: filepath.Join(
				s.mvnLocalRepo, groupDirs, artifactDirs, artifact.Version, "decompiled", jarName),
		})
	}
	err = decompile(ctx, s.log, alwaysDecompileFilter(true), 10, decompileJobs, s.fernflower, "", s.depToLabels, s.disableMavenSearch)
	if err != nil {
		return err
	}
	// move decompiled files to base location of the jar
	for _, decompileJob := range decompileJobs {
		jarName := strings.TrimSuffix(filepath.Base(decompileJob.inputPath), ".jar")
		err = moveFile(decompileJob.outputPath,
			filepath.Join(filepath.Dir(decompileJob.inputPath),
				fmt.Sprintf("%s-sources.jar", jarName)))
		if err != nil {
			s.log.Error(err, "failed to move decompiled file", "file", decompileJob.outputPath)
		}
	}
	return nil
}

// parseUnresolvedSources takes the output from the go-offline maven plugin and returns the artifacts whose sources
// could not be found.
func (s *mavenSourceResolver) parseUnresolvedSources(output io.Reader) ([]javaArtifact, error) {
	unresolvedSources := []javaArtifact{}
	unresolvedArtifacts := []javaArtifact{}

	scanner := bufio.NewScanner(output)

	unresolvedRegex := regexp.MustCompile(`\[WARNING] The following artifacts could not be resolved`)
	artifactRegex := regexp.MustCompile(`([\w\.]+):([\w\-]+):\w+:([\w\.]+):?([\w\.]+)?`)

	for scanner.Scan() {
		line := scanner.Text()

		if unresolvedRegex.Find([]byte(line)) != nil {
			gavs := artifactRegex.FindAllStringSubmatch(line, -1)
			for _, gav := range gavs {
				// dependency jar (not sources) also not found
				if len(gav) == 5 && gav[3] != "sources" {
					artifact := javaArtifact{
						packaging:  JavaArchive,
						GroupId:    gav[1],
						ArtifactId: gav[2],
						Version:    gav[3],
					}
					unresolvedArtifacts = append(unresolvedArtifacts, artifact)
					continue
				}

				var v string
				if len(gav) == 4 {
					v = gav[3]
				} else {
					v = gav[4]
				}
				artifact := javaArtifact{
					packaging:  JavaArchive,
					GroupId:    gav[1],
					ArtifactId: gav[2],
					Version:    v,
				}

				unresolvedSources = append(unresolvedSources, artifact)
			}
		}
	}

	// if we don't have the dependency itself available, we can't even decompile
	result := []javaArtifact{}
	for _, artifact := range unresolvedSources {
		if contains(unresolvedArtifacts, artifact) || contains(result, artifact) {
			continue
		}
		result = append(result, artifact)
	}

	return result, scanner.Err()
}

type gradleSourceResolver struct {
	sourceResolver
	gradleBuild string
}

func (s *gradleSourceResolver) BuildTool() string {
	return gradle
}

func (g *gradleSourceResolver) Resolve(ctx context.Context) error {
	ctx, span := tracing.StartNewSpan(ctx, "resolve-sources")
	defer span.End()

	g.log.V(5).Info("resolving dependency sources for gradle")

	// create a temporary build file to append the task for downloading sources
	taskgb := filepath.Join(filepath.Dir(g.gradleBuild), "tmp.gradle")
	err := CopyFile(g.gradleBuild, taskgb)
	if err != nil {
		return fmt.Errorf("error copying file %s to %s", g.gradleBuild, taskgb)
	}
	defer os.Remove(taskgb)

	// append downloader task
	taskfile := "/root/.gradle/task.gradle"
	err = AppendToFile(taskfile, taskgb)
	if err != nil {
		return fmt.Errorf("error appending file %s to %s", taskfile, taskgb)
	}

	tmpgbname := filepath.Join(g.location, "toberenamed.gradle")
	err = os.Rename(g.gradleBuild, tmpgbname)
	if err != nil {
		return fmt.Errorf("error renaming file %s to %s", g.gradleBuild, "toberenamed.gradle")
	}
	defer os.Rename(tmpgbname, g.gradleBuild)

	err = os.Rename(taskgb, g.gradleBuild)
	if err != nil {
		return fmt.Errorf("error renaming file %s to %s", g.gradleBuild, "toberenamed.gradle")
	}
	defer os.Remove(g.gradleBuild)

	// run gradle wrapper with tmp build file
	exe, err := filepath.Abs(filepath.Join(g.location, "gradlew"))
	if err != nil {
		return fmt.Errorf("error calculating gradle wrapper path")
	}
	if _, err = os.Stat(exe); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("a gradle wrapper must be present in the project")
	}

	// gradle must run with java 8 (see compatibility matrix)
	java8home := os.Getenv("JAVA8_HOME")
	if java8home == "" {
		return fmt.Errorf("")
	}

	args := []string{
		"konveyorDownloadSources",
	}
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Env = append(cmd.Env, fmt.Sprintf("JAVA_HOME=%s", java8home))
	cmd.Dir = g.location
	output, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}

	g.log.V(8).WithValues("output", output).Info("got gradle output")

	// TODO: what if all sources available
	reader := bytes.NewReader(output)
	unresolvedSources, err := g.parseUnresolvedSourcesForGradle(reader)
	if err != nil {
		return err
	}

	g.log.V(5).Info("total unresolved sources", "count", len(unresolvedSources))

	decompileJobs := []decompileJob{}
	if len(unresolvedSources) > 1 {
		// Gradle cache dir structure changes over time - we need to find where the actual dependencies are stored
		cache, err := g.findGradleCache(unresolvedSources[0].GroupId)
		if err != nil {
			return err
		}

		for _, artifact := range unresolvedSources {
			g.log.V(5).WithValues("artifact", artifact).Info("sources for artifact not found, decompiling...")

			artifactDir := filepath.Join(cache, artifact.GroupId, artifact.ArtifactId)
			jarName := fmt.Sprintf("%s-%s.jar", artifact.ArtifactId, artifact.Version)
			artifactPath, err := g.findGradleArtifact(artifactDir, jarName)
			if err != nil {
				return err
			}
			decompileJobs = append(decompileJobs, decompileJob{
				artifact:   artifact,
				inputPath:  artifactPath,
				outputPath: filepath.Join(filepath.Dir(artifactPath), "decompiled", jarName),
			})
		}
		err = decompile(ctx, g.log, alwaysDecompileFilter(true), 10, decompileJobs, g.fernflower, "", g.depToLabels, g.disableMavenSearch)
		if err != nil {
			return err
		}
		// move decompiled files to base location of the jar
		for _, decompileJob := range decompileJobs {
			jarName := strings.TrimSuffix(filepath.Base(decompileJob.inputPath), ".jar")
			err = moveFile(decompileJob.outputPath,
				filepath.Join(filepath.Dir(decompileJob.inputPath),
					fmt.Sprintf("%s-sources.jar", jarName)))
			if err != nil {
				g.log.V(5).Error(err, "failed to move decompiled file", "file", decompileJob.outputPath)
			}
		}

	}
	return nil
}

// findGradleCache looks for the folder within the Gradle cache where the actual dependencies are stored
// by walking the cache directory looking for a directory equal to the given sample group id
func (g *gradleSourceResolver) findGradleCache(sampleGroupId string) (string, error) {
	// TODO(jmle): atm taking for granted that the cache is going to be here
	root := "/root/.gradle/caches"
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
	err := filepath.WalkDir(root, walker)
	if err != nil {
		return "", err
	}
	cache = filepath.Dir(cache) // return the parent of the found directory
	return cache, nil
}

// findGradleArtifact looks for a given artifact jar within the given root dir
func (g *gradleSourceResolver) findGradleArtifact(root string, artifactId string) (string, error) {
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
func (g *gradleSourceResolver) parseUnresolvedSourcesForGradle(output io.Reader) ([]javaArtifact, error) {
	unresolvedSources := []javaArtifact{}
	unresolvedRegex := regexp.MustCompile(`Found 0 sources for (.*)`)
	artifactRegex := regexp.MustCompile(`(.+):(.+):(.+)|:(.+):`)

	scanner := bufio.NewScanner(output)
	for scanner.Scan() {
		line := scanner.Text()

		if match := unresolvedRegex.FindStringSubmatch(line); len(match) != 0 {
			gav := artifactRegex.FindStringSubmatch(match[1])
			if gav[4] != "" { // internal library, unknown group/version
				artifact := javaArtifact{
					ArtifactId: match[4],
				}
				unresolvedSources = append(unresolvedSources, artifact)
			} else { // external dependency
				artifact := javaArtifact{
					GroupId:    gav[1],
					ArtifactId: gav[2],
					Version:    gav[3],
				}
				unresolvedSources = append(unresolvedSources, artifact)
			}
		}
	}

	// dedup artifacts
	result := []javaArtifact{}
	for _, artifact := range unresolvedSources {
		if contains(result, artifact) {
			continue
		}
		result = append(result, artifact)
	}

	return result, scanner.Err()
}

func getSourceResolver(args sourceArgs) SourceResolver {
	sResolver := sourceResolver{
		fernflower:         args.fernflower,
		disableMavenSearch: args.disableMavenSearch,
		mvnSettingsFile:    args.mavenSettingsFiles,
		mvnLocalRepo:       args.m2Repo,
		mvnInsecure:        args.mvnInsecure,
		location:           args.location,
		log:                args.log,
		depToLabels:        args.openSourceDepLabels,
	}

	if bf := findPom(args.dependencyPath, args.location); bf != "" {
		return &mavenSourceResolver{sourceResolver: sResolver}
	}
	if bf := findGradleBuild(args.location); bf != "" {
		return &gradleSourceResolver{}
	}
	return nil
}

// TODO implement this for real
func findPom(depPath, location string) string {
	f, err := filepath.Abs(filepath.Join(location, depPath))
	if err != nil {
		return ""
	}
	if _, err := os.Stat(f); errors.Is(err, os.ErrNotExist) {
		return ""
	}
	return f
}

func findGradleBuild(location string) string {
	if location != "" {
		path := filepath.Join(location, "build.gradle")
		_, err := os.Stat(path)
		if err != nil {
			return ""
		}
		f, err := filepath.Abs(path)
		if err != nil {
			return ""
		}
		return f
	}
	return ""
}
