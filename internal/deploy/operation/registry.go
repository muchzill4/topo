package operation

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/arm/topo/internal/deploy/engine"
	"github.com/arm/topo/internal/operation"
)

const (
	RegistryContainerName = "topo-registry"
	DefaultRegistryPort   = "12737"
	registryImage         = "registry:2"
)

func NewRunRegistry(e engine.Engine, port string) operation.Sequence {
	localHost := engine.LocalHost
	return operation.NewSequence(
		NewPull(e, localHost, registryImage),
		operation.NewConditional(
			NewContainerExistsPredicate(e, localHost, RegistryContainerName),
			NewStart(e, localHost, RegistryContainerName),
			NewRegistryRunWrapper(NewContainerRun(e, localHost, registryImage, RegistryContainerName,
				[]string{
					"-d",
					"--restart", "always",
					"-p", fmt.Sprintf("127.0.0.1:%s:5000", port),
				},
			)),
		),
	)
}

type RegistryRunWrapper struct {
	*ContainerRun
}

func NewRegistryRunWrapper(r *ContainerRun) *RegistryRunWrapper {
	return &RegistryRunWrapper{ContainerRun: r}
}

func (r *RegistryRunWrapper) Run(w io.Writer) error {
	var buf bytes.Buffer
	combined := io.MultiWriter(w, &buf)
	if err := r.ContainerRun.Run(combined); err != nil {
		if strings.Contains(buf.String(), "already in use") || strings.Contains(buf.String(), "already allocated") {
			return fmt.Errorf("%w\nport is already in use, this could be an existing %s or another process", err, RegistryContainerName)
		}
		return err
	}
	return nil
}

type ContainerExistsPredicate struct {
	engine        engine.Engine
	host          engine.Host
	containerName string
}

func NewContainerExistsPredicate(e engine.Engine, host engine.Host, containerName string) *ContainerExistsPredicate {
	return &ContainerExistsPredicate{engine: e, host: host, containerName: containerName}
}

func (p *ContainerExistsPredicate) Eval() bool {
	cmd := engine.Cmd(p.engine, p.host, "inspect", p.containerName)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}
