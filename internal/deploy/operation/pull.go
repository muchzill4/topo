package operation

import (
	"fmt"
	"io"

	"github.com/arm/topo/internal/deploy/engine"
)

type Pull struct {
	engine engine.Engine
	host   engine.Host
	image  string
}

func NewPull(e engine.Engine, host engine.Host, image string) *Pull {
	return &Pull{engine: e, host: host, image: image}
}

func (p *Pull) Description() string {
	return fmt.Sprintf("Pull image %s", p.image)
}

func (p *Pull) Run(w io.Writer) error {
	cmd := engine.Cmd(p.engine, p.host, "pull", p.image)
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd.Run()
}
