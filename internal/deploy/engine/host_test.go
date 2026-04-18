package engine_test

import (
	"testing"

	"github.com/arm/topo/internal/deploy/engine"
	"github.com/arm/topo/internal/ssh"
	"github.com/stretchr/testify/assert"
)

func TestNewHostFromDestination(t *testing.T) {
	t.Run("gets new host from destination", func(t *testing.T) {
		dest := ssh.NewDestination("ssh://user@remote")

		got := engine.NewHostFromDestination(dest)

		dontwant := engine.LocalHost
		assert.NotEqual(t, dontwant, got)
	})

	t.Run("gets localhost when given localhost Destination", func(t *testing.T) {
		got := engine.NewHostFromDestination(ssh.PlainLocalhost)

		want := engine.LocalHost
		assert.Equal(t, want, got)
	})
}
