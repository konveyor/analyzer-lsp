package dependency

import (
	"context"
	"os"
	"os/exec"
	"sync"

	"github.com/go-logr/logr"
)

type classDecompileJob struct {
	classDirPath     string
	outputPath       string
	decompileTool    string
	javaPath         string
	responseChanndel chan DecomplierResponse
	wg               *sync.WaitGroup
	log              logr.Logger
}

func (c *classDecompileJob) Run(ctx context.Context, log logr.Logger) error {
	log.Info("Decompiling classes", "classDir", c.classDirPath)
	var err error
	var artifacts []JavaArtifact
	outputLocationBase := c.outputPath
	defer func() {
		log.V(9).Info("Returning", "artifact", c.classDirPath, "err", err)
		c.responseChanndel <- DecomplierResponse{
			Artifacts:         artifacts,
			ouputLocationBase: outputLocationBase,
			err:               err,
		}
	}()
	err = os.MkdirAll(c.outputPath, DirPermRWX)
	if err != nil {
		log.Error(err, "failed to decompile", "outputPath", c.outputPath, "perms", DirPermRWX)
		return err
	}
	decompileCommand := exec.CommandContext(context.Background(), c.javaPath, "-jar", c.decompileTool, c.classDirPath, c.outputPath)
	out, err := decompileCommand.Output()
	if err != nil {
		log.Error(err, "failed to decompile", "classDirPath", c.classDirPath, "output", string(out), "cmd", decompileCommand)
		return err
	}
	return nil
}
