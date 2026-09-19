package health_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/arm/topo/internal/health"
	"github.com/arm/topo/internal/runner"
	"github.com/arm/topo/internal/ssh"
	"github.com/stretchr/testify/assert"
)

func TestNewTargetSpecifiedDependency(t *testing.T) {
	t.Run("reports a missing target with the configured fix", func(t *testing.T) {
		dependency := health.NewTargetSpecifiedDependency(nil, "Specify a target")

		got := dependency.Check(context.Background())

		want := health.DependencyCheckResult{Failure: &health.DependencyCheckFailure{
			Severity: health.SeverityError,
			Message:  "target not specified",
			Fix:      &health.Fix{Description: "Specify a target"},
		}}
		assert.Equal(t, want, got)
	})

	t.Run("succeeds when a target is specified", func(t *testing.T) {
		target := ssh.NewDestination("localhost")
		dependency := health.NewTargetSpecifiedDependency(&target, "")

		got := dependency.Check(context.Background())

		assert.Equal(t, health.DependencyCheckResult{SuccessValue: "ssh://localhost"}, got)
	})
}

func TestNewConnectivityDependency(t *testing.T) {
	t.Run("does not construct SSH operations for plain localhost", func(t *testing.T) {
		target := ssh.NewDestination("localhost")
		calls := 0
		dependency := health.NewConnectivityDependency(&target, func(ssh.Destination) health.ConnectivityOperations {
			calls++
			return health.ConnectivityOperations{}
		})

		got := dependency.Check(context.Background())

		assert.Equal(t, health.DependencyCheckResult{SuccessValue: "local"}, got)
		assert.Zero(t, calls)
	})

	t.Run("uses injected known hosts entry for changed host keys", func(t *testing.T) {
		target := ssh.NewDestination("user@example.com")
		dependency := health.NewConnectivityDependency(&target, func(ssh.Destination) health.ConnectivityOperations {
			return health.ConnectivityOperations{
				Authenticate:    func(context.Context) error { return ssh.ErrHostKeyChanged },
				KnownHostsEntry: func() (string, error) { return "[example.com]:2222", nil },
			}
		})

		got := dependency.Check(context.Background())

		want := health.DependencyCheckResult{Failure: &health.DependencyCheckFailure{
			Severity: health.SeverityError,
			Message:  "host key has changed",
			Fix: &health.Fix{
				Description: "Remove the old SSH host key from known_hosts, then retry",
				Command:     "ssh-keygen -R '[example.com]:2222'",
			},
		}}
		assert.Equal(t, want, got)
	})

	t.Run("omits the removal command when the known hosts entry cannot be resolved", func(t *testing.T) {
		target := ssh.NewDestination("user@example.com")
		dependency := health.NewConnectivityDependency(&target, func(ssh.Destination) health.ConnectivityOperations {
			return health.ConnectivityOperations{
				Authenticate:    func(context.Context) error { return ssh.ErrHostKeyChanged },
				KnownHostsEntry: func() (string, error) { return "", errors.New("cannot load SSH config") },
			}
		})

		got := dependency.Check(context.Background())

		want := health.DependencyCheckResult{Failure: &health.DependencyCheckFailure{
			Severity: health.SeverityError,
			Message:  "host key has changed",
			Fix: &health.Fix{
				Description: "Remove the old SSH host key from known_hosts, then retry",
			},
		}}
		assert.Equal(t, want, got)
	})

	t.Run("returns setup keys advice for authentication failures", func(t *testing.T) {
		target := ssh.NewDestination("user@example.com")
		dependency := health.NewConnectivityDependency(&target, func(ssh.Destination) health.ConnectivityOperations {
			return health.ConnectivityOperations{Authenticate: func(context.Context) error { return ssh.ErrAuthFailed }}
		})

		got := dependency.Check(context.Background())

		want := health.DependencyCheckResult{Failure: &health.DependencyCheckFailure{
			Severity: health.SeverityError,
			Message:  "authentication failed",
			Fix: &health.Fix{
				Description: "Configure SSH keys on remote target",
				Command:     "topo setup-keys --target ssh://user@example.com",
			},
		}}
		assert.Equal(t, want, got)
	})

	t.Run("returns setup keys advice for too many authentication failures", func(t *testing.T) {
		target := ssh.NewDestination("user@example.com")
		dependency := health.NewConnectivityDependency(&target, func(ssh.Destination) health.ConnectivityOperations {
			return health.ConnectivityOperations{Authenticate: func(context.Context) error { return ssh.ErrTooManyAuthFails }}
		})

		got := dependency.Check(context.Background())

		want := health.DependencyCheckResult{Failure: &health.DependencyCheckFailure{
			Severity: health.SeverityError,
			Message:  "too many authentication failures",
			Fix: &health.Fix{
				Description: "Configure SSH keys on remote target",
				Command:     "topo setup-keys --target ssh://user@example.com",
			},
		}}
		assert.Equal(t, want, got)
	})

	t.Run("returns host key trust advice for unknown host keys", func(t *testing.T) {
		target := ssh.NewDestination("user@example.com")
		dependency := health.NewConnectivityDependency(&target, func(ssh.Destination) health.ConnectivityOperations {
			return health.ConnectivityOperations{Authenticate: func(context.Context) error { return ssh.ErrHostKeyUnknown }}
		})

		got := dependency.Check(context.Background())

		want := health.DependencyCheckResult{Failure: &health.DependencyCheckFailure{
			Severity: health.SeverityError,
			Message:  "host key is unknown",
			Fix: &health.Fix{
				Description: "Trust the target's SSH host key",
				Command:     "topo health --target ssh://user@example.com --accept-new-host-keys",
			},
		}}
		assert.Equal(t, want, got)
	})
}

func TestNewDependencyOnRemoteDocker(t *testing.T) {
	t.Run("does not probe the target when Docker is missing on the host", func(t *testing.T) {
		probeCalled := false
		target := ssh.NewDestination("user@example.com")
		dependency := health.NewDependencyOnRemoteDocker(&target, &runner.Fake{}, func(context.Context, ssh.Destination) error {
			probeCalled = true
			return nil
		})

		got := dependency.Check(context.Background())

		assert.False(t, probeCalled)
		assert.Equal(t, health.DependencyCheckResult{Failure: &health.DependencyCheckFailure{
			Severity: health.SeverityError,
			Message:  `cannot probe from host: "docker" not found in $PATH`,
			Fix:      &health.Fix{Description: "Install a supported container engine on the host. See https://github.com/arm/topo#install-a-container-engine"},
		}}, got)
	})
}

func TestNewDependencyOnSSHCheck(t *testing.T) {
	buildRunner := func(sshVResult runner.FakeResult) runner.Runner {
		return &runner.Fake{
			Binaries: []string{"ssh"},
			Commands: map[string]runner.FakeResult{"ssh -V": sshVResult},
		}
	}

	t.Run("accepts OpenSSH", func(t *testing.T) {
		dependency := health.NewDependencyOnSSH(buildRunner(runner.FakeResult{Stderr: "OpenSSH_9.9p1, OpenSSL 3.4.0"}))

		got := dependency.Check(context.Background())

		assert.Equal(t, health.DependencyCheckResult{SuccessValue: "ssh"}, got)
	})

	t.Run("rejects another SSH implementation", func(t *testing.T) {
		dependency := health.NewDependencyOnSSH(buildRunner(runner.FakeResult{Stderr: "Dropbear v2025.88"}))

		got := dependency.Check(context.Background())

		want := health.DependencyCheckResult{Failure: &health.DependencyCheckFailure{
			Message: `"ssh" does not resolve to OpenSSH: Dropbear v2025.88`,
			Fix:     &health.Fix{Description: "Install OpenSSH and ensure its ssh executable is first on PATH"},
		}}
		assert.Equal(t, want, got)
	})

	t.Run("fails when the version cannot be checked", func(t *testing.T) {
		versionErr := errors.New("version check failed")
		dependency := health.NewDependencyOnSSH(buildRunner(runner.FakeResult{Err: versionErr}))

		got := dependency.Check(context.Background())

		assert.Equal(t, health.DependencyCheckResult{Failure: &health.DependencyCheckFailure{Message: versionErr.Error()}}, got)
	})
}

func TestNewDependencyOnDockerComposeCheck(t *testing.T) {
	buildRunner := func(version string) runner.Runner {
		return &runner.Fake{Commands: map[string]runner.FakeResult{
			"docker compose version --format json": {Output: `{"version": "` + version + `"}`},
		}}
	}

	t.Run("accepts Docker Compose at the minimum version", func(t *testing.T) {
		dependency := health.NewDependencyOnDockerCompose(buildRunner("2.21.0"))

		got := dependency.Check(context.Background())

		assert.Equal(t, health.DependencyCheckResult{SuccessValue: "docker compose"}, got)
	})

	t.Run("accepts Docker Compose newer than the minimum version", func(t *testing.T) {
		dependency := health.NewDependencyOnDockerCompose(buildRunner("5.2.0"))

		got := dependency.Check(context.Background())

		assert.Equal(t, health.DependencyCheckResult{SuccessValue: "docker compose"}, got)
	})

	t.Run("returns an upgrade fix when Docker Compose is too old", func(t *testing.T) {
		dependency := health.NewDependencyOnDockerCompose(buildRunner("v1.9.0"))

		got := dependency.Check(context.Background())

		want := health.DependencyCheckResult{Failure: &health.DependencyCheckFailure{
			Message: "installed docker compose version v1.9.0 is older than required version 2.21.0",
			Fix:     &health.Fix{Description: "Upgrade Docker Compose to version 2.21.0 or later. See https://github.com/arm/topo#install-a-container-engine"},
		}}
		assert.Equal(t, want, got)
	})
}

func TestRemoteprocDependency(t *testing.T) {
	buildRunnerWithRemoteProcs := func(names []string) runner.Runner {
		return &runner.Fake{Commands: map[string]runner.FakeResult{
			"cat /sys/class/remoteproc/*/name": {Output: strings.Join(names, "\n")},
		}}
	}

	t.Run("fails when no remoteproc devices are found", func(t *testing.T) {
		r := buildRunnerWithRemoteProcs(nil)
		target := ssh.NewDestination("localhost")
		dependency := health.NewDependencyOnRemoteproc(&target, func(ssh.Destination) runner.Runner { return r })

		got := dependency.Check(context.Background())

		want := health.DependencyCheckResult{
			Failure: &health.DependencyCheckFailure{
				Severity: health.SeverityInfo,
				Message:  "no remoteproc devices found",
			},
		}
		assert.Equal(t, want, got)
	})

	t.Run("fails when remoteproc probe fails", func(t *testing.T) {
		r := &runner.Fake{Commands: map[string]runner.FakeResult{
			"cat /sys/class/remoteproc/*/name": {Err: runner.ErrTimeout},
		}}
		target := ssh.NewDestination("localhost")
		dependency := health.NewDependencyOnRemoteproc(&target, func(ssh.Destination) runner.Runner { return r })

		got := dependency.Check(context.Background())

		want := health.DependencyCheckResult{
			Failure: &health.DependencyCheckFailure{
				Severity: health.SeverityError,
				Message:  "timed out",
			},
		}
		assert.Equal(t, want, got)
	})

	t.Run("reports remoteproc device names", func(t *testing.T) {
		r := buildRunnerWithRemoteProcs([]string{"m4_0", "m4_1"})
		target := ssh.NewDestination("localhost")
		dependency := health.NewDependencyOnRemoteproc(&target, func(ssh.Destination) runner.Runner { return r })

		got := dependency.Check(context.Background())

		assert.Equal(t, health.DependencyCheckResult{SuccessValue: "m4_0, m4_1"}, got)
	})
}

func TestRemoteprocRuntimeDependency(t *testing.T) {
	t.Run("includes an install fix with the target", func(t *testing.T) {
		target := ssh.NewDestination("user@my-target")
		dependency := health.NewDependencyOnRemoteprocRuntime(&target, func(ssh.Destination) runner.Runner { return &runner.Fake{} })

		result := dependency.Check(context.Background())

		assert.Equal(t, &health.DependencyCheckFailure{
			Severity: health.SeverityWarning,
			Message:  `"remoteproc-runtime" not found in $PATH`,
			Fix: &health.Fix{
				Description: "Install the Remoteproc Runtime",
				Command:     "topo install remoteproc-runtime --target ssh://user@my-target",
			},
		}, result.Failure)
	})
}

func TestRemoteprocRuntimeShimDependency(t *testing.T) {
	t.Run("includes an install fix with the target", func(t *testing.T) {
		target := ssh.NewDestination("user@my-target")
		dependency := health.NewDependencyOnRemoteprocRuntimeShim(&target, func(ssh.Destination) runner.Runner {
			return &runner.Fake{}
		})

		got := dependency.Check(context.Background())

		assert.Equal(t, health.DependencyCheckResult{Failure: &health.DependencyCheckFailure{
			Severity: health.SeverityWarning,
			Message:  `"containerd-shim-remoteproc-v1" not found in $PATH`,
			Fix: &health.Fix{
				Description: "Install the Remoteproc Runtime",
				Command:     "topo install remoteproc-runtime --target ssh://user@my-target",
			},
		}}, got)
	})
}

const podmanInstallURL = "https://github.com/arm/topo#install-a-container-engine"

func TestPodmanDependency(t *testing.T) {
	t.Run("succeeds after finding the binary and probing info", func(t *testing.T) {
		dependency := health.NewDependencyOnPodman(&runner.Fake{
			Binaries: []string{"podman"},
			Commands: map[string]runner.FakeResult{"podman info": {}},
		})

		got := dependency.Check(t.Context())

		assert.Equal(t, health.DependencyCheckResult{SuccessValue: "podman"}, got)
	})

	t.Run("stops when the binary is missing", func(t *testing.T) {
		dependency := health.NewDependencyOnPodman(&runner.Fake{})

		got := dependency.Check(t.Context())

		want := failure(`"podman" not found in $PATH`, "Install a supported container engine. See "+podmanInstallURL)
		assert.Equal(t, want, got)
	})

	t.Run("reports an info failure", func(t *testing.T) {
		dependency := health.NewDependencyOnPodman(&runner.Fake{
			Binaries: []string{"podman"},
			Commands: map[string]runner.FakeResult{"podman info": {Err: errors.New("permission denied")}},
		})

		got := dependency.Check(t.Context())

		want := failure("permission denied", "Ensure current user can run podman commands. See "+podmanInstallURL)
		assert.Equal(t, want, got)
	})
}

func TestNewDependencyOnPodmanCompose(t *testing.T) {
	t.Run("reports an available Compose provider", func(t *testing.T) {
		calls := []string{}
		dependency := health.NewDependencyOnPodmanCompose(composeOperations(&calls, nil))

		got := dependency.Check(t.Context())

		assert.Equal(t, health.DependencyCheckResult{SuccessValue: "docker-compose"}, got)
		assert.Equal(t, []string{"compose:version"}, calls)
	})

	t.Run("reports an unavailable Compose provider", func(t *testing.T) {
		calls := []string{}
		dependency := health.NewDependencyOnPodmanCompose(composeOperations(&calls, errors.New("version failed")))

		got := dependency.Check(t.Context())

		assert.Equal(t, failure("version failed", composeFix), got)
		assert.Equal(t, []string{"compose:version"}, calls)
	})
}

func TestNewDependencyOnRemotePodman(t *testing.T) {
	t.Run("runs probes in order and closes the tunnel", func(t *testing.T) {
		calls := []string{}
		target := ssh.NewDestination("user@example.com")
		dependency := health.NewDependencyOnRemotePodman(&target, targetOperations(&calls, nil, nil, nil, nil, nil))

		got := dependency.Check(t.Context())

		assert.Equal(t, health.DependencyCheckResult{SuccessValue: "podman"}, got)
		assert.Equal(t, []string{"host-binary:podman", "compose:version", "binary:podman", "resolve", "open:/run/podman.sock", "info:tunnel", "compose:ls", "close"}, calls)
	})

	t.Run("reports a missing host Podman binary", func(t *testing.T) {
		calls := []string{}
		target := ssh.NewDestination("user@example.com")
		operations := targetOperations(&calls, nil, nil, nil, nil, nil)
		operations.HostRunner = recordingRunner{calls: &calls, prefix: "host-", binaryErr: errors.New("podman missing")}
		dependency := health.NewDependencyOnRemotePodman(&target, operations)

		got := dependency.Check(t.Context())

		assert.Equal(t, failure("cannot probe from host: podman missing", "Install a supported container engine on the host. See "+podmanInstallURL), got)
		assert.Equal(t, []string{"host-binary:podman"}, calls)
	})

	t.Run("reports an unavailable host Compose provider", func(t *testing.T) {
		calls := []string{}
		target := ssh.NewDestination("user@example.com")
		operations := targetOperations(&calls, nil, nil, nil, nil, nil)
		operations.Commands.ComposeVersion = func(context.Context) error {
			calls = append(calls, "compose:version")
			return errors.New("provider missing")
		}
		dependency := health.NewDependencyOnRemotePodman(&target, operations)

		got := dependency.Check(t.Context())

		assert.Equal(t, failure("cannot probe from host: provider missing", "Install the docker-compose provider on the host. See "+podmanInstallURL), got)
		assert.Equal(t, []string{"host-binary:podman", "compose:version"}, calls)
	})

	t.Run("stops when Podman is missing", func(t *testing.T) {
		calls := []string{}
		target := ssh.NewDestination("user@example.com")
		dependency := health.NewDependencyOnRemotePodman(&target, targetOperations(&calls, errors.New("podman missing"), nil, nil, nil, nil))

		got := dependency.Check(t.Context())

		assert.Equal(t, failure("podman missing", "Install Podman on the target. See "+podmanInstallURL), got)
		assert.Equal(t, []string{"host-binary:podman", "compose:version", "binary:podman"}, calls)
	})

	t.Run("stops when remote socket resolution fails", func(t *testing.T) {
		calls := []string{}
		target := ssh.NewDestination("user@example.com")
		dependency := health.NewDependencyOnRemotePodman(&target, targetOperations(&calls, nil, errors.New("no remote socket"), nil, nil, nil))

		got := dependency.Check(t.Context())

		assert.Equal(t, failure("no remote socket", "Start the Podman API socket and ensure the SSH user can access it. See "+podmanInstallURL), got)
		assert.Equal(t, []string{"host-binary:podman", "compose:version", "binary:podman", "resolve"}, calls)
	})

	t.Run("reports a tunnel open failure", func(t *testing.T) {
		calls := []string{}
		target := ssh.NewDestination("user@example.com")
		dependency := health.NewDependencyOnRemotePodman(&target, targetOperations(&calls, nil, nil, errors.New("forwarding denied"), nil, nil))

		got := dependency.Check(t.Context())

		assert.Equal(t, failure("forwarding denied", "Ensure SSH permits local forwarding to the target Podman API socket."), got)
		assert.Equal(t, []string{"host-binary:podman", "compose:version", "binary:podman", "resolve", "open:/run/podman.sock"}, calls)
	})

	t.Run("closes the tunnel after an info failure", func(t *testing.T) {
		calls := []string{}
		target := ssh.NewDestination("user@example.com")
		dependency := health.NewDependencyOnRemotePodman(&target, targetOperations(&calls, nil, nil, nil, errors.New("info failed"), nil))

		got := dependency.Check(t.Context())

		assert.Equal(t, failure("info failed", "Ensure the Podman API socket at /run/podman.sock is functional and accessible to the SSH user."), got)
		assert.Equal(t, []string{"host-binary:podman", "compose:version", "binary:podman", "resolve", "open:/run/podman.sock", "info:tunnel", "close"}, calls)
	})

	t.Run("closes the tunnel after a Compose failure", func(t *testing.T) {
		calls := []string{}
		target := ssh.NewDestination("user@example.com")
		dependency := health.NewDependencyOnRemotePodman(&target, targetOperations(&calls, nil, nil, nil, nil, errors.New("compose failed")))

		got := dependency.Check(t.Context())

		assert.Equal(t, failure("compose failed", "Ensure the Podman API socket at /run/podman.sock is functional and accessible to the SSH user."), got)
		assert.Equal(t, []string{"host-binary:podman", "compose:version", "binary:podman", "resolve", "open:/run/podman.sock", "info:tunnel", "compose:ls", "close"}, calls)
	})

	t.Run("reports a cleanup failure after successful probes", func(t *testing.T) {
		calls := []string{}
		target := ssh.NewDestination("user@example.com")
		dependency := health.NewDependencyOnRemotePodman(&target, targetOperations(&calls, nil, nil, nil, nil, nil, errors.New("close failed")))

		got := dependency.Check(t.Context())

		assert.Equal(t, failure("failed to close remote Podman socket tunnel: close failed", "Close the failed Podman SSH tunnel, then try again."), got)
	})

	t.Run("prefers a probe failure over a cleanup failure", func(t *testing.T) {
		calls := []string{}
		target := ssh.NewDestination("user@example.com")
		dependency := health.NewDependencyOnRemotePodman(&target, targetOperations(&calls, nil, nil, nil, errors.New("info failed"), nil, errors.New("close failed")))

		got := dependency.Check(t.Context())

		assert.Equal(t, failure("info failed", "Ensure the Podman API socket at /run/podman.sock is functional and accessible to the SSH user."), got)
		assert.Equal(t, []string{"host-binary:podman", "compose:version", "binary:podman", "resolve", "open:/run/podman.sock", "info:tunnel", "close"}, calls)
	})

	t.Run("does not invoke remote operations for plain localhost", func(t *testing.T) {
		calls := []string{}
		target := ssh.NewDestination("localhost")
		dependency := health.NewDependencyOnRemotePodman(&target, targetOperations(&calls, nil, nil, nil, nil, nil))

		got := dependency.Check(t.Context())

		assert.Equal(t, health.DependencyCheckResult{SuccessValue: "podman"}, got)
		assert.Equal(t, []string{"host-binary:podman", "compose:version"}, calls)
	})
}

const composeFix = "Ensure Podman, its API socket, and the docker-compose provider are available. See " + podmanInstallURL

func failure(message, fix string) health.DependencyCheckResult {
	return health.DependencyCheckResult{Failure: &health.DependencyCheckFailure{
		Severity: health.SeverityError,
		Message:  message,
		Fix:      &health.Fix{Description: fix},
	}}
}

func composeOperations(calls *[]string, versionErr error) health.PodmanComposeOperations {
	return health.PodmanComposeOperations{
		Commands: health.PodmanCommands{
			ComposeVersion: func(context.Context) error {
				*calls = append(*calls, "compose:version")
				return versionErr
			},
		},
	}
}

func targetOperations(calls *[]string, binaryErr, resolveErr, openErr, infoErr, composeErr error, closeErr ...error) health.TargetPodmanOperations {
	return health.TargetPodmanOperations{
		HostRunner: recordingRunner{calls: calls, prefix: "host-"},
		Runner: func(ssh.Destination) runner.Runner {
			return recordingRunner{calls: calls, binaryErr: binaryErr}
		},
		ResolveRemoteSocket: func(context.Context, ssh.Destination) (string, error) {
			*calls = append(*calls, "resolve")
			return "/run/podman.sock", resolveErr
		},
		OpenTunnel: func(_ context.Context, _ ssh.Destination, socketPath string) (health.SocketTunnel, error) {
			*calls = append(*calls, "open:"+socketPath)
			if openErr != nil {
				return nil, openErr
			}
			var err error
			if len(closeErr) > 0 {
				err = closeErr[0]
			}
			return recordingTunnel{calls: calls, closeErr: err}, nil
		},
		Commands: health.PodmanCommands{
			ComposeVersion: func(context.Context) error {
				*calls = append(*calls, "compose:version")
				return nil
			},
			Info: func(_ context.Context, socketURL string) error {
				*calls = append(*calls, "info:"+socketURL)
				return infoErr
			},
			Compose: func(_ context.Context, socketURL string, args ...string) error {
				*calls = append(*calls, "compose:"+args[0])
				return composeErr
			},
		},
	}
}

type recordingRunner struct {
	calls     *[]string
	prefix    string
	binaryErr error
}

func (r recordingRunner) BinaryExists(_ context.Context, binary string) error {
	*r.calls = append(*r.calls, r.prefix+"binary:"+binary)
	return r.binaryErr
}

func (recordingRunner) Run(context.Context, string) (string, string, error) { return "", "", nil }
func (r recordingRunner) RunWithStdin(ctx context.Context, command string, _ []byte) (string, string, error) {
	return r.Run(ctx, command)
}

type recordingTunnel struct {
	calls    *[]string
	closeErr error
}

func (recordingTunnel) SocketURL() string { return "tunnel" }
func (t recordingTunnel) Close() error {
	*t.calls = append(*t.calls, "close")
	return t.closeErr
}
