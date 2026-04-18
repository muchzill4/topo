package operation

import (
	"fmt"
	"io"

	"github.com/arm/topo/internal/deploy/engine"
)

type ContainerRun struct {
	engine    engine.Engine
	host      engine.Host
	image     string
	container string
	args      []string
}

func NewContainerRun(e engine.Engine, host engine.Host, image string, container string, args []string) *ContainerRun {
	return &ContainerRun{engine: e, host: host, image: image, container: container, args: args}
}

func (r *ContainerRun) Description() string {
	return fmt.Sprintf("Run image %s as container %s", r.image, r.container)
}

func (r *ContainerRun) Run(w io.Writer) error {
	runArgs := []string{"run"}
	runArgs = append(runArgs, r.args...)
	runArgs = append(runArgs, "--name", r.container)
	runArgs = append(runArgs, r.image)
	cmd := engine.Cmd(r.engine, r.host, runArgs...)
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd.Run()
}
