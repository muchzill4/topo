package deploy_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/arm/topo/internal/deploy"
	"github.com/arm/topo/internal/deploy/engine"
	"github.com/arm/topo/internal/deploy/operation"
	"github.com/arm/topo/internal/deploy/testutil"
	goperation "github.com/arm/topo/internal/operation"
	"github.com/arm/topo/internal/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDeployment(t *testing.T) {
	composeFile := "compose.yaml"
	e := engine.Docker

	t.Run("includes transfer operation for remote host", func(t *testing.T) {
		remoteDest := ssh.NewDestination("user@remote")
		deployOpts := deploy.DeployOptions{TargetHost: remoteDest, Engine: e}

		got, _ := deploy.NewDeployment(composeFile, deployOpts)

		remoteHost := engine.NewHostFromDestination(remoteDest)
		localHost := engine.LocalHost
		want := goperation.Sequence{
			operation.NewComposeBuild(e, composeFile, localHost),
			operation.NewComposePull(e, composeFile, localHost),
			operation.NewComposePipeTransfer(e, composeFile, localHost, remoteHost),
			operation.NewComposeUp(e, composeFile, remoteHost, operation.RecreateModeDefault),
		}
		assert.Equal(t, want, got)
	})

	t.Run("includes registry operations for remote host when enabled", func(t *testing.T) {
		remoteDest := ssh.NewDestination("user@remote")
		port := operation.DefaultRegistryPort
		opts := deploy.DeployOptions{TargetHost: remoteDest, Registry: &deploy.RegistryConfig{Port: port, UseControlSockets: true}, Engine: e}

		got, _ := deploy.NewDeployment(composeFile, opts)

		remoteHost := engine.NewHostFromDestination(remoteDest)
		localHost := engine.LocalHost
		want := goperation.Sequence{
			operation.NewComposeBuild(e, composeFile, localHost),
			operation.NewComposePull(e, composeFile, localHost),
		}
		want = append(want, operation.NewRunRegistry(e, port)...)
		wantTunnelStart, wantSecurityCheck, wantTunnelStop := ssh.NewSSHTunnel(remoteDest, port, opts.Registry.UseControlSockets)
		want = append(want,
			wantTunnelStart,
			wantSecurityCheck,
			operation.NewRegistryTransfer(e, composeFile, localHost, remoteHost, port),
			wantTunnelStop,
			operation.NewComposeUp(e, composeFile, remoteHost, operation.RecreateModeDefault),
		)
		assert.Equal(t, want, got)
	})

	t.Run("excludes transfer operation for local host", func(t *testing.T) {
		tests := []struct {
			name         string
			recreateMode operation.RecreateMode
		}{
			{
				name:         "default",
				recreateMode: operation.RecreateModeDefault,
			},
			{
				name:         "force recreate",
				recreateMode: operation.RecreateModeForce,
			},
			{
				name:         "no recreate",
				recreateMode: operation.RecreateModeNone,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				deployOpts := deploy.DeployOptions{
					TargetHost:   ssh.PlainLocalhost,
					RecreateMode: tt.recreateMode,
					Engine:       e,
				}

				got, _ := deploy.NewDeployment(composeFile, deployOpts)

				localHost := engine.LocalHost
				want := goperation.Sequence{
					operation.NewComposeBuild(e, composeFile, localHost),
					operation.NewComposePull(e, composeFile, localHost),
					operation.NewComposeUp(e, composeFile, localHost, tt.recreateMode),
				}
				assert.Equal(t, want, got)
			})
		}
	})

	t.Run("returns an SSH tunnel cleanup operation for remote host", func(t *testing.T) {
		remoteHost := ssh.NewDestination("user@remote")
		deployOpts := deploy.DeployOptions{TargetHost: remoteHost, Registry: &deploy.RegistryConfig{UseControlSockets: true}, Engine: e}

		_, cleanup := deploy.NewDeployment(composeFile, deployOpts)

		want := ssh.NewSSHTunnelStop(remoteHost)
		assert.Equal(t, want, cleanup)
	})

	t.Run("does not return an SSH tunnel cleanup operation for local host", func(t *testing.T) {
		localHost := ssh.PlainLocalhost
		deployOpts := deploy.DeployOptions{TargetHost: localHost, Registry: &deploy.RegistryConfig{}, Engine: e}

		_, cleanup := deploy.NewDeployment(composeFile, deployOpts)

		var want goperation.Operation = nil
		assert.Equal(t, want, cleanup)
	})

	t.Run("does not use SSH control sockets when disabled", func(t *testing.T) {
		remoteDest := ssh.NewDestination("user@remote")
		port := operation.DefaultRegistryPort
		opts := deploy.DeployOptions{TargetHost: remoteDest, Registry: &deploy.RegistryConfig{Port: port, UseControlSockets: false}, Engine: e}

		got, _ := deploy.NewDeployment(composeFile, opts)

		wantTunnelStart, wantSecurityCheck, wantTunnelEnd := ssh.NewSSHTunnel(remoteDest, opts.Registry.Port, opts.Registry.UseControlSockets)
		localHost := engine.LocalHost
		remoteHost := engine.NewHostFromDestination(remoteDest)
		want := goperation.Sequence{
			operation.NewComposeBuild(e, composeFile, localHost),
			operation.NewComposePull(e, composeFile, localHost),
		}
		want = append(want, operation.NewRunRegistry(e, port)...)
		want = append(want,
			wantTunnelStart,
			wantSecurityCheck,
			operation.NewRegistryTransfer(e, composeFile, localHost, remoteHost, port),
			wantTunnelEnd,
			operation.NewComposeUp(e, composeFile, remoteHost, operation.RecreateModeDefault),
		)
		assert.Equal(t, want, got)
	})
}

func TestDeployment(t *testing.T) {
	testutil.RequireDocker(t)

	t.Run("Run", func(t *testing.T) {
		container := testutil.StartContainer(t, testutil.DinDContainer)

		t.Run("builds images, transfers them, and starts services", func(t *testing.T) {
			e := engine.Docker
			remoteDockerHost := ssh.NewDestination(container.SSHDestination)
			tmpDir := t.TempDir()
			dockerFilePath := filepath.Join(tmpDir, "Dockerfile")
			dockerFileContent := `
FROM alpine:latest
CMD ["tail", "-f", "/dev/null"]
`
			testutil.RequireWriteFile(t, dockerFilePath, dockerFileContent)
			composeFilePath := filepath.Join(tmpDir, "compose.yaml")
			composeFileContent := fmt.Sprintf(`
name: %s
services:
  busybox:
    image: busybox
    command: ["tail", "-f", "/dev/null"]
  a-service:
    build: .
`, testutil.TestProjectName(t))
			testutil.RequireWriteFile(t, composeFilePath, composeFileContent)
			t.Cleanup(func() { testutil.ForceComposeDown(t, e, composeFilePath) })

			deployOpts := deploy.DeployOptions{TargetHost: remoteDockerHost, Engine: e}
			d, _ := deploy.NewDeployment(composeFilePath, deployOpts)
			err := d.Run(os.Stdout)

			require.NoError(t, err)
			testutil.AssertContainersRunning(t, e, remoteDockerHost, composeFilePath)
		})
	})
}
