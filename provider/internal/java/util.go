package java

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/xml"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/tracing"
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
  </dependencies>

  <build>
  </build>
</project>
`

// decompileJava unpacks archive at archivePath, decompiles all .class files in it
// creates new java project and puts the java files in the tree of the project
// returns path to exploded archive, path to java project, and an error when encountered
func decompileJava(ctx context.Context, log logr.Logger, archivePath string) (explodedPath, projectPath string, err error) {
	ctx, span := tracing.StartNewSpan(ctx, "decompile")
	defer span.End()

	projectPath = filepath.Join(filepath.Dir(archivePath), "java-project")

	err = createJavaProject(ctx, projectPath)
	if err != nil {
		log.Error(err, "failed to create java project", "path", projectPath)
		return "", "", err
	}
	log.V(5).Info("created java project", "path", projectPath)

	explodedPath, err = decompile(ctx, log, archivePath, projectPath)
	if err != nil {
		log.Error(err, "failed to decompile archive", "path", archivePath)
		return "", "", err
	}
	return explodedPath, projectPath, err
}

// decompile is a function that extracts the contents of a Java archive (.jar|.war|.ear) and
// decompiles any .class files found using the fernflower decompiler into java project location
// maintaining the tree. swallows decomp and copy errors, returns others
func decompile(ctx context.Context, log logr.Logger, archivePath, projectPath string) (string, error) {
	fileInfo, err := os.Stat(archivePath)
	if err != nil {
		return "", err
	}
	// Create the destDir directory using the same permissions as the Java archive file
	// java.jar should become java-jar-decompiled
	destDir := filepath.Join(path.Dir(archivePath), strings.Replace(path.Base(archivePath), ".", "-", -1)+"-decompiled")
	// make sure execute bits are set so that fernflower can decompile
	err = os.MkdirAll(destDir, fileInfo.Mode()|0111)
	if err != nil {
		return "", err
	}

	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer archive.Close()

	for _, f := range archive.File {
		// Stop processing if our context is cancelled
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		filePath := filepath.Join(destDir, f.Name)

		// fernflower already deemed this unparsable, skip...
		if strings.Contains(f.Name, "unparsable") || strings.Contains(f.Name, "NonParsable") {
			log.V(8).Info("unable to parse file", "file", filePath)
			continue
		}

		if f.FileInfo().IsDir() {
			// make sure execute bits are set so that fernflower can decompile
			os.MkdirAll(filePath, f.Mode()|0111)
			continue
		}

		if err = os.MkdirAll(filepath.Dir(filePath), f.Mode()); err != nil {
			return "", err
		}

		dstFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return "", err
		}
		defer dstFile.Close()

		archiveFile, err := f.Open()
		if err != nil {
			return "", err
		}
		defer archiveFile.Close()

		if _, err := io.Copy(dstFile, archiveFile); err != nil {
			return "", err
		}
		switch {
		// when it's a .class file, decompile it into java project
		case strings.HasSuffix(f.Name, ClassFile):
			// full path in the java project for the decompd file
			destPath := filepath.Join(
				projectPath, "src", "main", "java",
				strings.Replace(filePath, destDir, "", -1))
			destPath = strings.ReplaceAll(destPath, "WEB-INF/classes", "")
			destPath = strings.ReplaceAll(destPath, "META-INF/classes", "")
			if _, err = os.Stat(destPath); err == nil {
				// already decompiled, duplicate...
				continue
			}
			if err = os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				log.V(8).Error(err, "failed to create directories for decompiled file", "path", filepath.Dir(destPath))
				continue
			}
			cmd := exec.CommandContext(
				ctx, "java", "-jar", "/bin/fernflower.jar", filePath, path.Dir(destPath))
			err := cmd.Run()
			if err != nil {
				log.Error(err, "failed to decompile file", "file", filePath)
			} else {
				log.V(8).Info("decompiled file", "file", filePath)
			}
		// decompile web archives
		case strings.HasSuffix(f.Name, WebArchive):
			if _, err := decompile(ctx, log, filePath, projectPath); err != nil {
				log.Error(err, "failed to decompile file", "file", filePath)
			}
		case strings.HasSuffix(f.Name, JavaArchive):
			artifact, err := addProjectDep(ctx, filepath.Join(projectPath, "pom.xml"), f.Name)
			if err != nil {
				log.Error(err, "failed to add dep", "file", filePath)
			}
		}
	}

	return destDir, nil
}

func createJavaProject(ctx context.Context, dir string) error {
	err := os.MkdirAll(filepath.Join(dir, "src", "main", "java"), 0755)
	if err != nil {
		return err
	}
	err = os.WriteFile(filepath.Join(dir, "pom.xml"), []byte(javaProjectPom), 0755)
	if err != nil {
		return err
	}
	return nil
}

type javaArtifact struct {
	packaging  string
	groupId    string
	artifactId string
	version    string
}

type project struct {
	XMLName    xml.Name    `xml:"project"`
	Dependency dependencies `xml:"dependencies"`
}

type dependencies struct {
	XMLName    xml.Name    `xml:"dependencies"`
	Dependency []dependency `xml:"dependency"`
}

type dependency struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
}

func addProjectDep(ctx context.Context, pomFile string, jarFile string) (javaArtifact, error) {
	dep := javaArtifact{}
	jar, err := zip.OpenReader(jarFile)
	if err != nil {
		return dep, err
	}
	defer jar.Close()

	for _, jarFile := range jar.File {
		match, err := filepath.Match("META-INF/maven/*/*/pom.properties", jarFile.Name)
		if err != nil {
			return dep, err
		}

		if match {
			// Open the file in the ZIP archive
			rc, err := jarFile.Open()
			if err != nil {
				return dep, err
			}
			defer rc.Close()

			// Read and process the lines in the properties file
			scanner := bufio.NewScanner(rc)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "version=") {
					dep.version = strings.TrimSpace(strings.TrimPrefix(line, "version="))
				} else if strings.HasPrefix(line, "artifactId=") {
					dep.artifactId = strings.TrimSpace(strings.TrimPrefix(line, "artifactId="))
				} else if strings.HasPrefix(line, "groupId=") {
					dep.groupId = strings.TrimSpace(strings.TrimPrefix(line, "groupId="))
				}
			}

			// Read the pom
			content, err := os.ReadFile(pomFile)
			if err != nil {
				return dep, err
			}

			// Unmarshal the existing pom.xml content into a Project struct
			var project project
			if err := xml.Unmarshal(content, &project); err != nil {
				return dep, err
			}
			project.Dependency.Dependency = append(project.Dependency.Dependency, dependency{GroupID: dep.groupId, ArtifactID: dep.artifactId, Version: dep.version})
			// Marshal the modified Project struct back to XML
			updatedXML, err := xml.MarshalIndent(project, "", "  ")
			if err != nil {
				return dep, err
			}

			err = os.WriteFile(pomFile, updatedXML, 0755)
			return dep, err
		}
	}

	return dep, err
}
