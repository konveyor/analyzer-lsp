package bldtool

import (
	"sync"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
)

type depCache struct {
	hashFile string
	hash     *string     // SHA256 hash of pom.xml for caching
	hashSync *sync.Mutex // Mutex for thread-safe hash access
	deps     map[uri.URI][]provider.DepDAGItem
	depLog   logr.Logger
}

func (d *depCache) useCache() (bool, error) {
	hashString, err := getHash(d.hashFile)
	if err != nil {
		d.depLog.Error(err, "unable to generate hash from pom file")
		return false, err
	}
	if d.hash != nil && *d.hash == hashString {
		return true, nil
	}
	// We are locking this until deps are set.
	d.hashSync.Lock()
	return false, nil

}

func (d *depCache) getCachedDeps() map[uri.URI][]provider.DepDAGItem {
	return d.deps
}

func (d *depCache) setCachedDeps(deps map[uri.URI][]provider.DepDAGItem, err error) error {
	hashString, err := getHash(d.hashFile)
	if err != nil {
		d.depLog.Error(err, "unable to generate hash from pom file")
		return err
	}
	d.deps = deps
	d.hash = &hashString
	d.hashSync.Unlock()
	return nil
}
