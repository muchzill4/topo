package podman

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
	sourceSocket, targetSocket Socket,
	scope project.Scope,
) error {
	saveImage := func(ctx context.Context, image string) *exec.Cmd {
		return Command(ctx, sourceSocket, "save", image)
	}
	loadImage := func(ctx context.Context) *exec.Cmd {
		return Command(ctx, targetSocket, "load")
	}
	return deploy.TransferImagesViaPipe(ctx, output, scope, saveImage, loadImage)
}
