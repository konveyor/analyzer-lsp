package bldtool

import (
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr/testr"
	"github.com/konveyor/analyzer-lsp/output/v1/konveyor"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
)

func TestMultipleCallsToBuildCache(t *testing.T) {

	testCases := []struct {
		Name                    string
		updateHashFile          bool
		waitForTimeout          bool
		expectedDeps            map[uri.URI][]provider.DepDAGItem
		expectedDepsAfterUpdate map[uri.URI][]provider.DepDAGItem
	}{
		{
			Name:           "ValidTwoCalls",
			updateHashFile: false,
			expectedDeps: map[uri.URI][]provider.DepDAGItem{
				uri.File("/testing"): {
					{
						Dep: konveyor.Dep{
							Name:       "testing",
							Version:    "1.0.0",
							Classifier: "io.konveyor",
						},
						AddedDeps: []konveyor.DepDAGItem{},
					},
				},
			},
		},
		{
			Name:           "TimeoutSecondCall",
			updateHashFile: false,
			waitForTimeout: true,
			expectedDeps: map[uri.URI][]provider.DepDAGItem{
				uri.File("/testing"): {
					{
						Dep: konveyor.Dep{
							Name:       "testing",
							Version:    "1.0.0",
							Classifier: "io.konveyor",
						},
						AddedDeps: []konveyor.DepDAGItem{},
					},
				}},
		},
		{
			Name:           "HashFileUpdate",
			updateHashFile: true,
			expectedDeps: map[uri.URI][]provider.DepDAGItem{
				uri.File("/testing"): {
					{
						Dep: konveyor.Dep{
							Name:       "testing",
							Version:    "1.0.0",
							Classifier: "io.konveyor",
						},
						AddedDeps: []konveyor.DepDAGItem{},
					},
				}},
			expectedDepsAfterUpdate: map[uri.URI][]provider.DepDAGItem{
				uri.File("/testing"): {
					{
						Dep: konveyor.Dep{
							Name:       "testing",
							Version:    "1.0.0",
							Classifier: "io.konveyor",
						},
						AddedDeps: []konveyor.DepDAGItem{},
					},
					{
						Dep: konveyor.Dep{
							Name:       "new",
							Version:    "1.0.0",
							Classifier: "io.konveyor",
						},
					},
				},
			},
		},
	}

	log := testr.New(t)
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			//Create a temporty hash file
			file, err := os.CreateTemp("", "hashFile")
			if err != nil {
				t.Fatalf("unable to create hash file")
			}
			defer os.RemoveAll(file.Name())
			depCache := depCache{
				hashFile: file.Name(),
				hashSync: sync.Mutex{},
				depLog:   log,
			}
			if ok, err := depCache.useCache(); ok || err != nil {
				log.Info("should not be able to use cache after creation")
				t.Fail()
			}
			wg := sync.WaitGroup{}
			depReturn := make(chan map[uri.URI][]provider.DepDAGItem)
			if tc.waitForTimeout {
				wg.Add(1)
			}
			useCacheChan := make(chan bool)
			go func() {
				useCache, _ := depCache.useCache()
				useCacheChan <- useCache
			}()
			go func() {
				select {
				case useCache := <-useCacheChan:
					if !useCache {
						log.Info("should not have to reset cache")
						t.Fail()
					}
					depReturn <- depCache.getCachedDeps()
				case <-time.After(15 * time.Second):
					if tc.waitForTimeout {
						wg.Done()
					}
					depReturn <- nil
				}
			}()

			if tc.waitForTimeout {
				wg.Wait()
				ret := <-depReturn
				if ret != nil {
					log.Info("We should not get a return value we have not set the cache")
					t.Fail()
				}
				return
			}
			depCache.setCachedDeps(tc.expectedDeps, nil)
			ret := <-depReturn
			if !reflect.DeepEqual(ret, tc.expectedDeps) {
				log.Info("didn't get expected deps", "expected", tc.expectedDeps, "got", ret)
				t.Fail()
			}

			if tc.expectedDepsAfterUpdate != nil {
				err := os.WriteFile(file.Name(), []byte("testing"), 0644)
				if err != nil {
					t.Fatal("unable to write to temp hash file")
				}
				useCache, err := depCache.useCache()
				if err != nil || useCache {
					log.Info("Expected to be unble to use cache after file update")
					t.Fail()
				}
				depCache.setCachedDeps(tc.expectedDepsAfterUpdate, nil)
				ret := depCache.getCachedDeps()
				if !reflect.DeepEqual(ret, tc.expectedDepsAfterUpdate) {
					log.Info("didn't get expected deps", "expected", tc.expectedDeps, "got", ret)
					t.Fail()
				}
			}
		})
	}
}
