package dependency

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/tracing"
)

const (
	earModuleName  = "ear-module"
	mavenEarPlugin = "maven-ear-plugin"
)

type earArtifact struct {
	explodeArtifact
	tmpDir       string
	ctx          context.Context
	archiveFiles []string
	log          logr.Logger
}

// This handles the case, when we explode "something" and it contains a war artifact.
// The primary place this will happen, is in an ear file decomp/explosion
func (e *earArtifact) Run(ctx context.Context, log logr.Logger) error {
	e.ctx = ctx
	e.log = log.WithName("ear").WithValues("artifact", filepath.Base(e.artifactPath))
	_, span := tracing.StartNewSpan(ctx, "ear-artifact-job")
	defer span.End()
	var err error
	var artifacts []JavaArtifact
	var outputLocationBase string
	defer func() {
		log.V(9).Info("Returning")
		e.decompilerResponses <- DecomplierResponse{
			Artifacts:         artifacts,
			ouputLocationBase: outputLocationBase,
			err:               err,
		}
	}()
	// Handle explosion
	e.tmpDir, err = e.explodeArtifact.ExplodeArtifact(ctx, log)
	if err != nil {
		return err
	}
	outputLocationBase = e.tmpDir
	err = filepath.WalkDir(e.tmpDir, e.HandleFile)
	if err != nil {
		return err
	}

	// Ear files are VERY hard to decompile into the corect project structure
	// mostly because they are very configurable see: https://maven.apache.org/plugins/maven-ear-plugin/modules.html. Becasue they are so configurable
	// it is going to be challenging to get that right every time.
	// an option then, is to decompile into the project EVERYTHING that is at the top level.
	// IF a jar is in a subdirectory of the root, we will assume it is a dependency. This might not be a valid assumption for everything, but we can come
	// back to it if there are bugs that are filed.
	var errs []error
	for _, archivePath := range e.archiveFiles {
		// TODO: We can figure out potential deps, if they are in the lib folder of another archive and can skip
		// We should potentially do this.
		relPath, err := filepath.Rel(e.tmpDir, archivePath)
		e.log.Info("archive relPath", "path", relPath)
		if err != nil {
			return err
		}
		if relPath == filepath.Base(archivePath) {
			e.log.Info("archive path", "path", archivePath)
			// If it is in the top level directory
			// Then decompile into the project.
			err = e.decompiler.internalDecompileIntoProject(ctx, archivePath, e.outputPath, e.decompilerResponses, e.decompilerWG)
			if err != nil {
				// Errors return if we are unable to process this, and the thread
				// will be active again with nothing coming back on the return channel
				log.Error(err, "unable to decompile jar into project")
				errs = append(errs, err)
			}
		} else {
			// If it is in some other directory
			// Decompile as a dependency.
			err = e.decompiler.internalDecompile(ctx, archivePath, e.decompilerResponses, e.decompilerWG)
			if err != nil {
				// Errors return if we are unable to process this, and the thread
				// will be active again with nothing coming back on the return channel
				log.Error(err, "unable to decompile jar into project")
				errs = append(errs, err)
			}
		}
	}

	if len(errs) > 0 {
		err = errs[0]
		return err
	}

	return nil
}

func (e *earArtifact) HandleFile(path string, d fs.DirEntry, err error) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	relPath, err := filepath.Rel(e.tmpDir, path)
	if err != nil {
		return err
	}

	if !e.shouldHandleFile(relPath) {
		return nil
	}

	outputPath := e.getOutputPath(relPath)

	// Decompiles all of the class to the correct location in the output path "<path>/src/main/java"
	if d.IsDir() && filepath.Base(outputPath) == "classes" {
		decompileCommand := exec.CommandContext(context.Background(), e.javaPath, "-jar", e.decompileTool, absPath, outputPath)
		err = decompileCommand.Run()
		if err != nil {
			return err
		}
		return nil
	}
	if d.IsDir() && filepath.Base(outputPath) == "lib" {
		// We don't need to do anything  as all of these
		// will be treated as dependencies
		return nil
	}

	if d.IsDir() {
		if err = os.MkdirAll(filepath.Dir(outputPath), DirPermRWXGrp); err != nil {
			return err
		}
		return nil
	}

	switch filepath.Ext(outputPath) {
	case JavaArchive, WebArchive:
		e.log.Info("found archive", "out", outputPath)
		e.archiveFiles = append(e.archiveFiles, absPath)
		return nil
	}

	err = CopyFile(absPath, outputPath)
	if err != nil {
		return err
	}

	return nil
}

func (e *earArtifact) shouldHandleFile(relPath string) bool {
	// Everything here is not for source code but for the
	// binary. We can ignore this.
	if strings.Contains(relPath, METAINF) && !strings.Contains(relPath, "xml") {
		return false
	}
	return true
}

func (e *earArtifact) getOutputPath(relPath string) string {
	if strings.Contains(relPath, METAINF) && filepath.Base(relPath) == PomXmlFile {
		return filepath.Join(e.outputPath, filepath.Base(relPath))
	}
	return filepath.Join(e.outputPath, relPath)
}
