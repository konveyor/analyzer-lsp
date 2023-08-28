package java

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
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
