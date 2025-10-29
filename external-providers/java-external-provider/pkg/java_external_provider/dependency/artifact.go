package dependency

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency/labels"
	"github.com/vifraa/gopom"
)

// JavaArtifact represents Maven coordinates and metadata for a Java dependency artifact.
// It is used to identify JAR files and manage their Maven repository locations.
//
// The artifact can be constructed from various sources:
//   - SHA1 hash lookup in Maven index
//   - Embedded pom.properties in JAR META-INF
//   - Inferred from file path structure
type JavaArtifact struct {
	FoundOnline bool   // Whether the artifact was found in Maven Central or known OSS repositories
	Packaging   string // Archive type: .jar, .war, .ear
	GroupId     string // Maven groupId (e.g., "org.springframework")
	ArtifactId  string // Maven artifactId (e.g., "spring-core")
	Version     string // Maven version (e.g., "5.3.21")
	Sha1        string // SHA1 hash for verification and lookups
}

// IsValid checks if the artifact has the minimum required Maven coordinates.
// Returns true if groupId, artifactId, and version are all non-empty.
func (j JavaArtifact) IsValid() bool {
	return (j.ArtifactId != "" && j.GroupId != "" && j.Version != "")
}

// EqualsPomDep compares a JavaArtifact with a gopom.Dependency for equality.
// Returns true if groupId, artifactId, and version all match.
// Returns false if any field is nil or doesn't match.
func (j JavaArtifact) EqualsPomDep(dependency gopom.Dependency) bool {
	if dependency.ArtifactID == nil || dependency.GroupID == nil || dependency.Version == nil {
		return false
	}
	if j.ArtifactId == *dependency.ArtifactID && j.GroupId == *dependency.GroupID && j.Version == *dependency.Version {
		return true
	}
	return false
}

// ToPomDep converts a JavaArtifact to a gopom.Dependency structure.
// This is used when generating or updating pom.xml files with discovered dependencies.
func (j JavaArtifact) ToPomDep() gopom.Dependency {
	return gopom.Dependency{
		GroupID:    &j.GroupId,
		ArtifactID: &j.ArtifactId,
		Version:    &j.Version,
	}
}

// ToDependency identifies Maven coordinates for a JAR file using multiple strategies.
// It attempts identification in the following order:
//  1. SHA1 hash lookup in Maven index (fastest, requires mavenIndexPath)
//  2. Extract from embedded pom.properties in JAR META-INF (fallback)
//
// Parameters:
//   - jarFile: Absolute path to the JAR file to identify
//   - mavenIndexPath: Path to Maven index directory for SHA1 lookups
//   - log: Logger for progress and error reporting
//   - labeler: Used to determine if dependency is open source (unused in current implementation)
//
// Returns the JavaArtifact with coordinates, or empty artifact with error if all strategies fail.
func ToDependency(_ context.Context, log logr.Logger, labeler labels.Labeler, jarFile string, mavenIndexPath string) (JavaArtifact, error) {
	dep, err := constructArtifactFromSHA(log, jarFile, mavenIndexPath)
	if err == nil {
		return dep, nil
	}
	log.V(3).Error(err, "unable to look up dependency by SHA, falling back to get maven cordinates", "jar", jarFile)
	dep, err = constructArtifactFromPom(log, jarFile)
	if err != nil {
		return JavaArtifact{}, err
	}
	return dep, nil
}

// mavenSearchErrorCache caches errors from Maven search to avoid repeated failures.
// TODO: This is currently unused but intended for error caching optimization.
var mavenSearchErrorCache error

// constructArtifactFromSHA identifies a JAR file by computing its SHA1 hash
// and looking it up in a Maven index file.
//
// This is the fastest identification method as it uses a pre-built index
// of SHA1 hashes to Maven coordinates, avoiding the need to open and parse the JAR.
//
// Parameters:
//   - log: Logger for error reporting
//   - jarFile: Absolute path to the JAR file
//   - mavenIndexPath: Path to directory containing maven-index.txt
//
// Returns JavaArtifact with FoundOnline=true if found, or error if lookup fails.
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
	dep, err = search(log, sha1sum, dataFilePath)
	return dep, nil
}

// constructArtifactFromPom extracts Maven coordinates from a JAR's embedded pom.properties file.
// This is used as a fallback when SHA1 lookup fails.
//
// The function looks for META-INF/maven/*/*/pom.properties inside the JAR and parses
// the groupId, artifactId, and version from the properties file.
//
// Parameters:
//   - log: Logger for progress and error reporting
//   - jarFile: Absolute path to the JAR file to analyze
//
// Returns JavaArtifact with coordinates from the embedded POM, or error if not found.
func constructArtifactFromPom(log logr.Logger, jarFile string) (JavaArtifact, error) {
	log.V(5).Info("trying to find pom within jar", "jarFile", jarFile)
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

		log.V(5).Info("match", "match", match, "name", file.Name)

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
			if scanner.Err() != nil {
				return dep, scanner.Err()
			}
			return dep, err
		}
	}
	return dep, fmt.Errorf("failed to construct artifact from pom properties")
}

// ToFilePathDependency infers Maven coordinates from a file path structure.
// This is used as a last-resort fallback when neither SHA1 lookup nor embedded POM work.
//
// The function assumes the file path follows Java package structure:
// /org/springframework/boot/loader/jar/Something.class becomes:
//   - GroupId: org.springframework.boot.loader
//   - ArtifactId: jar
//   - Version: 0.0.0 (placeholder)
//
// Parameters:
//   - filePath: Path to a .class file within a decompiled structure
//
// Returns JavaArtifact with inferred coordinates. Version is always set to "0.0.0".
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
func search(log logr.Logger, key, dataFile string) (JavaArtifact, error) {
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
		entryKey, newEntry, err := readKeyAt(f, i)
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

// buildJavaArtifact constructs a JavaArtifact from index lookup results.
// The input string is expected to be in Maven coordinate format from the index:
// "groupId:artifactId:packaging:classifier:version"
//
// Parameters:
//   - sha: SHA1 hash of the artifact
//   - str: Maven coordinates string from index lookup
//
// Returns JavaArtifact with FoundOnline=true and coordinates parsed from the string.
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
