package java

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/tracing"
	"go.opentelemetry.io/otel/attribute"
)

// decompile decompiles files submitted via a list of decompileJob concurrently
// if a .class file is encountered, it will be decompiled to output path right away
// if a .jar file is encountered, it will be decompiled as a whole, then exploded to project path
func decompile(ctx context.Context, log logr.Logger, filter decompileFilter, workerCount int, jobs []decompileJob, fernflower, projectPath string, depLabels map[string]*depLabelItem) error {
	wg := &sync.WaitGroup{}
	jobChan := make(chan decompileJob)

	workerCount = int(math.Min(float64(len(jobs)), float64(workerCount)))
	// init workers
	for i := 0; i < workerCount; i++ {
		logger := log.WithName(fmt.Sprintf("decompileWorker-%d", i))
		wg.Add(1)
		go func(log logr.Logger, workerId int) {
			defer log.V(6).Info("shutting down decompile worker")
			defer wg.Done()
			log.V(6).Info("init decompile worker")
			for job := range jobChan {
				// TODO (pgaikwad): when we move to external provider, inherit context from parent
				jobCtx, span := tracing.StartNewSpan(ctx, "decomp-job",
					attribute.Key("worker").Int(workerId))
				// apply decompile filter
				if !filter.shouldDecompile(job.artifact) {
					continue
				}
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
				// multiple java versions may be installed - chose $JAVA_HOME one
				java := filepath.Join(os.Getenv("JAVA_HOME"), "bin", "java")
				// -mpm (max processing method) is required to keep decomp time low
				cmd := exec.CommandContext(
					jobCtx, java, "-jar", fernflower, "-mpm=30", job.inputPath, outputPathDir)
				err := cmd.Run()
				if err != nil {
					log.V(5).Error(err, "failed to decompile file", "file", job.inputPath, job.outputPath)
				} else {
					log.V(5).Info("decompiled file", "source", job.inputPath, "dest", job.outputPath)
				}
				// if we just decompiled a java archive, we need to
				// explode it further and copy files to project
				if job.artifact.packaging == JavaArchive && projectPath != "" {
					_, _, _, err = explode(jobCtx, log, job.outputPath, projectPath, job.m2RepoPath, depLabels)
					if err != nil {
						log.V(5).Error(err, "failed to explode decompiled jar", "path", job.inputPath)
					}
				}
				span.End()
				jobCtx.Done()
			}
		}(logger, i)
	}

	seenJobs := map[string]bool{}
	for _, job := range jobs {
		jobKey := fmt.Sprintf("%s-%s", job.inputPath, job.outputPath)
		if _, ok := seenJobs[jobKey]; !ok {
			seenJobs[jobKey] = true
			jobChan <- job
		}
	}

	close(jobChan)

	wg.Wait()

	return nil
}

// decompileJava unpacks archive at archivePath, decompiles all .class files in it
// creates new java project and puts the java files in the tree of the project
// returns path to exploded archive, path to java project, and an error when encountered
func decompileJava(ctx context.Context, log logr.Logger, fernflower, archivePath string, m2RepoPath string, cleanBin bool, depLabels map[string]*depLabelItem) (explodedPath, projectPath string, err error) {
	ctx, span := tracing.StartNewSpan(ctx, "decompile")
	defer span.End()

	// only need random project name if there is not dir cleanup after
	if cleanBin {
		projectPath = filepath.Join(filepath.Dir(archivePath), fmt.Sprintf("java-project-%v", RandomName()))
	} else {
		projectPath = filepath.Join(filepath.Dir(archivePath), "java-project")
	}

	decompFilter := alwaysDecompileFilter(true)

	explodedPath, decompJobs, deps, err := explode(ctx, log, archivePath, projectPath, m2RepoPath, depLabels)
	if err != nil {
		log.Error(err, "failed to decompile archive", "path", archivePath)
		return "", "", err
	}

	err = createJavaProject(ctx, projectPath, deduplicateJavaArtifacts(deps))
	if err != nil {
		log.Error(err, "failed to create java project", "path", projectPath)
		return "", "", err
	}
	log.V(5).Info("created java project", "path", projectPath)

	err = decompile(ctx, log, decompFilter, 10, decompJobs, fernflower, projectPath, depLabels)
	if err != nil {
		log.Error(err, "failed to decompile", "path", archivePath)
		return "", "", err
	}

	return explodedPath, projectPath, err
}

func deduplicateJavaArtifacts(artifacts []javaArtifact) []javaArtifact {
	uniq := []javaArtifact{}
	seen := map[string]bool{}
	for _, a := range artifacts {
		key := fmt.Sprintf("%s-%s-%s%s",
			a.ArtifactId, a.GroupId, a.Version, a.packaging)
		if _, ok := seen[key]; !ok {
			seen[key] = true
			uniq = append(uniq, a)
		}
	}
	return uniq
}

// explodeSimple decompresses a JAR, EAR or WAR file without
func explodeSimple(jarPath, targetDir string) error {
	r, err := zip.OpenReader(jarPath)
	if err != nil {
		return fmt.Errorf("failed to open JAR file: %w", err)
	}
	defer r.Close()

	for _, file := range r.File {
		destPath := filepath.Join(targetDir, file.Name)

		// Prevent Zip Slip vulnerability
		if !strings.HasPrefix(destPath, filepath.Clean(targetDir)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", destPath)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(destPath, os.ModePerm); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", destPath, err)
			}
			continue
		}

		// Make sure parent directory exists
		if err := os.MkdirAll(filepath.Dir(destPath), os.ModePerm); err != nil {
			return fmt.Errorf("failed to create parent directory for %s: %w", destPath, err)
		}

		dstFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			return fmt.Errorf("failed to create file %s: %w", destPath, err)
		}

		srcFile, err := file.Open()
		if err != nil {
			dstFile.Close()
			return fmt.Errorf("failed to open compressed file %s: %w", file.Name, err)
		}

		_, err = io.Copy(dstFile, srcFile)
		dstFile.Close()
		srcFile.Close()

		if err != nil {
			return fmt.Errorf("failed to extract file %s: %w", file.Name, err)
		}
	}

	return nil
}

// explode explodes the given JAR, WAR or EAR archive, generates javaArtifact struct for given archive
// and identifies all .class found recursively. returns output path, a list of decompileJob for .class files
// it also returns a list of any javaArtifact we could interpret from jars
func explode(ctx context.Context, log logr.Logger, archivePath, projectPath string, m2Repo string, depLabels map[string]*depLabelItem) (string, []decompileJob, []javaArtifact, error) {
	var dependencies []javaArtifact
	fileInfo, err := os.Stat(archivePath)
	if err != nil {
		return "", nil, dependencies, err
	}

	// Create the destDir directory using the same permissions as the Java archive file
	// java.jar should become java-jar-exploded
	destDir := filepath.Join(filepath.Dir(archivePath), strings.Replace(filepath.Base(archivePath), ".", "-", -1)+"-exploded")
	// make sure execute bits are set so that fernflower can decompile
	err = os.MkdirAll(destDir, fileInfo.Mode()|0111)
	if err != nil {
		return "", nil, dependencies, err
	}

	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", nil, dependencies, err
	}
	defer archive.Close()

	decompileJobs := []decompileJob{}

	for _, f := range archive.File {
		// Stop processing if our context is cancelled
		select {
		case <-ctx.Done():
			return "", decompileJobs, dependencies, ctx.Err()
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
			err := os.MkdirAll(filePath, f.Mode()|0111)
			if err != nil {
				log.V(5).Error(err, "failed to create directory when exploding the archive", "filePath", filePath)
			}
			continue
		}

		if err = os.MkdirAll(filepath.Dir(filePath), f.Mode()|0111); err != nil {
			return "", decompileJobs, dependencies, err
		}

		dstFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode()|0111)
		if err != nil {
			return "", decompileJobs, dependencies, err
		}
		defer dstFile.Close()

		archiveFile, err := f.Open()
		if err != nil {
			return "", decompileJobs, dependencies, err
		}
		defer archiveFile.Close()

		if _, err := io.Copy(dstFile, archiveFile); err != nil {
			return "", decompileJobs, dependencies, err
		}
		seenDirArtificat := map[string]interface{}{}
		switch {
		// when it's a .class file and it is in the web-inf, decompile it into java project
		// This is the users code.
		case strings.HasSuffix(f.Name, ClassFile) &&
			(strings.Contains(f.Name, "WEB-INF") || strings.Contains(f.Name, "META-INF")):

			// full path in the java project for the decompd file
			destPath := filepath.Join(
				projectPath, "src", "main", "java",
				strings.Replace(filePath, destDir, "", -1))
			destPath = strings.ReplaceAll(destPath, filepath.Join("WEB-INF", "classes"), "")
			destPath = strings.ReplaceAll(destPath, filepath.Join("META-INF", "classes"), "")
			destPath = strings.TrimSuffix(destPath, ClassFile) + ".java"
			decompileJobs = append(decompileJobs, decompileJob{
				inputPath:  filePath,
				outputPath: destPath,
				artifact: javaArtifact{
					packaging: ClassFile,
				},
			})
		// when it's a .class file and it is not in the web-inf, decompile it into java project
		// This is some dependency that is not packaged as dependency.
		case strings.HasSuffix(f.Name, ClassFile) &&
			!(strings.Contains(f.Name, "WEB-INF") || strings.Contains(f.Name, "META-INF")):
			destPath := filepath.Join(
				projectPath, "src", "main", "java",
				strings.Replace(filePath, destDir, "", -1))
			destPath = strings.TrimSuffix(destPath, ClassFile) + ".java"
			decompileJobs = append(decompileJobs, decompileJob{
				inputPath:  filePath,
				outputPath: destPath,
				artifact: javaArtifact{
					packaging: ClassFile,
				},
			})
			if _, ok := seenDirArtificat[filepath.Dir(f.Name)]; !ok {
				dep, err := toFilePathDependency(ctx, f.Name)
				if err != nil {
					log.V(8).Error(err, "error getting dependcy for path", "path", destPath)
					continue
				}
				dependencies = append(dependencies, dep)
				seenDirArtificat[filepath.Dir(f.Name)] = nil
			}
		// when it's a java file, it's already decompiled, move it to project path
		case strings.HasSuffix(f.Name, JavaFile):
			destPath := filepath.Join(
				projectPath, "src", "main", "java",
				strings.Replace(filePath, destDir, "", -1))
			destPath = strings.ReplaceAll(destPath, filepath.Join("WEB-INF", "classes"), "")
			destPath = strings.ReplaceAll(destPath, filepath.Join("META-INF", "classes"), "")
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				log.V(8).Error(err, "error creating directory for java file", "path", destPath)
				continue
			}
			if err := moveFile(filePath, destPath); err != nil {
				log.V(8).Error(err, "error moving decompiled file to project path",
					"src", filePath, "dest", destPath)
				continue
			}
		// decompile web archives
		case strings.HasSuffix(f.Name, WebArchive):
			// TODO(djzager): Should we add these deps to the pom?
			_, nestedJobs, deps, err := explode(ctx, log, filePath, projectPath, m2Repo, depLabels)
			if err != nil {
				log.Error(err, "failed to decompile file", "file", filePath)
			}
			decompileJobs = append(decompileJobs, nestedJobs...)
			dependencies = append(dependencies, deps...)
		// attempt to add nested jars as dependency before decompiling
		case strings.HasSuffix(f.Name, JavaArchive):
			dep, err := toDependency(ctx, log, depLabels, filePath)
			if err != nil {
				log.V(3).Error(err, "failed to add dep", "file", filePath)
				// when we fail to identify a dep we will fallback to
				// decompiling it ourselves and adding as source
				if (dep != javaArtifact{}) {
					outputPath := filepath.Join(
						filepath.Dir(filePath), fmt.Sprintf("%s-decompiled",
							strings.TrimSuffix(f.Name, JavaArchive)), filepath.Base(f.Name))
					decompileJobs = append(decompileJobs, decompileJob{
						inputPath:  filePath,
						outputPath: outputPath,
						artifact: javaArtifact{
							packaging:  JavaArchive,
							GroupId:    dep.GroupId,
							ArtifactId: dep.ArtifactId,
						},
					})
				}
			}
			if (dep != javaArtifact{}) {
				if dep.foundOnline {
					dependencies = append(dependencies, dep)
					// copy this into m2 repo to avoid downloading again
					groupPath := filepath.Join(strings.Split(dep.GroupId, ".")...)
					artifactPath := filepath.Join(strings.Split(dep.ArtifactId, ".")...)
					destPath := filepath.Join(m2Repo, groupPath, artifactPath,
						dep.Version, filepath.Base(filePath))
					if err := CopyFile(filePath, destPath); err != nil {
						log.V(8).Error(err, "failed copying jar to m2 local repo")
					} else {
						log.V(8).Info("copied jar file", "src", filePath, "dest", destPath)
					}
				} else {
					// when it isn't found online, decompile it
					outputPath := filepath.Join(
						filepath.Dir(filePath), fmt.Sprintf("%s-decompiled",
							strings.TrimSuffix(f.Name, JavaArchive)), filepath.Base(f.Name))
					decompileJobs = append(decompileJobs, decompileJob{
						inputPath:  filePath,
						outputPath: outputPath,
						artifact: javaArtifact{
							packaging:  JavaArchive,
							GroupId:    dep.GroupId,
							ArtifactId: dep.ArtifactId,
						},
					})
				}
			}
		// any other files, move to java project as-is
		default:
			baseName := strings.ToValidUTF8(f.Name, "_")
			re := regexp.MustCompile(`[^\w\-\.\\/]+`)
			baseName = re.ReplaceAllString(baseName, "_")
			destPath := filepath.Join(
				projectPath, strings.Replace(filepath.Base(archivePath), ".", "-", -1)+"-exploded", baseName)
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				log.V(8).Error(err, "error creating directory for java file", "path", destPath)
				continue
			}
			if err := moveFile(filePath, destPath); err != nil {
				log.V(8).Error(err, "error moving decompiled file to project path",
					"src", filePath, "dest", destPath)
				continue
			}
		}
	}

	return destDir, decompileJobs, dependencies, nil
}
