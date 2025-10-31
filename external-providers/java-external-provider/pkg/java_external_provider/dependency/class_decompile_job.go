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
	responseChanndel chan DecomplierResponse
	wg               *sync.WaitGroup
	log              logr.Logger
}

func (c *classDecompileJob) Run(ctx context.Context, log logr.Logger) error {
	defer c.wg.Done()
	err := os.MkdirAll(c.outputPath, DirPermRWXGrp)
	if err != nil {
		return err
	}
	decompileCommand := exec.CommandContext(context.Background(), "java", "-jar", c.decompileTool, c.classDirPath, c.outputPath)
	err = decompileCommand.Run()
	if err != nil {
		c.log.Error(err, "failed to decompile", "classDirPath", c.classDirPath)
		return err
	}
	return nil
}

func (c *classDecompileJob) Done() {
	c.wg.Done()
}
