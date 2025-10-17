package dependency

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
)

type exploadArtifact struct {
	baseArtifact
	outputPath string
}

func (e *exploadArtifact) ExploadArtifact(ctx context.Context, log logr.Logger) (string, error) {
	log.V(7).Info(fmt.Sprintf("exploading: %s", e.baseArtifact.artifactPath))
	// First we are going to expload the artifact to a tmp directory.
	tmpDir := os.TempDir()
	tmpDir = filepath.Join(tmpDir, fmt.Sprintf("expload-%s-%v", strings.TrimSuffix(filepath.Base(e.artifactPath), filepath.Ext(e.artifactPath)), rand.IntN(100)))
	log.V(7).Info("exploading into tmpDir", "tmpDir", tmpDir)
	os.MkdirAll(tmpDir, 0770)

	// Now we need to expload the archive into the tmp folder whole sale.
	cmd := exec.CommandContext(ctx, "jar", "-xvf", e.artifactPath)
	cmd.Dir = tmpDir
	err := cmd.Run()
	if err != nil {
		log.V(7).Error(err, "exploading into tmpDir error", "tmpDir", tmpDir)
		return "", err
	}

	return tmpDir, nil
}
