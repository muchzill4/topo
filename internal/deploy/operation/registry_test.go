package operation_test

import (
	"fmt"
	"testing"

	"github.com/arm/topo/internal/deploy/engine"
	"github.com/arm/topo/internal/deploy/operation"
	"github.com/arm/topo/internal/deploy/testutil"
	goperation "github.com/arm/topo/internal/operation"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRunRegistry(t *testing.T) {
	t.Run("returns expected sequence", func(t *testing.T) {
		port := operation.DefaultRegistryPort
		e := engine.Docker

		got := operation.NewRunRegistry(e, port)

		localHost := engine.LocalHost
		want := goperation.NewSequence(
			operation.NewPull(e, localHost, "registry:2"),
			goperation.NewConditional(
				operation.NewContainerExistsPredicate(e, localHost, operation.RegistryContainerName),
				operation.NewStart(e, localHost, operation.RegistryContainerName),
				operation.NewRegistryRunWrapper(operation.NewContainerRun(e, localHost, "registry:2", operation.RegistryContainerName,
					[]string{
						"-d",
						"--restart", "always",
						"-p", fmt.Sprintf("127.0.0.1:%s:5000", port),
					},
				)),
			),
		)
		assert.Equal(t, want, got)
	})
}

func TestContainerExistsPredicate(t *testing.T) {
	t.Run("evaluates to true when container exists", func(t *testing.T) {
		testutil.RequireLinuxDockerEngine(t)
		containerName := testutil.TestContainerName(t)
		imageName := testutil.TestImageName(t)
		localHost := engine.LocalHost
		e := engine.Docker
		testutil.BuildMinimalImage(t, e, localHost, imageName)
		runCmd := engine.Cmd(e, localHost, "run", "-d", "--name", containerName, imageName)
		require.NoError(t, runCmd.Run())
		t.Cleanup(func() {
			stopCmd := engine.Cmd(e, localHost, "rm", "-f", containerName)
			_ = stopCmd.Run()
		})

		predicate := operation.NewContainerExistsPredicate(e, engine.LocalHost, containerName)
		got := predicate.Eval()

		assert.True(t, got)
	})

	t.Run("evaluates to false when container does not exist", func(t *testing.T) {
		testutil.RequireDocker(t)
		containerName := "non-existent-container-12345"

		predicate := operation.NewContainerExistsPredicate(engine.Docker, engine.LocalHost, containerName)
		got := predicate.Eval()

		assert.False(t, got)
	})
}
