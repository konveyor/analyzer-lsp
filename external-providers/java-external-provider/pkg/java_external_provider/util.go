package java

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
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

const EMBEDDED_KONVEYOR_GROUP = "io.konveyor.embeddedep"

type javaArtifact struct {
	foundOnline bool
	packaging   string
	GroupId     string
	ArtifactId  string
	Version     string
	sha1        string
}

func (j javaArtifact) isValid() bool {
	return (j.ArtifactId != "" && j.GroupId != "" && j.Version != "")
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
func decompile(ctx context.Context, log logr.Logger, filter decompileFilter, workerCount int, jobs []decompileJob, fernflower, projectPath string, mavenIndexPath string) error {
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
					_, _, _, err = explode(jobCtx, log, job.outputPath, projectPath, job.m2RepoPath, mavenIndexPath)
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
func decompileJava(ctx context.Context, log logr.Logger, fernflower, archivePath string, m2RepoPath string, cleanBin bool, mavenIndexPath string) (explodedPath, projectPath string, err error) {
	ctx, span := tracing.StartNewSpan(ctx, "decompile")
	defer span.End()

	// only need random project name if there is not dir cleanup after
	if cleanBin {
		projectPath = filepath.Join(filepath.Dir(archivePath), fmt.Sprintf("java-project-%v", RandomName()))
	} else {
		projectPath = filepath.Join(filepath.Dir(archivePath), "java-project")
	}

	decompFilter := alwaysDecompileFilter(true)

	explodedPath, decompJobs, deps, err := explode(ctx, log, archivePath, projectPath, m2RepoPath, mavenIndexPath)
	if err != nil {
		log.Error(err, "failed to decompile archive", "path", archivePath)
		return "", "", err
	}

	err = createJavaProject(ctx, projectPath, removeIncompleteDependencies(deduplicateJavaArtifacts(deps)))
	if err != nil {
		log.Error(err, "failed to create java project", "path", projectPath)
		return "", "", err
	}
	log.V(5).Info("created java project", "path", projectPath)

	err = decompile(ctx, log, decompFilter, 10, decompJobs, fernflower, projectPath, mavenIndexPath)
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

func removeIncompleteDependencies(dependencies []javaArtifact) []javaArtifact {
	complete := []javaArtifact{}
	for _, dep := range dependencies {
		if dep.ArtifactId != "" && dep.GroupId != "" && dep.Version != "" {
			complete = append(complete, dep)
		}
	}
	return complete
}

// explode explodes the given JAR, WAR or EAR archive, generates javaArtifact struct for given archive
// and identifies all .class found recursively. returns output path, a list of decompileJob for .class files
// it also returns a list of any javaArtifact we could interpret from jars
func explode(ctx context.Context, log logr.Logger, archivePath, projectPath string, m2Repo string, mvnIndexPath string) (string, []decompileJob, []javaArtifact, error) {
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

		explodedFilePath := filepath.Join(destDir, f.Name)

		// fernflower already deemed this unparsable, skip...
		if strings.Contains(f.Name, "unparsable") || strings.Contains(f.Name, "NonParsable") {
			log.V(8).Info("unable to parse file", "file", explodedFilePath)
			continue
		}

		if f.FileInfo().IsDir() {
			// make sure execute bits are set so that fernflower can decompile
			err := os.MkdirAll(explodedFilePath, f.Mode()|0111)
			if err != nil {
				log.V(5).Error(err, "failed to create directory when exploding the archive", "filePath", explodedFilePath)
			}
			continue
		}

		if err = os.MkdirAll(filepath.Dir(explodedFilePath), f.Mode()|0111); err != nil {
			return "", decompileJobs, dependencies, err
		}

		dstFile, err := os.OpenFile(explodedFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode()|0111)
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
				strings.Replace(explodedFilePath, destDir, "", -1))
			destPath = strings.ReplaceAll(destPath, filepath.Join("WEB-INF", "classes"), "")
			destPath = strings.ReplaceAll(destPath, filepath.Join("META-INF", "classes"), "")
			destPath = strings.TrimSuffix(destPath, ClassFile) + ".java"
			decompileJobs = append(decompileJobs, decompileJob{
				inputPath:  explodedFilePath,
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
				strings.Replace(explodedFilePath, destDir, "", -1))
			destPath = strings.TrimSuffix(destPath, ClassFile) + ".java"
			decompileJobs = append(decompileJobs, decompileJob{
				inputPath:  explodedFilePath,
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
				strings.Replace(explodedFilePath, destDir, "", -1))
			destPath = strings.ReplaceAll(destPath, filepath.Join("WEB-INF", "classes"), "")
			destPath = strings.ReplaceAll(destPath, filepath.Join("META-INF", "classes"), "")
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				log.V(8).Error(err, "error creating directory for java file", "path", destPath)
				continue
			}
			if err := moveFile(explodedFilePath, destPath); err != nil {
				log.V(8).Error(err, "error moving decompiled file to project path",
					"src", explodedFilePath, "dest", destPath)
				continue
			}
		// decompile web archives
		case strings.HasSuffix(f.Name, WebArchive):
			// TODO(djzager): Should we add these deps to the pom?
			_, nestedJobs, deps, err := explode(ctx, log, explodedFilePath, projectPath, m2Repo, mvnIndexPath)
			if err != nil {
				log.Error(err, "failed to decompile file", "file", explodedFilePath)
			}
			decompileJobs = append(decompileJobs, nestedJobs...)
			dependencies = append(dependencies, deps...)
		// attempt to add nested jars as dependency before decompiling
		case strings.HasSuffix(f.Name, JavaArchive):
			dep, err := toDependency(ctx, log, explodedFilePath, mvnIndexPath)
			if err != nil {
				log.Error(err, "failed to add dep", "file", explodedFilePath)
				// when we fail to identify a dep we will fallback to
				// decompiling it ourselves and adding as source
				continue
			}
			if !dep.isValid() {
				log.Info("failed to create maven coordinates -- using file to create dummy values", "file", explodedFilePath)
				name, _ := strings.CutSuffix(filepath.Base(explodedFilePath), ".jar")
				newDep := javaArtifact{
					foundOnline: false,
					packaging:   "",
					GroupId:     EMBEDDED_KONVEYOR_GROUP,
					ArtifactId:  name,
					Version:     "0.0.0-SNAPSHOT",
					sha1:        "",
				}
				dependencies = append(dependencies, newDep)
				gropupPath := filepath.Join(strings.Split(EMBEDDED_KONVEYOR_GROUP, ".")...)
				destPath := filepath.Join(m2Repo, gropupPath, name, "0.0.0-SNAPSHOT", fmt.Sprintf("%s-%s.jar", newDep.ArtifactId, newDep.Version))
				if err := CopyFile(explodedFilePath, destPath); err != nil {
					log.Error(err, "failed copying jar to m2 local repo")
					continue
				}
				log.Info("copied jar file", "src", explodedFilePath, "dest", destPath)
				continue
			}

			if dep.foundOnline {
				log.Info("determined that dependency is avaliable in maven central", "dep", dep)
				dependencies = append(dependencies, dep)
				// copy this into m2 repo to avoid downloading again
				groupPath := filepath.Join(strings.Split(dep.GroupId, ".")...)
				artifactPath, _ := strings.CutSuffix(filepath.Base(explodedFilePath), ".jar")
				destPath := filepath.Join(m2Repo, groupPath, artifactPath,
					dep.Version, filepath.Base(explodedFilePath))
				if err := CopyFile(explodedFilePath, destPath); err != nil {
					log.Error(err, "failed copying jar to m2 local repo")
					continue
				}
				log.Info("copied jar file", "src", explodedFilePath, "dest", destPath)
				continue
			}
			// when it isn't found online, decompile it
			log.Info("decompiling and adding to source because we can't determine if it is avalable in maven central", "file", f.Name)
			outputPath := filepath.Join(
				filepath.Dir(explodedFilePath), fmt.Sprintf("%s-decompiled",
					strings.TrimSuffix(f.Name, JavaArchive)), filepath.Base(f.Name))
			decompileJobs = append(decompileJobs, decompileJob{
				inputPath:  explodedFilePath,
				outputPath: outputPath,
				artifact: javaArtifact{
					packaging:  JavaArchive,
					GroupId:    dep.GroupId,
					ArtifactId: dep.ArtifactId,
				},
			})
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
			if err := moveFile(explodedFilePath, destPath); err != nil {
				log.V(8).Error(err, "error moving decompiled file to project path",
					"src", explodedFilePath, "dest", destPath)
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

func toDependency(_ context.Context, log logr.Logger, jarFile string, indexPath string) (javaArtifact, error) {
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

	dataFilePath := filepath.Join(indexPath, "maven-index.txt")
	indexFilePath := filepath.Join(indexPath, "maven-index.idx")
	dep, err = search(sha1sum, dataFilePath, indexFilePath)
	if err != nil {
		return constructArtifactFromPom(log, jarFile)
	}
	return dep, nil
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

const KeySize = 40

// entrySize defines the fixed size of each index entry in bytes.
// Each entry contains: key (KeySize bytes) + offset (8 bytes) + length (8 bytes).
const entrySize = KeySize + 8 + 8

// IndexEntry represents a single entry in the search index.
// It contains the key and metadata needed to locate the corresponding value in the data file.
type IndexEntry struct {
	Key    string // The search key
	Offset int64  // Byte offset of the line in the data file
	Length int64  // Length of the line in the data file
}

// search performs a complete search operation for a given key.
// It opens the index and data files, searches for the key, and prints the result.
// This is the main search function used by the CLI.
//
// Parameters:
//   - key: the key to search for
//   - indexFile: path to the binary index file
//   - dataFile: path to the original data file
//
// Returns an error if any step of the search process fails.
func search(key, dataFile, indexFile string) (javaArtifact, error) {
	index, err := os.Open(indexFile)
	if err != nil {
		return javaArtifact{}, fmt.Errorf("failed to open index file: %w", err)
	}
	defer index.Close()

	data, err := os.Open(dataFile)
	if err != nil {
		return javaArtifact{}, fmt.Errorf("failed to open data file: %w", err)
	}
	defer data.Close()

	entry, err := searchIndex(index, key)
	if err != nil {
		return javaArtifact{}, fmt.Errorf("search failed: %w", err)
	}

	val, err := findValue(data, entry)
	if err != nil {
		return javaArtifact{}, fmt.Errorf("failed to find value: %w", err)
	}

	dep := buildJavaArtifact(key, val)

	return dep, nil
}

// searchIndex performs a binary search on the index file to find an exact key match.
// It uses Go's sort.Search function to efficiently locate the key in the sorted index.
// This removes the need to read the entire index file into memory.
//
// Parameters:
//   - f: open file handle to the binary index file
//   - key: the key to search for
//
// Returns the IndexEntry if found, or an error if the key doesn't exist.
func searchIndex(f *os.File, key string) (*IndexEntry, error) {
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	n := int(fi.Size() / entrySize)

	// binary search over file
	i := sort.Search(n, func(i int) bool {
		entryKey, _ := readKeyAt(f, i)
		return entryKey >= key
	})
	if i >= n {
		return nil, fmt.Errorf("not found")
	}

	// read full entry and verify exact match
	entry, err := readEntryAt(f, i)
	if err != nil {
		return nil, err
	}

	// Check if we found an exact match
	if entry.Key != key {
		return nil, fmt.Errorf("not found")
	}

	return entry, nil
}

// readKeyAt reads just the key portion of an index entry at the specified position.
// This is used during binary search to compare keys without reading the full entry.
//
// Parameters:
//   - f: open file handle to the binary index file
//   - i: the index position (0-based) of the entry to read
//
// Returns the key string with null bytes trimmed, or an error if the read fails.
func readKeyAt(f *os.File, i int) (string, error) {
	pos := int64(i) * entrySize
	buf := make([]byte, KeySize)
	_, err := f.ReadAt(buf, pos)
	if err != nil {
		return "", err
	}
	return string(bytes.TrimRight(buf, "\x00")), nil
}

// readEntryAt reads a complete index entry at the specified position.
// It deserializes the binary data into an IndexEntry struct.
//
// Parameters:
//   - f: open file handle to the binary index file
//   - i: the index position (0-based) of the entry to read
//
// Returns a pointer to the IndexEntry, or an error if the read or deserialization fails.
func readEntryAt(f *os.File, i int) (*IndexEntry, error) {
	pos := int64(i) * entrySize
	buf := make([]byte, entrySize)
	_, err := f.ReadAt(buf, pos)
	if err != nil {
		return nil, err
	}

	key := string(bytes.TrimRight(buf[:KeySize], "\x00"))
	offset := int64(binary.LittleEndian.Uint64(buf[KeySize : KeySize+8]))
	length := int64(binary.LittleEndian.Uint64(buf[KeySize+8 : KeySize+16]))

	return &IndexEntry{Key: key, Offset: offset, Length: length}, nil
}

// findValue extracts the value portion from a line in the data file.
// It uses the offset and length from the IndexEntry to read the exact line,
// then splits it to extract the value part after the key.
//
// Parameters:
//   - dataFile: open file handle to the original data file
//   - e: IndexEntry containing the offset and length of the target line
//
// Returns the value string, or an error if the read fails or the line format is invalid.
func findValue(dataFile *os.File, e *IndexEntry) (string, error) {
	buf := make([]byte, e.Length)
	_, err := dataFile.ReadAt(buf, e.Offset)
	if err != nil {
		return "", err
	}
	parts := bytes.SplitN(bytes.TrimSpace(buf), []byte(" "), 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("malformed line")
	}
	return string(parts[1]), nil
}

func buildJavaArtifact(sha, str string) javaArtifact {
	dep := javaArtifact{}
	parts := strings.Split(str, ":")
	dep.GroupId = parts[0]
	dep.ArtifactId = parts[1]
	dep.Version = parts[4]
	dep.foundOnline = true
	dep.sha1 = sha
	return dep
}
