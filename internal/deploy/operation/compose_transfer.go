package operation

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/arm/topo/internal/compose"
	"github.com/arm/topo/internal/deploy/engine"
	"golang.org/x/sync/errgroup"
)

type ComposePipeTransfer struct {
	sourceEngine engine.Engine
	targetEngine engine.Engine
	composeFile  string
	source       engine.Host
	dest         engine.Host
}

func NewComposePipeTransfer(sourceEngine, targetEngine engine.Engine, composeFile string, source, dest engine.Host) *ComposePipeTransfer {
	return &ComposePipeTransfer{
		sourceEngine: sourceEngine,
		targetEngine: targetEngine,
		composeFile:  composeFile,
		source:       source,
		dest:         dest,
	}
}

func (t *ComposePipeTransfer) Description() string {
	return "Transfer images"
}

func (t *ComposePipeTransfer) Run(cmdOutput io.Writer) error {
	images, err := t.getImagesFromCompose()
	if err != nil {
		return err
	}
	var g errgroup.Group
	for _, image := range images {
		g.Go(func() error {
			return t.transferImage(cmdOutput, image)
		})
	}
	return g.Wait()
}

func (t *ComposePipeTransfer) buildTransferCommands(imageName string) (*exec.Cmd, *exec.Cmd) {
	saveCmd := engine.Cmd(t.sourceEngine, t.source, "save", imageName)
	loadCmd := engine.Cmd(t.targetEngine, t.dest, "load")
	return saveCmd, loadCmd
}

func (t *ComposePipeTransfer) getImagesFromCompose() ([]string, error) {
	f, err := os.Open(t.composeFile)
	if err != nil {
		return nil, fmt.Errorf("reading compose file: %w", err)
	}
	defer f.Close() //nolint:errcheck
	return compose.ImageNames(f, compose.ProjectName(t.composeFile))
}

func (t *ComposePipeTransfer) transferImage(cmdOutput io.Writer, imageName string) error {
	pipeReader, pipeWriter := io.Pipe()

	saveCmd, loadCmd := t.buildTransferCommands(imageName)
	saveCmd.Stdout = pipeWriter
	saveCmd.Stderr = cmdOutput
	loadCmd.Stdin = pipeReader
	loadCmd.Stdout = cmdOutput
	loadCmd.Stderr = cmdOutput

	var g errgroup.Group
	g.Go(func() error {
		defer pipeWriter.Close() //nolint:errcheck
		if err := saveCmd.Run(); err != nil {
			return fmt.Errorf("failed to save image: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		defer pipeReader.Close() //nolint:errcheck
		if err := loadCmd.Run(); err != nil {
			return fmt.Errorf("failed to load image: %w", err)
		}
		return nil
	})

	return g.Wait()
}
