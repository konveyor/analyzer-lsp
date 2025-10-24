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

type jarExplodeArtifact struct {
	explodeArtifact
	tmpDir         string
	ctx            context.Context
	foundClassDirs map[string]struct{}
	log            logr.Logger
}

// This handles the case, when we explode "something" and it contains a war artifact.
// The primary place this will happen, is in an ear file decomp/explosion
func (j *jarExplodeArtifact) Run(ctx context.Context, log logr.Logger) error {
	defer j.decompilerWG.Done()
	j.ctx = ctx
	j.log = log.WithName("explode_jar").WithValues("archive", filepath.Base(j.artifactPath))
	jobCtx, span := tracing.StartNewSpan(ctx, "jar-explode-artifact-job")
	log.V(7).Info("starting jar archive job")
	// Handle explosion
	var err error
	j.tmpDir, err = j.explodeArtifact.ExplodeArtifact(ctx, log)
	j.log.V(7).Info(fmt.Sprintf("explode: %#v, %#v", j.tmpDir, err))
	if err != nil {
		log.Error(err, "unable to explode")
		return err
	}

	err = filepath.WalkDir(j.tmpDir, j.HandleFile)
	if err != nil {
		log.Error(err, "unable to walk directory")
		return err
	}

	span.End()
	jobCtx.Done()
	log.V(7).Info("job finished")
	return nil
}

func (j *jarExplodeArtifact) HandleFile(path string, d fs.DirEntry, err error) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	relPath, err := filepath.Rel(j.tmpDir, path)
	if err != nil {
		return err
	}

	if !j.shouldHandleFile(relPath) {
		return nil
	}

	outputPath := j.getOutputPath(relPath)

	j.log.Info("paths", "relPath", relPath, "output", outputPath)

	if d.IsDir() && filepath.Base(outputPath) == "lib" {
		// We don't need to do anything  as all of these
		// will be treated as dependencies
		return nil
	}

	if d.IsDir() {
		return nil
	}
	if err = os.MkdirAll(filepath.Dir(outputPath), DirPermRWXGrp); err != nil {
		return err
	}

	if strings.Contains(outputPath, "lib") {
		// We need to handle this library as a dependency
		err = j.decompiler.internalDecompile(j.ctx, absPath, j.decompilerResponses, j.decompilerWG)
		if err != nil {
			return err
		}
		return nil
	}

	if strings.Contains(outputPath, "class") {
		// get directory from the base of tmp.
		rel, err := filepath.Rel(j.tmpDir, absPath)
		if err != nil {
			return err
		}
		parts := strings.Split(rel, string(filepath.Separator))
		var dirToCreate string
		if len(parts) == 0 {
			dirToCreate = relPath
		} else {
			dirToCreate = parts[0]
		}
		if _, ok := j.foundClassDirs[dirToCreate]; ok {
			return nil
		}
		err = os.MkdirAll(filepath.Join(j.outputPath, JAVA, dirToCreate), DirPermRWXGrp)
		if err != nil {
			j.log.Info("here failed to create dir")
			return err
		}
		decompileCommand := exec.CommandContext(context.Background(), "java", "-jar", j.decompileTool, filepath.Join(j.tmpDir, dirToCreate), filepath.Join(j.outputPath, JAVA+"/", dirToCreate))
		err = decompileCommand.Run()
		if err != nil {
			j.log.Info("here failed to decompile", "err", err)
			return err
		}
		j.foundClassDirs[dirToCreate] = struct{}{}
		return nil

	}

	err = CopyFile(absPath, outputPath)
	if err != nil {
		return err
	}

	return nil
}

func (j *jarExplodeArtifact) shouldHandleFile(relPath string) bool {
	return true
}

func (j *jarExplodeArtifact) getOutputPath(relPath string) string {
	if strings.Contains(relPath, METAINF) && filepath.Base(relPath) == PomXmlFile {
		return filepath.Join(j.outputPath, filepath.Base(relPath))
	}
	if strings.Contains(relPath, "class") {
		return filepath.Join(j.outputPath, JAVA, relPath)
	}
	return filepath.Join(j.outputPath, relPath)
}
