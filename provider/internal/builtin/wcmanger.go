package builtin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/provider"
	"go.lsp.dev/uri"
)

type workingCopy struct {
	filePath string
	wcPath   string
}

type workingCopyManager struct {
	changesChan chan provider.FileChange
	ctx         context.Context
	cancelFunc  context.CancelFunc
	started     bool
	log         logr.Logger

	wcMutex       sync.RWMutex
	workingCopies map[string]workingCopy
	tempDir       string
}

func (t *workingCopyManager) init() error {
	if t.started {
		return nil
	}
	dir, err := os.MkdirTemp("", "analyzer-lsp-wc-manager-")
	if err != nil {
		t.log.Error(err, "failed to create temporary directory to hold working copies")
		return err
	}
	t.log.V(5).Info("inited working copy manager", "dir", dir)
	t.tempDir = dir
	t.started = true
	go t.startWorker()
	return nil
}

func (t *workingCopyManager) stop() {
	t.log.V(5).Info("stopping working copy manager")
	os.RemoveAll(t.tempDir)
	t.cancelFunc()
	close(t.changesChan)
}

func (t *workingCopyManager) getWorkingCopies() []workingCopy {
	t.wcMutex.RLock()
	defer t.wcMutex.RUnlock()
	all := []workingCopy{}
	for _, c := range t.workingCopies {
		all = append(all, c)
	}
	return all
}

func (t *workingCopyManager) notifyChanges(changes ...provider.FileChange) {
	for _, change := range changes {
		t.log.V(7).Info("processing notified change", "path", change.Path, "saved", change.Saved)
		t.changesChan <- change
	}
}

func (t *workingCopyManager) reformatIncidents(incidents ...provider.IncidentContext) []provider.IncidentContext {
	formatted := []provider.IncidentContext{}
	for i := range incidents {
		inc := &incidents[i]
		if strings.HasPrefix(string(inc.FileURI), "file://") &&
			strings.HasPrefix(inc.FileURI.Filename(), t.tempDir) {
			inc.FileURI = uri.File(
				filepath.Clean(strings.Replace(
					inc.FileURI.Filename(), t.tempDir, "", -1)))
		}
		formatted = append(formatted, *inc)
	}
	return formatted
}

func (t *workingCopyManager) startWorker() {
	for {
		select {
		case <-t.ctx.Done():
			return
		case change, ok := <-t.changesChan:
			if !ok {
				return
			}
			_, wcExists := t.workingCopies[change.Path]
			wcPath := filepath.Join(t.tempDir, change.Path)
			// if the change is notifying a file save event
			// we discard the working copy for it
			if change.Saved && wcExists {
				t.wcMutex.Lock()
				delete(t.workingCopies, change.Path)
				t.wcMutex.Unlock()
				if _, err := os.Stat(change.Path); err == nil || !os.IsNotExist(err) {
					err := os.Remove(wcPath)
					if err != nil {
						t.log.Error(err, "failed to remove working copy")
					}
					t.log.V(7).Info("working copy deleted", "change", change.Path, "wcPath", wcPath)
				}
			} else if !change.Saved {
				err := os.MkdirAll(filepath.Dir(wcPath), 0755)
				if err != nil {
					t.log.Error(err, "failed to create dir for working copy", "path", change.Path)
					continue
				}
				err = os.WriteFile(wcPath, []byte(change.Content), 0755)
				if err != nil {
					t.log.Error(err, "failed to create working copy", "path", change.Path)
					continue
				}
				t.log.V(7).Info("working copy created", "change", change.Path, "wcPath", wcPath)
				t.wcMutex.Lock()
				t.workingCopies[change.Path] = workingCopy{
					filePath: change.Path,
					wcPath:   wcPath,
				}
				t.wcMutex.Unlock()
			}
		}
	}
}

func NewTempFileWorkingCopyManger(log logr.Logger) *workingCopyManager {
	ctx, cancelFunc := context.WithCancel(context.Background())
	return &workingCopyManager{
		changesChan:   make(chan provider.FileChange, 1024),
		ctx:           ctx,
		cancelFunc:    cancelFunc,
		log:           log,
		workingCopies: make(map[string]workingCopy),
		wcMutex:       sync.RWMutex{},
	}
}
