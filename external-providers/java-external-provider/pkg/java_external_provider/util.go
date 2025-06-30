package java

import (
	"archive/zip"
	"bufio"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"text/template"
	"time"

	"math/rand"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/tracing"
	"go.opentelemetry.io/otel/attribute"
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
func toDependency(_ context.Context, log logr.Logger, depToLabels map[string]*depLabelItem, jarFile string) (javaArtifact, error) {
	//dep, err := constructArtifactFromSHA(jarFile)
	//if err == nil {
	//	return dep, nil
	//}
	// if we fail to lookup on maven, construct it from pom
	dep, err := constructArtifactFromPom(log, jarFile)
	if err != nil {
		log.V(10).Info("could not construct artifact object from pom for artifact", "jarFile", jarFile)
	}

	dep, err = constructArtifactFromStructure(log, jarFile, depToLabels)
	if err != nil {
		return javaArtifact{}, err
	}

	return dep, err
}

func constructArtifactFromPom(log logr.Logger, jarFile string) (javaArtifact, error) {
	log.V(5).Info("trying to find pom within jar %s to get info", jarFile)
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

// constructArtifactFromStructure builds an artifact object out of the JAR internal structure.
func constructArtifactFromStructure(log logr.Logger, jarFile string, depToLabels map[string]*depLabelItem) (javaArtifact, error) {
	log.V(10).Info(fmt.Sprintf("trying to infer if %s is a public dependency", jarFile))
	groupId, err := inferGroupName(jarFile)
	if err != nil {
		return javaArtifact{}, err
	}
	artifact := javaArtifact{GroupId: groupId}
	// check the inferred groupId against list of public groups
	// if groupId is not found, remove last segment. repeat if not found until no segments are left.
	sgmts := strings.Split(groupId, ".")
	for len(sgmts) > 0 {
		// check against depToLabels. add *
		groupIdRegex := strings.Join([]string{groupId, "*"}, ".")
		if depToLabels[groupIdRegex] != nil {
			log.V(10).Info(fmt.Sprintf("%s is a public dependency", jarFile))
			artifact.foundOnline = true
			return artifact, nil
		} else {
			// lets try to remove one segment from the end
			sgmts = sgmts[:len(sgmts)-1]
			groupId = strings.Join(sgmts, ".")
			groupIdRegex = strings.Join([]string{groupId, "*"}, ".")
		}
	}
	log.V(10).Info(fmt.Sprintf("could not decide whether %s is public, setting as private", jarFile))
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

//func constructArtifactFromSHA(jarFile string) (javaArtifact, error) {
//	dep := javaArtifact{}
//	// we look up the jar in maven
//	file, err := os.Open(jarFile)
//	if err != nil {
//		return dep, err
//	}
//	defer file.Close()
//
//	hash := sha1.New()
//	_, err = io.Copy(hash, file)
//	if err != nil {
//		return dep, err
//	}
//
//	sha1sum := hex.EncodeToString(hash.Sum(nil))
//
//	// Make an HTTPS request to search.maven.org
//	searchURL := fmt.Sprintf("https://search.maven.org/solrsearch/select?q=1:%s&rows=20&wt=json", sha1sum)
//	resp, err := http.Get(searchURL)
//	if err != nil {
//		return dep, err
//	}
//	defer resp.Body.Close()
//
//	// Read and parse the JSON response
//	body, err := io.ReadAll(resp.Body)
//	if err != nil {
//		return dep, err
//	}
//
//	var data map[string]interface{}
//	err = json.Unmarshal(body, &data)
//	if err != nil {
//		return dep, err
//	}
//
//	// Check if a single result is found
//	response, ok := data["response"].(map[string]interface{})
//	if !ok {
//		return dep, err
//	}
//
//	numFound, ok := response["numFound"].(float64)
//	if !ok {
//		return dep, err
//	}
//
//	if numFound == 1 {
//		jarInfo := response["docs"].([]interface{})[0].(map[string]interface{})
//		dep.GroupId = jarInfo["g"].(string)
//		dep.ArtifactId = jarInfo["a"].(string)
//		dep.Version = jarInfo["v"].(string)
//		dep.sha1 = sha1sum
//		dep.foundOnline = true
//		return dep, nil
//	} else if numFound > 1 {
//		dep, err = constructArtifactFromPom(jarFile)
//		if err == nil {
//			dep.foundOnline = true
//			return dep, nil
//		}
//	}
//	return dep, fmt.Errorf("failed to construct artifact from maven lookup")
//}

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
