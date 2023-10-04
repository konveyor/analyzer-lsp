package java

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/engine/labels"
	"github.com/konveyor/analyzer-lsp/provider"
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

type javaArtifact struct {
	packaging  string
	groupId    string
	artifactId string
	version    string
}

type decompileFilter interface {
	shouldDecompile(javaArtifact) bool
}

type alwaysDecompileFilter bool

func (a alwaysDecompileFilter) shouldDecompile(j javaArtifact) bool {
	return bool(a)
}

type excludeOpenSourceDecompileFilter map[string]*depLabelItem

func (o excludeOpenSourceDecompileFilter) shouldDecompile(j javaArtifact) bool {
	matchWith := fmt.Sprintf("%s.%s", j.groupId, j.artifactId)
	for _, r := range o {
		if r.r.MatchString(matchWith) {
			if _, ok := r.labels[labels.AsString(provider.DepSourceLabel, javaDepSourceOpenSource)]; ok {
				return false
			}
		}
	}
	return true
}

type decompileJob struct {
	inputPath  string
	outputPath string
	artifact   javaArtifact
}

func decompile(ctx context.Context, log logr.Logger, filter decompileFilter, workerCount int, jobs []decompileJob, projectPath string) error {
	wg := &sync.WaitGroup{}
	jobChan := make(chan decompileJob)

	// init workers
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		log.V(6).Info("init decompile worker")
		go func(log logr.Logger) {
			defer log.V(6).Info("shutting down decompile worker")
			defer wg.Done()
			for job := range jobChan {
				if _, err := os.Stat(job.outputPath); err == nil {
					// already decompiled, duplicate...
					continue
				}
				outputPathDir := filepath.Dir(job.outputPath)
				if err := os.MkdirAll(outputPathDir, 0755); err != nil {
					log.V(3).Error(err,
						"failed to create directories for decompiled file", "path", outputPathDir)
					continue
				}
				cmd := exec.CommandContext(
					ctx, "java", "-jar", "/bin/fernflower.jar", job.inputPath, outputPathDir)
				err := cmd.Run()
				if err != nil {
					log.V(5).Error(err, "failed to decompile file", "file", job.inputPath, job.outputPath)
				} else {
					log.V(5).Info("decompiled file", "source", job.inputPath, "dest", job.outputPath)
				}
				// if we just decompiled a java archive, we need to
				// explode it further and copy files to project
				if job.artifact.packaging == JavaArchive {
					_, _, err = explode(ctx, log, job.outputPath, projectPath)
					if err != nil {
						log.V(5).Error(err, "failed to explode decompiled jar", "path", job.inputPath)
					}
				}
			}
		}(log.WithName(fmt.Sprintf("decompileWorker-%d", i)))
	}

	for _, job := range jobs {
		jobChan <- job
	}

	close(jobChan)

	wg.Wait()

	return nil
}

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

	decompFilter := alwaysDecompileFilter(true)

	explodedPath, decompJobs, err := explode(ctx, log, archivePath, projectPath)
	if err != nil {
		log.Error(err, "failed to decompile archive", "path", archivePath)
		return "", "", err
	}

	err = decompile(context.TODO(), log, decompFilter, 10, decompJobs, projectPath)
	if err != nil {
		log.Error(err, "failed to decompile", "path", archivePath)
		return "", "", err
	}

	return explodedPath, projectPath, err
}

// explode explodes the given JAR, WAR or EAR archive, generates javaArtifact struct for given archive
// and identifies all .class found recursively. returns output path, a list of decompileJob for .class files
func explode(ctx context.Context, log logr.Logger, archivePath, projectPath string) (string, []decompileJob, error) {
	fileInfo, err := os.Stat(archivePath)
	if err != nil {
		return "", nil, err
	}

	// Create the destDir directory using the same permissions as the Java archive file
	// java.jar should become java-jar-decompiled
	destDir := filepath.Join(path.Dir(archivePath), strings.Replace(path.Base(archivePath), ".", "-", -1)+"-exploded")
	// make sure execute bits are set so that fernflower can decompile
	err = os.MkdirAll(destDir, fileInfo.Mode()|0111)
	if err != nil {
		return "", nil, err
	}

	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", nil, err
	}
	defer archive.Close()

	decompileJobs := []decompileJob{}

	for _, f := range archive.File {
		// Stop processing if our context is cancelled
		select {
		case <-ctx.Done():
			return "", decompileJobs, ctx.Err()
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

		if err = os.MkdirAll(filepath.Dir(filePath), f.Mode()|0111); err != nil {
			return "", decompileJobs, err
		}

		dstFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode()|0111)
		if err != nil {
			return "", decompileJobs, err
		}
		defer dstFile.Close()

		archiveFile, err := f.Open()
		if err != nil {
			return "", decompileJobs, err
		}
		defer archiveFile.Close()

		if _, err := io.Copy(dstFile, archiveFile); err != nil {
			return "", decompileJobs, err
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
			destPath = strings.TrimSuffix(destPath, ClassFile) + ".java"
			// TODO (pgaikwad): pass real artifact for filtering to work correctly
			decompileJobs = append(decompileJobs, decompileJob{
				inputPath:  filePath,
				outputPath: destPath,
				artifact: javaArtifact{
					packaging: ClassFile,
				},
			})
		// when it's a java file, it's already decompiled, copy it to project path
		case strings.HasSuffix(f.Name, JavaFile):
			destPath := filepath.Join(
				projectPath, "src", "main", "java",
				strings.Replace(filePath, destDir, "", -1))
			destPath = strings.ReplaceAll(destPath, "WEB-INF/classes", "")
			destPath = strings.ReplaceAll(destPath, "META-INF/classes", "")
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				log.V(8).Error(err, "error creating directory for java file", "path", destPath)
				continue
			}
			log.V(6).Info("copying file", "src", filePath, "dest", destPath)
			inputFile, err := os.Open(filePath)
			if err != nil {
				log.V(8).Error(err, "failed to open input file", "path", filePath)
				continue
			}
			outputFile, err := os.Create(destPath)
			if err != nil {
				inputFile.Close()
				log.V(8).Error(err, "failed to open output file", "path", destPath)
				continue
			}
			_, err = io.Copy(outputFile, inputFile)
			inputFile.Close()
			if err != nil {
				log.V(8).Error(err, "failed to move java file to project", "src", filePath, "dest", destPath)
				continue
			}
			// The copy was successful, so now delete the original file
			err = os.Remove(filePath)
			if err != nil {
				log.V(8).Error(err, "failed to remove source file", "src", filePath)
				continue
			}
			defer outputFile.Close()
		// decompile web archives
		case strings.HasSuffix(f.Name, WebArchive):
			_, nestedJobs, err := explode(ctx, log, filePath, projectPath)
			if err != nil {
				log.Error(err, "failed to decompile file", "file", filePath)
			}
			decompileJobs = append(decompileJobs, nestedJobs...)
		// nested JARs won't be exploded further, they will be decompiled as whole
		case strings.HasSuffix(f.Name, JavaArchive):
			artifact, err := addProjectDep(ctx, filepath.Join(projectPath, "pom.xml"), filePath)
			if err != nil {
				log.Error(err, "failed to add dep", "file", filePath)
				// when we fail to identify a dep we will fallback to
				// decompiling it ourselves and adding as source
				outputPath := filepath.Join(
					filepath.Dir(filePath), fmt.Sprintf("%s-decompiled",
						strings.TrimSuffix(f.Name, JavaArchive)), filepath.Base(f.Name))
				decompileJobs = append(decompileJobs, decompileJob{
					inputPath:  filePath,
					outputPath: outputPath,
					artifact: javaArtifact{
						packaging:  JavaArchive,
						groupId:    artifact.groupId,
						artifactId: artifact.artifactId,
					},
				})
			}
		}
	}

	return destDir, decompileJobs, nil
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

type project struct {
	XMLName    xml.Name     `xml:"project"`
	Dependency dependencies `xml:"dependencies"`
}

type dependencies struct {
	XMLName    xml.Name     `xml:"dependencies"`
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
