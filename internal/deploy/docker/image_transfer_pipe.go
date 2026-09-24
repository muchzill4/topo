package docker

import (
	"context"
	"io"
	"os/exec"

	"github.com/arm/topo/internal/deploy"
	"github.com/arm/topo/internal/project"
)

func TransferImagesViaPipe(
	ctx context.Context,
	output io.Writer,
	source, destination Host,
	scope project.Scope,
) error {
	saveImage := func(ctx context.Context, image string) *exec.Cmd {
		return Command(ctx, source, "save", image)
	}
	loadImage := func(ctx context.Context) *exec.Cmd {
		return Command(ctx, destination, "load")
	}
	return deploy.TransferImagesViaPipe(ctx, output, scope, saveImage, loadImage)
}
