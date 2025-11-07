package bldtool

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency"
	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency/labels"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/vifraa/gopom"
	"go.lsp.dev/uri"
)

// mavenBaseTool provides shared functionality for Maven-based build tools.
// It contains common configuration and methods used by both mavenBuildTool
// and mavenBinaryBuildTool implementations.
//
// This base type handles:
//   - Maven repository configuration and access
//   - Fallback dependency parsing from pom.xml when Maven commands fail
//   - Local repository path management
//   - Artifact labeling (open source vs internal)
//   - Common Maven settings and security options
type mavenBaseTool struct {
	mvnInsecure     bool           // Whether to allow insecure HTTPS connections
	mvnSettingsFile string         // Path to Maven settings.xml file
	mvnLocalRepo    string         // Path to local Maven repository (.m2/repository)
	mavenIndexPath  string         // Path to Maven index for artifact searches
	dependencyPath  string         // Path to dependency configuration file
	log             logr.Logger    // Logger instance for this build tool
	labeler         labels.Labeler // Labeler for identifying dependency types
}

func (m *mavenBaseTool) GetLocalRepoPath() string {
	return m.mvnLocalRepo
}

func (m *mavenBaseTool) GetDependenciesFallback(ctx context.Context, location string) (map[uri.URI][]provider.DepDAGItem, error) {
	deps := []provider.DepDAGItem{}

	pom, err := gopom.Parse(location)
	if err != nil {
		m.log.Error(err, "Analyzing POM", "file", location)
		return nil, err
	}
	m.log.V(10).Info("Analyzing POM",
		"POM", fmt.Sprintf("%s:%s:%s", m.pomCoordinate(pom.GroupID), m.pomCoordinate(pom.ArtifactID), m.pomCoordinate(pom.Version)),
		"error", err)

	// If the pom object is empty then parse failed silently.
	if reflect.DeepEqual(*pom, gopom.Project{}) {
		return nil, nil
	}

	// have to get both <dependencies> and <dependencyManagement> dependencies (if present)
	var pomDeps []gopom.Dependency
	if pom.Dependencies != nil && *pom.Dependencies != nil {
		pomDeps = append(pomDeps, *pom.Dependencies...)
	}
	if pom.DependencyManagement != nil {
		if pom.DependencyManagement.Dependencies != nil {
			pomDeps = append(pomDeps, *pom.DependencyManagement.Dependencies...)
		}
	}

	// add each dependency found
	for _, d := range pomDeps {
		if d.GroupID == nil || d.ArtifactID == nil {
			continue
		}
		dep := provider.Dep{}
		dep.Name = fmt.Sprintf("%s.%s", *d.GroupID, *d.ArtifactID)
		dep.Extras = map[string]any{
			groupIdKey:    *d.GroupID,
			artifactIdKey: *d.ArtifactID,
			pomPathKey:    location,
		}
		if d.Version != nil {
			if strings.Contains(*d.Version, "$") {
				version := strings.TrimSuffix(strings.TrimPrefix(*d.Version, "${"), "}")
				m.log.V(10).Info("Searching for property in properties",
					"property", version,
					"properties", pom.Properties)
				if pom.Properties == nil {
					m.log.Info("Cannot resolve version property value as POM does not have properties",
						"POM", fmt.Sprintf("%s.%s", m.pomCoordinate(pom.GroupID), m.pomCoordinate(pom.ArtifactID)),
						"property", version,
						"dependency", dep.Name)
					dep.Version = version
				} else {
					version = pom.Properties.Entries[version]
					if version != "" {
						dep.Version = version
					}
				}
			} else {
				dep.Version = *d.Version
			}
			if m.mvnLocalRepo != "" && d.ArtifactID != nil && d.GroupID != nil {
				dep.FileURIPrefix = fmt.Sprintf("file://%s", filepath.Join(m.mvnLocalRepo,
					strings.ReplaceAll(*d.GroupID, ".", "/"), *d.ArtifactID, dep.Version))
			}
		}
		dagDep := provider.DepDAGItem{Dep: dep}
		deps = append(deps, dagDep)
	}
	if len(deps) == 0 {
		m.log.V(1).Info("unable to get dependencies from "+dependency.PomXmlFile+" in fallback", "pom", location)
		return nil, nil
	}

	fileToDeps := map[uri.URI][]provider.DepDAGItem{}
	fileToDeps[uri.File(location)] = deps
	// recursively find deps in submodules
	if pom.Modules != nil {
		for _, mod := range *pom.Modules {
			mPath := filepath.Join(filepath.Dir(location), mod, dependency.PomXmlFile)
			moreDeps, err := m.GetDependenciesFallback(ctx, mPath)
			if err != nil {
				return nil, err
			}

			// add found dependencies to map
			for depPath := range moreDeps {
				fileToDeps[depPath] = moreDeps[depPath]
			}
		}
	}

	return fileToDeps, nil
}

func (m *mavenBaseTool) pomCoordinate(value *string) string {
	if value != nil {
		return *value
	}
	return "unknown"
}

func (m *mavenBaseTool) getMavenLocalRepoPath() string {
	args := []string{
		"help:evaluate", "-Dexpression=settings.localRepository", "-q", "-DforceStdout",
	}
	if m.mvnSettingsFile != "" {
		args = append(args, "-s", m.mvnSettingsFile)
	}
	cmd := exec.Command("mvn", args...)
	var outb bytes.Buffer
	cmd.Stdout = &outb
	err := cmd.Run()
	if err != nil {
		return ""
	}

	// check errors
	return outb.String()
}
