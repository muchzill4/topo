package operation_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/arm/topo/internal/deploy/engine"
	"github.com/arm/topo/internal/deploy/operation"
	"github.com/arm/topo/internal/deploy/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComposeBuild(t *testing.T) {
	t.Run("Description", func(t *testing.T) {
		op := operation.NewComposeBuild(engine.Docker, "/path/to/compose.yaml", engine.LocalHost)

		assert.Equal(t, "Build images", op.Description())
	})
}

func TestComposePull(t *testing.T) {
	t.Run("Description", func(t *testing.T) {
		op := operation.NewComposePull(engine.Docker, "/path/to/compose.yaml", engine.LocalHost)

		assert.Equal(t, "Pull images", op.Description())
	})

	t.Run("Run", func(t *testing.T) {
		testutil.RequireDocker(t)

		t.Run("skips services that have a build context", func(t *testing.T) {
			tmpDir := t.TempDir()
			composeFilePath := filepath.Join(tmpDir, "compose.yaml")
			composeFileContent := `
services:
  locally-built:
    build:
      context: .
      dockerfile_inline: "FROM alpine:latest"
    image: this-image-does-not-exist-on-docker-hub
`
			testutil.RequireWriteFile(t, composeFilePath, composeFileContent)
			var buf bytes.Buffer
			op := operation.NewComposePull(engine.Docker, composeFilePath, engine.LocalHost)

			err := op.Run(&buf)

			require.NoError(t, err)
		})
	})
}

func TestComposeUp(t *testing.T) {
	t.Run("Description", func(t *testing.T) {
		op := operation.NewComposeUp(engine.Docker, "/path/to/compose.yaml", engine.LocalHost, operation.RecreateModeDefault)

		assert.Equal(t, "Start services", op.Description())
	})
}

func TestComposeStop(t *testing.T) {
	t.Run("Description", func(t *testing.T) {
		op := operation.NewComposeStop(engine.Docker, "/path/to/compose.yaml", engine.LocalHost)

		assert.Equal(t, "Stop services", op.Description())
	})
}
