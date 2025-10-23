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

type explodeArtifact struct {
	baseArtifact
	outputPath string
}

func (e *explodeArtifact) ExplodeArtifact(ctx context.Context, log logr.Logger) (string, error) {
	log.V(7).Info(fmt.Sprintf("exploding: %s", e.baseArtifact.artifactPath))
	// First we are going to explode the artifact to a tmp directory.
	tmpDir := os.TempDir()
	tmpDir = filepath.Join(tmpDir, fmt.Sprintf("explode-%s-%v", strings.TrimSuffix(filepath.Base(e.artifactPath), filepath.Ext(e.artifactPath)), rand.IntN(100)))
	log.V(7).Info("exploding into tmpDir", "tmpDir", tmpDir)
	os.MkdirAll(tmpDir, DirPermRWXGrp)

	// Now we need to explode the archive into the tmp folder whole sale.
	cmd := exec.CommandContext(ctx, "jar", "-xvf", e.artifactPath)
	cmd.Dir = tmpDir
	err := cmd.Run()
	if err != nil {
		log.V(7).Error(err, "exploding into tmpDir error", "tmpDir", tmpDir)
		return "", err
	}

	return tmpDir, nil
}
