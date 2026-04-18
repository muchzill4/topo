package operation

import (
	"fmt"
	"io"

	"github.com/arm/topo/internal/deploy/engine"
)

type Start struct {
	engine    engine.Engine
	host      engine.Host
	container string
}

func NewStart(e engine.Engine, host engine.Host, container string) *Start {
	return &Start{engine: e, host: host, container: container}
}

func (s *Start) Description() string {
	return fmt.Sprintf("Start container %s", s.container)
}

func (s *Start) Run(w io.Writer) error {
	cmd := engine.Cmd(s.engine, s.host, "start", s.container)
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd.Run()
}
