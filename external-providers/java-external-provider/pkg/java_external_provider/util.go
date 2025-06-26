package java

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
	"text/template"
	"time"

	"math/rand"
)

const javaProjectPom = `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
  xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 http://maven.apache.org/xsd/maven-4.0.0.xsd">
  <modelVersion>4.0.0</modelVersion>

  <groupId>io.konveyor</groupId>
  <artifactId>java-project</artifactId>
  <version>1.0-SNAPSHOT</version>

  <name>java-project</name>
  <url>http://www.konveyor.io</url>

  <properties>
    <project.build.sourceEncoding>UTF-8</project.build.sourceEncoding>
  </properties>

  <dependencies>
{{range .}}
    <dependency>
      <groupId>{{.GroupId}}</groupId>
      <artifactId>{{.ArtifactId}}</artifactId>
      <version>{{.Version}}</version>
    </dependency>
{{end}}
  </dependencies>

  <build>
  </build>
</project>
`

type javaArtifact struct {
	foundOnline bool
	packaging   string
	GroupId     string
	ArtifactId  string
	Version     string
	sha1        string
}

type decompileFilter interface {
	shouldDecompile(javaArtifact) bool
}

type alwaysDecompileFilter bool

func (a alwaysDecompileFilter) shouldDecompile(j javaArtifact) bool {
	return bool(a)
}

type decompileJob struct {
	inputPath  string
	outputPath string
	artifact   javaArtifact
	m2RepoPath string
}

func createJavaProject(_ context.Context, dir string, dependencies []javaArtifact) error {
	tmpl := template.Must(template.New("javaProjectPom").Parse(javaProjectPom))

	err := os.MkdirAll(filepath.Join(dir, "src", "main", "java"), 0755)
	if err != nil {
		return err
	}

	pom, err := os.OpenFile(filepath.Join(dir, "pom.xml"), os.O_CREATE|os.O_WRONLY, 0755)
	if err != nil {
		return err
	}

	err = tmpl.Execute(pom, dependencies)
	if err != nil {
		return err
	}
	return nil
}

func moveFile(srcPath string, destPath string) error {
	err := CopyFile(srcPath, destPath)
	if err != nil {
		return err
	}
	err = os.Remove(srcPath)
	if err != nil {
		return err
	}
	return nil
}

func CopyFile(srcPath string, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}
	inputFile, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer inputFile.Close()
	outputFile, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer outputFile.Close()
	_, err = io.Copy(outputFile, inputFile)
	if err != nil {
		return err
	}
	return nil
}

func AppendToFile(src string, dst string) error {
	// Read the contents of the source file
	content, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("error reading source file: %s", err)
	}

	// Open the destination file in append mode
	destFile, err := os.OpenFile(dst, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("error opening destination file: %s", err)
	}
	defer destFile.Close()

	// Append the content to the destination file
	_, err = destFile.Write(content)
	if err != nil {
		return fmt.Errorf("error apending to destination file: %s", err)
	}

	return nil
}

// toDependency returns javaArtifact constructed for a jar
func toDependency(_ context.Context, depToLabels map[string]*depLabelItem, jarFile string) (javaArtifact, error) {
	// (!) would it be good to keep using SHA lookup and still have the index lookup?
	// 1. try to find properties from pom
	// attempt to lookup java artifact in maven
	//dep, err := constructArtifactFromSHA(jarFile)
	//if err == nil {
	//	return dep, nil
	//}
	// if we fail to lookup on maven, construct it from pom
	dep, err := constructArtifactFromPom(jarFile)
	if err != nil {
		return javaArtifact{}, err
	}

	foundOnline, err := isJarPublic(jarFile, depToLabels)
	if err != nil {
		return javaArtifact{}, err
	}
	dep.foundOnline = foundOnline

	return dep, err
}

func constructArtifactFromPom(jarFile string) (javaArtifact, error) {
	dep := javaArtifact{}
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
				if strings.HasPrefix(line, "version=") {
					dep.Version = strings.TrimSpace(strings.TrimPrefix(line, "version="))
				} else if strings.HasPrefix(line, "artifactId=") {
					dep.ArtifactId = strings.TrimSpace(strings.TrimPrefix(line, "artifactId="))
				} else if strings.HasPrefix(line, "groupId=") {
					dep.GroupId = strings.TrimSpace(strings.TrimPrefix(line, "groupId="))
				}
			}

			return dep, err
		}
	}
	return dep, fmt.Errorf("failed to construct artifact from pom properties")
}

// isJarPublic tries to infer if a JAR is a public piece of software based on its internal structure
func isJarPublic(jarFile string, depToLabels map[string]*depLabelItem) (bool, error) {
	groupId, err := inferGroupName(jarFile)
	if err != nil {
		return false, err
	}
	// check against depToLabels. add *?
	groupIdRegex := strings.Join([]string{groupId, "*"}, ".")
	if depToLabels[groupIdRegex] != nil {
		return true, nil
	}
	return false, nil
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

	// Store only class file paths that resemble Java packages
	var classPaths []string
	for _, file := range r.File {
		if strings.HasSuffix(file.Name, ".class") && !strings.Contains(file.Name, "META-INF") {
			classPaths = append(classPaths, file.Name)
		}
	}

	if len(classPaths) == 0 {
		return "", fmt.Errorf("no class files found in JAR")
	}

	// Split each path into segments
	var allPaths [][]string
	for _, p := range classPaths {
		dir := path.Dir(p) // remove the class file name
		parts := strings.Split(dir, "/")
		allPaths = append(allPaths, parts)
	}

	// Find common prefix
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

func constructArtifactFromSHA(jarFile string) (javaArtifact, error) {
	dep := javaArtifact{}
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

	// Make an HTTPS request to search.maven.org
	searchURL := fmt.Sprintf("https://search.maven.org/solrsearch/select?q=1:%s&rows=20&wt=json", sha1sum)
	resp, err := http.Get(searchURL)
	if err != nil {
		return dep, err
	}
	defer resp.Body.Close()

	// Read and parse the JSON response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return dep, err
	}

	var data map[string]interface{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return dep, err
	}

	// Check if a single result is found
	response, ok := data["response"].(map[string]interface{})
	if !ok {
		return dep, err
	}

	numFound, ok := response["numFound"].(float64)
	if !ok {
		return dep, err
	}

	if numFound == 1 {
		jarInfo := response["docs"].([]interface{})[0].(map[string]interface{})
		dep.GroupId = jarInfo["g"].(string)
		dep.ArtifactId = jarInfo["a"].(string)
		dep.Version = jarInfo["v"].(string)
		dep.sha1 = sha1sum
		dep.foundOnline = true
		return dep, nil
	} else if numFound > 1 {
		dep, err = constructArtifactFromPom(jarFile)
		if err == nil {
			dep.foundOnline = true
			return dep, nil
		}
	}
	return dep, fmt.Errorf("failed to construct artifact from maven lookup")
}

func toFilePathDependency(_ context.Context, filePath string) (javaArtifact, error) {
	dep := javaArtifact{}
	// Move up one level to the artifact. we are assuming that we get the full class file here.
	// For instance the dir /org/springframework/boot/loader/jar/Something.class.
	// in this cass the artificat is: Group: org.springframework.boot.loader, Artifact: Jar
	dir := filepath.Dir(filePath)
	dep.ArtifactId = filepath.Base(dir)
	dep.GroupId = strings.Replace(filepath.Dir(dir), "/", ".", -1)
	dep.Version = "0.0.0"
	return dep, nil

}

func RandomName() string {
	rand.Seed(int64(time.Now().Nanosecond()))
	charset := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, 16)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}
