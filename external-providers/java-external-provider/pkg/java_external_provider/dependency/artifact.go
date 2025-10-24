package dependency

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-logr/logr"
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
func ToDependency(_ context.Context, log logr.Logger, labeler labels.Labeler, jarFile string, mavenIndexPath string) (JavaArtifact, error) {
	dep, err := constructArtifactFromSHA(log, jarFile, mavenIndexPath)
	if err == nil {
		return dep, nil
	}
	log.V(3).Error(err, "unable to look up dependency by SHA, falling back to get maven cordinates", "jar", jarFile)
	dep, err = constructArtifactFromPom(log, jarFile)
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

func constructArtifactFromSHA(log logr.Logger, jarFile string, mavenIndexPath string) (JavaArtifact, error) {
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
	dataFilePath := filepath.Join(mavenIndexPath, "maven-index.txt")
	indexFilePath := filepath.Join(mavenIndexPath, "maven-index.idx")
	dep, err = search(log, sha1sum, dataFilePath, indexFilePath)
	if err != nil {
		return constructArtifactFromPom(log, jarFile)
	}
	return dep, nil
}

func constructArtifactFromPom(log logr.Logger, jarFile string) (JavaArtifact, error) {
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

		log.Info("match", "match", match, "name", file.Name)

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
func search(log logr.Logger, key, dataFile, indexFile string) (JavaArtifact, error) {
	data, err := os.Open(dataFile)
	if err != nil {
		return JavaArtifact{}, err
	}
	defer data.Close()
	val, err := searchIndex(log, data, key)
	if err != nil {
		return JavaArtifact{}, fmt.Errorf("search failed: %w", err)
	}

	return buildJavaArtifact(key, val), nil
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
func searchIndex(log logr.Logger, f *os.File, key string) (string, error) {
	fi, err := f.Stat()
	if err != nil {
		return "", err
	}
	n := int(fi.Size())

	// binary search over file
	var entry string
	var searchErr error
	i := sort.Search(n, func(i int) bool {
		// Hopefully this short circuts the search loop
		if searchErr != nil {
			return true
		}
		log.Info("read at", "index", i)
		entryKey, newEntry, err := readKeyAt(f, i)
		log.Info("finished read at", "index", i, "key", key, "newKey", entryKey, "newEntry", newEntry, "err", err)
		if err != nil {
			searchErr = err
			return true
		}
		if entryKey == key {
			entry = newEntry
		}
		return entryKey >= key
	})
	if searchErr != nil {
		return "", searchErr
	}
	if i >= n {
		return "", fmt.Errorf("not found")
	}
	if entry != "" {
		return entry, nil
	} else {
		// read again from i
		return "", errors.New("not found")
	}
}

// readKeyAt reads just the key portion of an index entry at the specified position.
// This is used during binary search to compare keys without reading the full entry.
//
// Parameters:
//   - f: open file handle to the binary index file
//   - i: the index position (0-based) of the entry to read
//
// Returns the key string with null bytes trimmed, or an error if the read fails.
func readKeyAt(f *os.File, i int) (string, string, error) {
	_, err := f.Seek(int64(i), io.SeekStart)
	if err != nil {
		return "", "", err
	}

	// For now test with 500 bytes (largest line is 206, so worst case i is firt byte in that line, so 206 * 2 is what we want in the buffer, or 412 so 500 is a bit extra
	scan := bufio.NewReaderSize(f, 500)
	_, err = scan.ReadString('\n')
	if err != nil {
		return "", "", err
	}
	line, err := scan.ReadString('\n')
	if err != nil {
		return "", "", err
	}
	parts := strings.Split(strings.TrimSpace(line), " ")
	if len(parts) != 2 {
		return "", "", errors.New("invalid line in the index file")
	}
	return parts[0], parts[1], nil
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

func buildJavaArtifact(sha, str string) JavaArtifact {
	dep := JavaArtifact{}
	parts := strings.Split(str, ":")
	dep.GroupId = parts[0]
	dep.ArtifactId = parts[1]
	dep.Version = parts[4]
	dep.FoundOnline = true
	dep.Sha1 = sha
	return dep
}
