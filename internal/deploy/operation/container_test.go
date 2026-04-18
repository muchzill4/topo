package operation_test

import (
	"bytes"
	"testing"

	"github.com/arm/topo/internal/deploy/engine"
	"github.com/arm/topo/internal/deploy/operation"
	"github.com/arm/topo/internal/deploy/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPull(t *testing.T) {
	t.Run("Description", func(t *testing.T) {
		op := operation.NewPull(engine.Docker, engine.LocalHost, "nginx:latest")

		assert.Equal(t, "Pull image nginx:latest", op.Description())
	})

	t.Run("Run", func(t *testing.T) {
		testutil.RequireDocker(t)

		t.Run("pulls an image", func(t *testing.T) {
			var buf bytes.Buffer
			op := operation.NewPull(engine.Docker, engine.LocalHost, "alpine:latest")

			err := op.Run(&buf)

			require.NoError(t, err)
		})
	})
}

func TestStart(t *testing.T) {
	t.Run("Description", func(t *testing.T) {
		op := operation.NewStart(engine.Docker, engine.LocalHost, "my-container")

		assert.Equal(t, "Start container my-container", op.Description())
	})
}

func TestContainerRun(t *testing.T) {
	t.Run("Description", func(t *testing.T) {
		op := operation.NewContainerRun(engine.Docker, engine.LocalHost, "alpine:latest", "test-container", []string{})

		assert.Equal(t, "Run image alpine:latest as container test-container", op.Description())
	})
}
