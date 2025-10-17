package dependency

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/template"

	"github.com/go-logr/logr"
	"github.com/hashicorp/go-version"
	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency/labels"
)

type Resolver interface {
	ResolveSources(ctx context.Context) (string, string, error)
}

type ResolverOptions struct {
	Log                logr.Logger
	Location           string
	DecompileTool      string
	Labeler            labels.Labeler
	LocalRepo          string
	DisableMavenSearch bool
	// for gradle this is the build file
	// for maven this is the settings file
	BuildFile string

	// mvn specifics
	Insecure bool

	// Gradle Specifics
	Version        version.Version
	Wrapper        string
	JavaHome       string
	GradleTaskFile string
}

func contains(artifacts []JavaArtifact, artifactToFind JavaArtifact) bool {
	if len(artifacts) == 0 {
		return false
	}

	for _, artifact := range artifacts {
		if artifact == artifactToFind {
			return true
		}
	}

	return false
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

func createJavaProject(_ context.Context, dir string, dependencies []JavaArtifact) error {
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
