package operation_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/arm/topo/internal/deploy/engine"
	"github.com/arm/topo/internal/deploy/operation"
	"github.com/arm/topo/internal/deploy/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComposePipeTransfer(t *testing.T) {
	e := engine.Docker

	t.Run("Description", func(t *testing.T) {
		h := engine.LocalHost
		tmpDir := t.TempDir()
		composeFilePath := filepath.Join(tmpDir, "compose.yaml")
		transfer := operation.NewComposePipeTransfer(e, e, composeFilePath, h, h)

		got := transfer.Description()

		assert.Equal(t, "Transfer images", got)
	})

	t.Run("Run", func(t *testing.T) {
		t.Run("transfers images from source to target", func(t *testing.T) {
			testutil.RequireLinuxDockerEngine(t)
			h := engine.LocalHost
			tmpDir := t.TempDir()
			composeFilePath := filepath.Join(tmpDir, "compose.yaml")
			dockerFilePath := filepath.Join(tmpDir, "Dockerfile")
			imageName := testutil.TestImageName(t)
			composeFileContent := fmt.Sprintf(`
services:
  test:
    build: .
    image: %s
`, imageName)
			dockerFileContent := `FROM alpine:latest`
			testutil.RequireWriteFile(t, composeFilePath, composeFileContent)
			testutil.RequireWriteFile(t, dockerFilePath, dockerFileContent)

			buildCmd := engine.ComposeCmd(e, h, composeFilePath, "build")
			buildOutput, err := buildCmd.CombinedOutput()
			require.NoError(t, err, "failed to build image: %s", string(buildOutput))

			transfer := operation.NewComposePipeTransfer(e, e, composeFilePath, h, h)

			err = transfer.Run(os.Stdout)

			require.NoError(t, err)
			testutil.RequireImageExists(t, e, h, imageName)
		})
	})
}
