package operation

import (
	"context"
	"fmt"
	"io"
	"os/exec"

	"github.com/arm/topo/internal/compose"
	"github.com/arm/topo/internal/deploy/command"
	"golang.org/x/sync/errgroup"
)

type DockerComposePipeTransfer struct {
	composeFile string
	source      command.Host
	dest        command.Host
}

func NewDockerComposePipeTransfer(composeFile string, source, dest command.Host) *DockerComposePipeTransfer {
	return &DockerComposePipeTransfer{
		composeFile: composeFile,
		source:      source,
		dest:        dest,
	}
}

func (t *DockerComposePipeTransfer) Description() string {
	return "Transfer images"
}

func (t *DockerComposePipeTransfer) Run(ctx context.Context, cmdOutput io.Writer) error {
	images, err := compose.ImageNames(t.composeFile)
	if err != nil {
		return err
	}
	g, ctx := errgroup.WithContext(ctx)
	for _, image := range images {
		g.Go(func() error {
			return t.transferImage(ctx, cmdOutput, image)
		})
	}
	return g.Wait()
}

func (t *DockerComposePipeTransfer) buildTransferCommands(ctx context.Context, imageName string) (*exec.Cmd, *exec.Cmd) {
	saveCmd := command.Docker(ctx, t.source, "save", imageName)
	loadCmd := command.Docker(ctx, t.dest, "load")
	return saveCmd, loadCmd
}

func (t *DockerComposePipeTransfer) transferImage(ctx context.Context, cmdOutput io.Writer, imageName string) error {
	pipeReader, pipeWriter := io.Pipe()

	saveCmd, loadCmd := t.buildTransferCommands(ctx, imageName)
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
