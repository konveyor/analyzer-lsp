package bldtool

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
)

// depCache provides thread-safe dependency caching for build tool implementations.
// It caches dependency resolution results based on SHA256 hash of build files
// (pom.xml, build.gradle) to avoid expensive re-execution of build commands.
//
// The cache is invalidated automatically when the build file changes. It uses
// a mutex to ensure only one dependency resolution happens at a time, preventing
// concurrent execution of Maven/Gradle commands.
//
// Thread Safety:
//   - Lock is acquired at the start of useCache() before hash computation
//   - Lock is released immediately on cache hit
//   - Lock is held through dependency resolution on cache miss
//   - Lock is released by setCachedDeps() after updating cache
//
// TODO: Handle cached Dep errors
type depCache struct {
	hashFile string                            // Path to build file (pom.xml, build.gradle)
	hash     *string                           // SHA256 hash of build file for cache validation
	hashSync sync.Mutex                        // Mutex for thread-safe cache access
	deps     map[uri.URI][]provider.DepDAGItem // Cached dependency DAG
	depLog   logr.Logger                       // Logger for cache operations
}

// useCache checks if cached dependencies are valid by comparing build file hash.
// It acquires a lock immediately to ensure thread-safe cache access.
//
// Returns:
//   - (true, nil) if cache is valid - lock is released before returning
//   - (false, nil) if cache is invalid - lock remains held for caller to populate cache
//   - (false, error) if hash computation fails - lock is not acquired
//
// The caller must call setCachedDeps() after populating dependencies to release the lock.
func (d *depCache) useCache() (bool, error) {
	hashString, err := getHash(d.hashFile)
	if err != nil {
		d.depLog.Error(err, "unable to generate hash from pom file")
		return false, err
	}
	// We are locking this until deps are set.
	// Only allow one thing to get deps at a time.
	d.hashSync.Lock()
	if d.hash != nil && *d.hash == hashString {
		d.hashSync.Unlock()
		return true, nil
	}
	return false, nil

}

// getCachedDeps returns the cached dependency DAG.
// This should only be called when useCache() returns true.
func (d *depCache) getCachedDeps() map[uri.URI][]provider.DepDAGItem {
	return d.deps
}

// setCachedDeps updates the cache with new dependencies and releases the lock
// acquired by useCache(). This method must be called after useCache() returns
// false to update the cache and release the lock.
//
// Parameters:
//   - deps: The dependency DAG to cache
//   - err: Error from dependency resolution (currently unused, see TODO)
//
// Returns:
//   - error if hash computation fails
//
// The lock is always released, even if an error occurs.
func (d *depCache) setCachedDeps(deps map[uri.URI][]provider.DepDAGItem, err error) error {
	hashString, err := getHash(d.hashFile)
	if err != nil {
		d.depLog.Error(err, "unable to generate hash from pom file")
		d.hashSync.Unlock()
		// TODO: Handle cached dep errors.
		return err
	}
	d.deps = deps
	d.hash = &hashString
	d.hashSync.Unlock()
	return nil
}

func getHash(path string) (string, error) {
	hash := sha256.New()
	var file *os.File
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("unable to open the pom file %s - %w", path, err)
	}
	if _, err = io.Copy(hash, file); err != nil {
		file.Close()
		return "", fmt.Errorf("unable to copy file to hash %s - %w", path, err)
	}
	file.Close()
	return string(hash.Sum(nil)), nil
}
