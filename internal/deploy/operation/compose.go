package operation

import (
	"fmt"
	"io"
	"os"

	"github.com/arm/topo/internal/compose"
	"github.com/arm/topo/internal/deploy/engine"
)

type RecreateMode int

const (
	RecreateModeDefault RecreateMode = iota
	RecreateModeForce
	RecreateModeNone
)

type ComposeBuild struct {
	engine      engine.Engine
	composeFile string
	host        engine.Host
}

func NewComposeBuild(e engine.Engine, composeFile string, host engine.Host) *ComposeBuild {
	return &ComposeBuild{engine: e, composeFile: composeFile, host: host}
}

func (c *ComposeBuild) Description() string { return "Build images" }

func (c *ComposeBuild) Run(w io.Writer) error {
	cmd := engine.ComposeCmd(c.engine, c.host, c.composeFile, "build")
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd.Run()
}

type ComposePull struct {
	engine      engine.Engine
	composeFile string
	host        engine.Host
}

func NewComposePull(e engine.Engine, composeFile string, host engine.Host) *ComposePull {
	return &ComposePull{engine: e, composeFile: composeFile, host: host}
}

func (c *ComposePull) Description() string { return "Pull images" }

func (c *ComposePull) Run(w io.Writer) error {
	f, err := os.Open(c.composeFile)
	if err != nil {
		return fmt.Errorf("reading compose file: %w", err)
	}
	defer f.Close() //nolint:errcheck
	services, err := compose.PullableServices(f)
	if err != nil {
		return err
	}
	if len(services) == 0 {
		return nil
	}
	args := append([]string{"pull"}, services...)
	cmd := engine.ComposeCmd(c.engine, c.host, c.composeFile, args...)
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd.Run()
}

type ComposeStop struct {
	engine      engine.Engine
	composeFile string
	host        engine.Host
}

func NewComposeStop(e engine.Engine, composeFile string, host engine.Host) *ComposeStop {
	return &ComposeStop{engine: e, composeFile: composeFile, host: host}
}

func (c *ComposeStop) Description() string { return "Stop services" }

func (c *ComposeStop) Run(w io.Writer) error {
	cmd := engine.ComposeCmd(c.engine, c.host, c.composeFile, "stop")
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd.Run()
}

type ComposeUp struct {
	engine      engine.Engine
	composeFile string
	host        engine.Host
	mode        RecreateMode
}

func NewComposeUp(e engine.Engine, composeFile string, host engine.Host, mode RecreateMode) *ComposeUp {
	return &ComposeUp{engine: e, composeFile: composeFile, host: host, mode: mode}
}

func (c *ComposeUp) Description() string { return "Start services" }

func (c *ComposeUp) Run(w io.Writer) error {
	args := []string{"up", "-d", "--no-build", "--pull", "never"}
	switch c.mode {
	case RecreateModeForce:
		args = append(args, "--force-recreate")
	case RecreateModeNone:
		args = append(args, "--no-recreate")
	}
	cmd := engine.ComposeCmd(c.engine, c.host, c.composeFile, args...)
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd.Run()
}
