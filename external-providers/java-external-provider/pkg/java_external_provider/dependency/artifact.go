package dependency

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	engine_labels "github.com/konveyor/analyzer-lsp/engine/labels"
	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency/labels"
	"github.com/vifraa/gopom"
)

type JavaArtifact struct {
	FoundOnline bool
	Packaging   string
	GroupId     string
	ArtifactId  string
	Version     string
	Sha1        string
}

func (j JavaArtifact) IsValid() bool {
	return (j.ArtifactId != "" && j.GroupId != "" && j.Version != "")
}

func (j JavaArtifact) EqualsPomDep(dependency gopom.Dependency) bool {
	if dependency.ArtifactID == nil || dependency.GroupID == nil || dependency.Version == nil {
		return false
	}
	if j.ArtifactId == *dependency.ArtifactID && j.GroupId == *dependency.GroupID && j.Version == *dependency.Version {
		return true
	}
	return false
}

func (j JavaArtifact) ToPomDep() gopom.Dependency {
	return gopom.Dependency{
		GroupID:    &j.GroupId,
		ArtifactID: &j.ArtifactId,
		Version:    &j.Version,
	}
}

// toDependency returns javaArtifact constructed for a jar
func ToDependency(_ context.Context, log logr.Logger, labeler labels.Labeler, jarFile string, disableMavenSearch bool) (JavaArtifact, error) {
	if !disableMavenSearch {
		dep, err := constructArtifactFromSHA(log, jarFile)
		if err == nil {
			return dep, nil
		}
		log.V(3).Error(err, "unable to look up dependency by SHA, falling back to get maven cordinates", "jar", jarFile)
	} else {
		log.Info("maven search disabled - looking for dependencies from poms and jar structure")
	}
	dep, err := constructArtifactFromPom(log, jarFile, labeler)
	if err == nil {
		return dep, nil
	}
	log.V(3).Error(err, "could not construct artifact object from pom for artifact, trying to infer from structure", "jarFile", jarFile)

	dep, err = constructArtifactFromStructure(log, jarFile, labeler)
	if err != nil {
		log.V(3).Error(err, "could not construct artifact object from structure", "jarFile", jarFile)
		return JavaArtifact{}, err
	}

	return dep, err
}

var mavenSearchErrorCache error

func constructArtifactFromSHA(log logr.Logger, jarFile string) (JavaArtifact, error) {
	dep := JavaArtifact{}
	// we look up the jar in maven
	file, err := os.Open(jarFile)
	if err != nil {
		return dep, err
	}
	defer file.Close()

	hash := sha1.New()
	_, err = io.Copy(hash, file)
	if err != nil {
		return dep, err
	}

	sha1sum := hex.EncodeToString(hash.Sum(nil))

	// if maven search is down, we do not want to keep trying on each dep
	if mavenSearchErrorCache != nil {
		log.Info("maven search is down, returning cached error", "error", mavenSearchErrorCache)
		return dep, mavenSearchErrorCache
	}

	// Make an HTTPS request to search.maven.org
	searchURL := fmt.Sprintf("https://search.maven.org/solrsearch/select?q=1:%s&rows=20&wt=json", sha1sum)
	resp, err := http.Get(searchURL)
	if err != nil {
		return dep, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		statusErr := fmt.Errorf("Maven search is unavailable: %s", resp.Status)
		// cache the server errors
		if resp.StatusCode >= 500 {
			mavenSearchErrorCache = statusErr
		}
		return dep, statusErr
	}

	// Read and parse the JSON response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return dep, err
	}

	var data map[string]any
	err = json.Unmarshal(body, &data)
	if err != nil {
		return dep, err
	}

	// Check if a single result is found
	response, ok := data["response"].(map[string]any)
	if !ok {
		return dep, err
	}

	numFound, ok := response["numFound"].(float64)
	if !ok {
		return dep, err
	}

	if numFound == 1 {
		jarInfo := response["docs"].([]any)[0].(map[string]any)
		dep.GroupId = jarInfo["g"].(string)
		dep.ArtifactId = jarInfo["a"].(string)
		dep.Version = jarInfo["v"].(string)
		dep.Sha1 = sha1sum
		dep.FoundOnline = true
		return dep, nil
	} else if numFound > 1 {
		return dep, fmt.Errorf("unable to determine maven cordinates, got more then one for jar")
	}
	return dep, fmt.Errorf("failed to construct artifact from maven lookup")
}
func constructArtifactFromPom(log logr.Logger, jarFile string, labeler labels.Labeler) (JavaArtifact, error) {
	log.V(5).Info("trying to find pom within jar %s to get info", jarFile)
	dep := JavaArtifact{}
	jar, err := zip.OpenReader(jarFile)
	if err != nil {
		return dep, err
	}
	defer jar.Close()

	for _, file := range jar.File {
		match, err := filepath.Match("META-INF/maven/*/*/pom.properties", file.Name)
		if err != nil {
			return dep, err
		}

		if match {
			// Open the file in the ZIP archive
			rc, err := file.Open()
			if err != nil {
				return dep, err
			}
			defer rc.Close()

			// Read and process the lines in the properties file
			scanner := bufio.NewScanner(rc)
			for scanner.Scan() {
				line := scanner.Text()
				if after, ok := strings.CutPrefix(line, "version="); ok {
					dep.Version = strings.TrimSpace(after)
				} else if after0, ok0 := strings.CutPrefix(line, "artifactId="); ok0 {
					dep.ArtifactId = strings.TrimSpace(after0)
				} else if after1, ok1 := strings.CutPrefix(line, "groupId="); ok1 {
					dep.GroupId = strings.TrimSpace(after1)
				}
			}
			// Setting false here because we don't know if it is opensource or not.
			depName := fmt.Sprintf("%s.%s", dep.GroupId, dep.ArtifactId)
			groupIdRegex := strings.Join([]string{dep.GroupId, "*"}, ".")
			if labeler.HasLabel(groupIdRegex) {
				dep.FoundOnline = true
			}
			l := labeler.AddLabels(depName, dep.FoundOnline)
			for _, l := range l {
				if l == engine_labels.AsString(labels.DepSourceLabel, labels.JavaDepSourceOpenSource) {
					// Setting here to make things easier.
					dep.FoundOnline = true
					break
				}
				if l == engine_labels.AsString(labels.DepSourceLabel, labels.JavaDepSourceInternal) {
					break
				}
			}
			return dep, err
		}
	}
	return dep, fmt.Errorf("failed to construct artifact from pom properties")
}

// constructArtifactFromStructure builds an artifact object out of the JAR internal structure.
func constructArtifactFromStructure(log logr.Logger, jarFile string, labeler labels.Labeler) (JavaArtifact, error) {
	log.V(10).Info(fmt.Sprintf("trying to infer if %s is a public dependency", jarFile))
	groupId, err := inferGroupName(jarFile)
	if err != nil {
		return JavaArtifact{}, err
	}
	// since the extracted groupId is not reliable, lets just name the dependency after its filename
	artifact := JavaArtifact{ArtifactId: filepath.Base(jarFile)}
	// check the inferred groupId against list of public groups
	// if groupId is not found, remove last segment. repeat if not found until no segments are left.
	sgmts := strings.Split(groupId, ".")
	for len(sgmts) > 0 {
		// check against depToLabels. add *
		groupIdRegex := strings.Join([]string{groupId, "*"}, ".")
		if labeler.HasLabel(groupIdRegex) {
			log.V(10).Info(fmt.Sprintf("%s is a public dependency with a group id of: %s", jarFile, groupId))
			// do a best effort to set some dependency data
			artifact.GroupId = groupId
			artifact.ArtifactId = strings.TrimSuffix(filepath.Base(jarFile), ".jar")
			artifact.Version = "Unknown"
			// Adding this back to make some things easier.
			artifact.FoundOnline = true
			return artifact, nil
		} else {
			// lets try to remove one segment from the end
			sgmts = sgmts[:len(sgmts)-1]
			groupId = strings.Join(sgmts, ".")
		}
	}
	log.V(10).Info(fmt.Sprintf("could not find groupId for in our public listing of group id's for jar: %s", jarFile))
	return artifact, nil
}

// inferGroupName tries to extract the name of the group based on the directory structure.
// Usually group names coincide with package names, this is, the dir structure
// We go down the dir structure until we find either more than one dir, or a file that is not a dir
func inferGroupName(jarPath string) (string, error) {
	r, err := zip.OpenReader(jarPath)
	if err != nil {
		return "", fmt.Errorf("failed to open JAR file: %w", err)
	}
	defer r.Close()

	var classPaths []string
	for _, file := range r.File {
		// Skip entries that aren't .class files
		if !strings.HasSuffix(file.Name, ".class") {
			continue
		}

		// Skip inner or anonymous classes
		if strings.Contains(path.Base(file.Name), "$") {
			continue
		}

		// Skip top-level class files (no package)
		if !strings.Contains(file.Name, "/") {
			continue
		}

		// Skip known metadata paths
		if strings.HasPrefix(file.Name, "META-INF/") || strings.HasPrefix(file.Name, "BOOT-INF/") {
			continue
		}

		classPaths = append(classPaths, file.Name)
	}

	if len(classPaths) == 0 {
		return "", fmt.Errorf("no valid class files found in JAR")
	}

	// Convert each path to a list of package segments
	var allPaths [][]string
	for _, p := range classPaths {
		dir := path.Dir(p)
		parts := strings.Split(dir, "/")
		allPaths = append(allPaths, parts)
	}

	// Find the longest common prefix
	var groupParts []string
	for i := 0; ; i++ {
		var part string
		for j, segments := range allPaths {
			if i >= len(segments) {
				return strings.Join(groupParts, "."), nil
			}
			if j == 0 {
				part = segments[i]
			} else if segments[i] != part {
				return strings.Join(groupParts, "."), nil
			}
		}
		groupParts = append(groupParts, part)
	}
}

func ToFilePathDependency(_ context.Context, filePath string) (JavaArtifact, error) {
	dep := JavaArtifact{}
	// Move up one level to the artifact. we are assuming that we get the full class file here.
	// For instance the dir /org/springframework/boot/loader/jar/Something.class.
	// in this cass the artificat is: Group: org.springframework.boot.loader, Artifact: Jar
	dir := filepath.Dir(filePath)
	dep.ArtifactId = filepath.Base(dir)
	dep.GroupId = strings.ReplaceAll(filepath.Dir(dir), "/", ".")
	dep.Version = "0.0.0"
	return dep, nil
}
