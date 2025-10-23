package dependency

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/tracing"
)

const (
	CSS    = "css"
	JS     = "js"
	IMAGES = "images"
	HTML   = "html"
)

type warArtifact struct {
	explodeArtifact
	tmpDir string
	ctx    context.Context
	log    logr.Logger
}

// This handles the case, when we explode "something" and it contains a war artifact.
// The primary place this will happen, is in an ear file decomp/explosion
func (w *warArtifact) Run(ctx context.Context, log logr.Logger) error {
	defer w.decompilerWG.Done()
	w.ctx = ctx
	w.log = log.WithName("war").WithValues("artifact", filepath.Base(w.artifactPath))
	jobCtx, span := tracing.StartNewSpan(ctx, "war-artifact-job")
	// Handle explosion
	var err error
	w.tmpDir, err = w.explodeArtifact.ExplodeArtifact(ctx, log)
	if err != nil {
		return err
	}

	err = filepath.WalkDir(w.tmpDir, w.HandleFile)
	if err != nil {
		return err
	}

	span.End()
	jobCtx.Done()
	return nil
}

func (w *warArtifact) HandleFile(path string, d fs.DirEntry, err error) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	relPath, err := filepath.Rel(w.tmpDir, path)
	if err != nil {
		return err
	}

	if !w.shouldHandleFile(relPath) {
		return nil
	}

	outputPath := w.getOutputPath(relPath)
	w.log.Info("outputpath", "output", outputPath)

	// Decompiles all of the class to the correct location in the output path "<path>/src/main/java"
	if d.IsDir() && strings.Contains(outputPath, JAVA) {
		if err = os.MkdirAll(outputPath, DirPermRWXGrp); err != nil {
			return err
		}
		decompileCommand := exec.CommandContext(context.Background(), w.javaPath, "-jar", w.decompileTool, absPath, outputPath)
		out, err := decompileCommand.Output()
		w.log.Info(fmt.Sprintf("%s", out))
		if err != nil {
			return err
		}
		return nil
	}
	if d.IsDir() {
		// We don't need to do anything  as all of these
		// will be treated as dependencies
		return nil
	}

	if !d.IsDir() {
		if strings.Contains(outputPath, "classes") {
			return nil
		}
		if err = os.MkdirAll(filepath.Dir(filepath.Base(outputPath)), DirPermRWXGrp); err != nil {
			return err
		}
	}

	if strings.Contains(outputPath, "lib") && strings.Contains(outputPath, WEBINF) {
		// We need to handle this library as a dependency
		err = w.decompiler.internalDecompile(w.ctx, absPath, w.decompilerResponses, w.decompilerWG)
		if err != nil {
			return err
		}
		return nil
	}

	err = CopyFile(absPath, outputPath)
	if err != nil {
		return err
	}

	return nil
}

func (w *warArtifact) convertToWebappFolder(relPath string) string {
	return filepath.Join(w.outputPath, WEBAPP, relPath)
}

func (w *warArtifact) shouldHandleFile(relPath string) bool {
	// Everything here is not for source code but for the
	// binary. We can ignore this.
	if strings.Contains(relPath, METAINF) && !strings.Contains(relPath, PomXmlFile) {
		return false
	}
	return true
}

func (w *warArtifact) getOutputPath(relPath string) string {
	if strings.Contains(relPath, CSS) || strings.Contains(relPath, JS) || strings.Contains(relPath, IMAGES) {
		// These folders need to move to src/main/webapp
		return w.convertToWebappFolder(relPath)
	}
	if strings.Contains(filepath.Ext(relPath), HTML) {
		return w.convertToWebappFolder(relPath)
	}
	if strings.Contains(relPath, WEBINF) && !(strings.Contains(relPath, "classes") || strings.Contains(relPath, "lib")) {
		return w.convertToWebappFolder(relPath)
	}
	if strings.Contains(relPath, METAINF) && filepath.Base(relPath) == PomXmlFile {
		return filepath.Join(w.outputPath, filepath.Base(relPath))
	}
	if strings.Contains(relPath, WEBINF) && filepath.Base(relPath) == "classes" {
		return filepath.Join(w.outputPath, JAVA)
	}
	return filepath.Join(w.outputPath, relPath)

}
