package dependency

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency/labels"
	"github.com/konveyor/analyzer-lsp/tracing"
)

type mavenDependencyResolver struct {
	decompileTool  string
	labeler        labels.Labeler
	localRepo      string
	log            logr.Logger
	settingsFile   string
	insecure       bool
	location       string
	mavenIndexPath string
}

func GetMavenResolver(options ResolverOptions) Resolver {
	return &mavenDependencyResolver{
		localRepo:      options.LocalRepo,
		settingsFile:   options.BuildFile,
		insecure:       options.Insecure,
		location:       options.Location,
		log:            options.Log,
		decompileTool:  options.DecompileTool,
		labeler:        options.Labeler,
		mavenIndexPath: options.MavenIndexPath,
	}
}

func (m *mavenDependencyResolver) ResolveSources(ctx context.Context) (string, string, error) {
	ctx, span := tracing.StartNewSpan(ctx, "resolve-sources")
	defer span.End()

	m.log.Info("resolving dependency sources")

	args := []string{
		"-B",
		"de.qaware.maven:go-offline-maven-plugin:resolve-dependencies",
		"-DdownloadSources",
		"-Djava.net.useSystemProxies=true",
	}
	if m.settingsFile != "" {
		args = append(args, "-s", m.settingsFile)
	}
	if m.insecure {
		args = append(args, "-Dmaven.wagon.http.ssl.insecure=true")
	}
	cmd := exec.CommandContext(ctx, "mvn", args...)
	cmd.Dir = m.location
	mvnOutput, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("maven downloadSources command failed with error %w, maven output: %s", err, string(mvnOutput))
	}

	reader := bytes.NewReader(mvnOutput)
	artifacts, err := m.parseUnresolvedSources(reader)
	if err != nil {
		return "", "", err
	}

	decompiler, err := getDecompiler(DecompilerOpts{
		DecompileTool:  m.decompileTool,
		log:            m.log,
		workers:        DefaultWorkerPoolSize,
		labler:         m.labeler,
		m2Repo:         m.localRepo,
		mavenIndexPath: m.mavenIndexPath,
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
				wg.Done()
				if resp.err != nil {
					m.log.Error(err, "unable to get java artifact")
					continue
				}
				dependencies = append(dependencies, resp.artifact...)
			case <-decompilerCtx.Done():
				return
			}
		}
	}()
	for _, artifact := range artifacts {
		m.log.WithValues("artifact", artifact).Info("sources for artifact not found, decompiling...")

		groupDirs := filepath.Join(strings.Split(artifact.GroupId, ".")...)
		jarName := fmt.Sprintf("%s-%s.jar", artifact.ArtifactId, artifact.Version)
		wg.Add(1)
		m.log.Info("adding to wait group")
		go func() {
			artifact, err := decompiler.Decompile(decompilerCtx, filepath.Join(m.localRepo, groupDirs, artifact.ArtifactId, artifact.Version, jarName))
			returnChan <- struct {
				artifact []JavaArtifact
				err      error
			}{artifact: artifact, err: err}
		}()
	}
	m.log.Info("wating in resolver")
	wg.Wait()
	m.log.Info("finished waiting in resolver")
	cancelFunc()

	return m.location, m.localRepo, nil
}

// parseUnresolvedSources takes the output from the go-offline maven plugin and returns the artifacts whose sources
// could not be found.
func (m *mavenDependencyResolver) parseUnresolvedSources(output io.Reader) ([]JavaArtifact, error) {
	unresolvedSources := []JavaArtifact{}
	unresolvedArtifacts := []JavaArtifact{}

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
					artifact := JavaArtifact{
						Packaging:  JavaArchive,
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
				artifact := JavaArtifact{
					Packaging:  JavaArchive,
					GroupId:    gav[1],
					ArtifactId: gav[2],
					Version:    v,
				}

				unresolvedSources = append(unresolvedSources, artifact)
			}
		}
	}

	// if we don't have the dependency itself available, we can't even decompile
	result := []JavaArtifact{}
	for _, artifact := range unresolvedSources {
		if contains(unresolvedArtifacts, artifact) || contains(result, artifact) {
			continue
		}
		result = append(result, artifact)
	}

	return result, scanner.Err()
}
