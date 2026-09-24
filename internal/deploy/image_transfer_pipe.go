package deploy

import (
	"context"
	"fmt"
	"io"
	"os/exec"

	"github.com/arm/topo/internal/project"
	"golang.org/x/sync/errgroup"
)

// SaveImageCommand creates a save command bound to a source engine endpoint.
type SaveImageCommand func(ctx context.Context, image string) *exec.Cmd

// LoadImageCommand creates a load command bound to a target engine endpoint.
type LoadImageCommand func(ctx context.Context) *exec.Cmd

// TransferImagesViaPipe transfers project images concurrently using save/load commands.
// The output writer and command factories must support concurrent use.
func TransferImagesViaPipe(
	ctx context.Context,
	output io.Writer,
	scope project.Scope,
	saveImage SaveImageCommand,
	loadImage LoadImageCommand,
) error {
	images, err := project.ImageNames(scope)
	if err != nil {
		return err
	}

	var group errgroup.Group
	for _, image := range images {
		group.Go(func() error {
			return transferImageViaPipe(ctx, output, image, saveImage, loadImage)
		})
	}
	return group.Wait()
}

func transferImageViaPipe(
	ctx context.Context,
	output io.Writer,
	image string,
	saveImage SaveImageCommand,
	loadImage LoadImageCommand,
) error {
	reader, writer := io.Pipe()

	save := saveImage(ctx, image)
	save.Stdout = writer
	save.Stderr = output

	load := loadImage(ctx)
	load.Stdin = reader
	load.Stdout = output
	load.Stderr = output

	var group errgroup.Group
	group.Go(func() error {
		err := save.Run()
		// Propagate failure instead of presenting a truncated image as complete.
		_ = writer.CloseWithError(err)
		if err != nil {
			return fmt.Errorf("failed to save image %s: %w", image, err)
		}
		return nil
	})
	group.Go(func() error {
		err := load.Run()
		// Unblock the writer if loading stops before consuming the image.
		_ = reader.CloseWithError(err)
		if err != nil {
			return fmt.Errorf("failed to load image %s: %w", image, err)
		}
		return nil
	})
	return group.Wait()
}
