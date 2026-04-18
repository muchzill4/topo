package engine_test

import (
	"testing"

	"github.com/arm/topo/internal/deploy/engine"
	"github.com/arm/topo/internal/ssh"
	"github.com/stretchr/testify/assert"
)

func TestRun(t *testing.T) {
	t.Run("builds command with engine binary", func(t *testing.T) {
		dest := ssh.NewDestination("ssh://user@remote")
		host := engine.NewHostFromDestination(dest)
		cmd := engine.Cmd(engine.Docker, host, "save", "alpine:latest")

		got := engine.String(cmd)

		assert.Equal(t, "docker -H ssh://user@remote save alpine:latest", got)
	})

	t.Run("uses podman binary", func(t *testing.T) {
		cmd := engine.Cmd(engine.Podman, engine.LocalHost, "pull", "alpine:latest")

		got := engine.String(cmd)

		assert.Equal(t, "podman pull alpine:latest", got)
	})
}

func TestCompose(t *testing.T) {
	t.Run("builds compose command with engine binary", func(t *testing.T) {
		dest := ssh.NewDestination("ssh://user@remote")
		host := engine.NewHostFromDestination(dest)
		cmd := engine.ComposeCmd(engine.Docker, host, "/path/to/compose.yaml", "up", "-d")

		got := engine.String(cmd)

		assert.Equal(t, "docker -H ssh://user@remote compose -f /path/to/compose.yaml up -d", got)
	})

	t.Run("uses nerdctl binary", func(t *testing.T) {
		cmd := engine.ComposeCmd(engine.Nerdctl, engine.LocalHost, "compose.yaml", "build")

		got := engine.String(cmd)

		assert.Equal(t, "nerdctl compose -f compose.yaml build", got)
	})
}
