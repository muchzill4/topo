package operation_test

import (
	"io"
	"testing"

	"github.com/arm/topo/internal/deploy/operation"
	"github.com/arm/topo/internal/ssh"
	"github.com/stretchr/testify/assert"
)

func TestRegistryTunnelExposureCheck(t *testing.T) {
	t.Run("returns the expected description", func(t *testing.T) {
		check := operation.NewRegistryTunnelExposureCheck(ssh.NewDestination("user@remote"), "12345")

		got := check.Description()

		assert.Equal(t, "Check registry tunnel is not exposed on remote network", got)
	})

	t.Run("delegates the exposure check", func(t *testing.T) {
		check := operation.NewRegistryTunnelExposureCheck(ssh.Destination{}, "12345")

		err := check.Run(io.Discard)

		assert.ErrorContains(t, err, "cannot conclusively rule out network access to registry port 12345")
	})
}
