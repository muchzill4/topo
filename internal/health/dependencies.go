package health

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/arm/topo/internal/command"
	"github.com/arm/topo/internal/deploy/docker"
	"github.com/arm/topo/internal/deploy/podman"
	"github.com/arm/topo/internal/output/logger"
	"github.com/arm/topo/internal/probe"
	"github.com/arm/topo/internal/runner"
	"github.com/arm/topo/internal/ssh"
	"github.com/arm/topo/internal/upgrade"
	"github.com/arm/topo/internal/version"
)

const containerEngineInstallURL = "https://github.com/arm/topo#install-a-container-engine"

type DependencyID string

const (
	DependencyIDConnectivity DependencyID = "target-connectivity"
	DependencyIDRemoteproc   DependencyID = "remoteproc"
)

type Dependency struct {
	Label string
	Check DependencyCheckFn
	// Used to maintain legacy JSON output
	ID DependencyID
}

type DependencyCheckFn func(ctx context.Context) DependencyCheckResult

type DependencyCheckResult struct {
	SuccessValue string
	Failure      *DependencyCheckFailure
}

type DependencyCheckFailure struct {
	Severity CheckSeverity
	Message  string
	Fix      *Fix
}

type CheckSeverity int

const (
	SeverityError CheckSeverity = iota
	SeverityWarning
	SeverityInfo
)

type Fix struct {
	Description string
	Command     string
}

func NewDependencyOnSSH(r runner.Runner) Dependency {
	return Dependency{
		Label: "OpenSSH",
		Check: func(ctx context.Context) DependencyCheckResult {
			if err := r.BinaryExists(ctx, "ssh"); err != nil {
				return DependencyCheckResult{Failure: &DependencyCheckFailure{Severity: SeverityError, Message: err.Error()}}
			}
			_, stderr, err := r.Run(ctx, "ssh -V")
			if err != nil {
				return DependencyCheckResult{Failure: &DependencyCheckFailure{Message: err.Error()}}
			}
			if !strings.Contains(stderr, "OpenSSH_") {
				return DependencyCheckResult{Failure: &DependencyCheckFailure{
					Message: fmt.Sprintf("%q does not resolve to OpenSSH: %s", "ssh", stderr),
					Fix: &Fix{
						Description: "Install OpenSSH and ensure its ssh executable is first on PATH",
					},
				}}
			}
			return DependencyCheckResult{SuccessValue: "ssh"}
		},
	}
}

func NewDependencyOnTopo(skipVersionChecks bool) Dependency {
	return Dependency{
		Label: "Topo",
		Check: func(ctx context.Context) DependencyCheckResult {
			if skipVersionChecks || version.Version == version.Dev {
				return DependencyCheckResult{SuccessValue: "topo"}
			}
			binPath, binPathErr := upgrade.CurrentBinaryPath()

			var latest string
			var err error
			if binPathErr == nil && upgrade.IsBinaryManagedByHomebrew(binPath) {
				latest, err = version.FetchLatestHomebrew(ctx, version.HomebrewFormulaURL)
			} else {
				latest, err = version.FetchLatestArtifactory(ctx, version.ArtifactoryBaseURL)
			}
			if err != nil {
				logger.Warn(fmt.Sprintf("failed to fetch latest version: %v", err))
				return DependencyCheckResult{SuccessValue: "topo"}
			}
			if latest == version.Version {
				return DependencyCheckResult{SuccessValue: "topo"}
			}

			fix := Fix{Description: "Upgrade Topo"}
			if binPathErr == nil {
				_, fix.Command = upgrade.GetUpgradeCommand(binPath)
			}
			return DependencyCheckResult{Failure: &DependencyCheckFailure{
				Severity: SeverityInfo,
				Message:  fmt.Sprintf("out of date - current: %s, latest version: %s", version.Version, latest),
				Fix:      &fix,
			}}
		},
	}
}

func NewDependencyOnDocker(r runner.Runner) Dependency {
	return Dependency{
		Label: "Container Engine",
		Check: func(ctx context.Context) DependencyCheckResult {
			if err := r.BinaryExists(ctx, "docker"); err != nil {
				return DependencyCheckResult{Failure: &DependencyCheckFailure{
					Severity: SeverityError,
					Message:  err.Error(),
					Fix:      &Fix{Description: "Install a supported container engine. See " + containerEngineInstallURL},
				}}
			}
			if _, _, err := r.Run(ctx, "docker info"); err != nil {
				return DependencyCheckResult{Failure: &DependencyCheckFailure{
					Severity: SeverityError,
					Message:  err.Error(),
					Fix:      &Fix{Description: "Ensure current user can run docker commands. See " + containerEngineInstallURL},
				}}
			}
			return DependencyCheckResult{SuccessValue: "docker"}
		},
	}
}

func NewDependencyOnRemoteDocker(dest ssh.Destination) Dependency {
	return Dependency{
		Label: "Container Engine",
		Check: func(ctx context.Context) DependencyCheckResult {
			r := runner.NewLocal()
			if err := r.BinaryExists(ctx, "docker"); err != nil {
				return DependencyCheckResult{Failure: &DependencyCheckFailure{
					Severity: SeverityError,
					Message:  fmt.Errorf("cannot probe from host: %w", err).Error(),
					Fix:      &Fix{Description: "Install a supported container engine on the host. See " + containerEngineInstallURL},
				}}
			}
			host := docker.NewHostFromDestination(dest)
			if err := docker.RunCommand(ctx, io.Discard, host, "info"); err != nil {
				return DependencyCheckResult{
					Failure: &DependencyCheckFailure{
						Severity: SeverityError,
						Message:  err.Error(),
						Fix:      &Fix{Description: "Ensure docker is installed and running on the target. See " + containerEngineInstallURL},
					},
				}
			}
			return DependencyCheckResult{SuccessValue: "docker"}
		},
	}
}

func NewDependencyOnPodman(r runner.Runner) Dependency {
	return Dependency{
		Label: "Container Engine",
		Check: func(ctx context.Context) DependencyCheckResult {
			if err := r.BinaryExists(ctx, "podman"); err != nil {
				return DependencyCheckResult{Failure: &DependencyCheckFailure{
					Severity: SeverityError,
					Message:  err.Error(),
					Fix:      &Fix{Description: "Install a supported container engine. See " + containerEngineInstallURL},
				}}
			}
			if _, _, err := r.Run(ctx, "podman info"); err != nil {
				return DependencyCheckResult{Failure: &DependencyCheckFailure{
					Severity: SeverityError,
					Message:  err.Error(),
					Fix:      &Fix{Description: "Ensure current user can run podman commands. See " + containerEngineInstallURL},
				}}
			}
			return DependencyCheckResult{SuccessValue: "podman"}
		},
	}
}

func NewDependencyOnPodmanCompose() Dependency {
	return Dependency{
		Label: "Podman Compose",
		Check: func(ctx context.Context) DependencyCheckResult {
			socketURL, err := podman.ResolveLocalComposeSocket(ctx)
			if err != nil {
				return podmanFailure(err)
			}
			socket := podman.NewSocket(socketURL)
			if err := runPodmanCommand(podman.Command(ctx, socket, "info")); err != nil {
				return podmanFailure(err)
			}
			if err := runPodmanComposeProbe(ctx, socket, "version"); err != nil {
				return podmanFailure(err)
			}
			if err := runPodmanComposeProbe(ctx, socket, "ls"); err != nil {
				return podmanFailure(err)
			}
			return DependencyCheckResult{SuccessValue: "docker-compose"}
		},
	}
}

func NewDependencyOnTargetPodman(target ssh.Destination) Dependency {
	return Dependency{
		Label: "Podman API",
		Check: func(ctx context.Context) DependencyCheckResult {
			if target.IsPlainLocalhost() {
				return DependencyCheckResult{SuccessValue: "podman"}
			}

			if err := runner.NewSSH(target).BinaryExists(ctx, "podman"); err != nil {
				return targetPodmanFailure(err, "Install Podman on the target. See "+containerEngineInstallURL)
			}

			remoteSocketPath, err := podman.ResolveRemoteSocketPath(ctx, target)
			if err != nil {
				return targetPodmanFailure(err, "Start the Podman API socket and ensure the SSH user can access it. See "+containerEngineInstallURL)
			}

			var tunnelOutput bytes.Buffer
			tunnel, err := ssh.OpenTCPToUnixSocketTunnel(ctx, &tunnelOutput, target, remoteSocketPath)
			if err != nil {
				return targetPodmanFailure(withPodmanOutput(err, tunnelOutput.String()), "Ensure SSH permits local forwarding to the target Podman API socket.")
			}

			socket := podman.NewSocket(tunnel.SocketURL())
			probeErr := runPodmanCommand(podman.Command(ctx, socket, "info"))
			if probeErr == nil {
				probeErr = runPodmanComposeProbe(ctx, socket, "ls")
			}
			closeErr := tunnel.Close()
			if probeErr != nil {
				return targetPodmanFailure(probeErr, fmt.Sprintf("Ensure the Podman API socket at %s is functional and accessible to the SSH user.", remoteSocketPath))
			}
			if closeErr != nil {
				return targetPodmanFailure(fmt.Errorf("failed to close remote Podman socket tunnel: %w", closeErr), "Close the failed Podman SSH tunnel, then try again.")
			}
			return DependencyCheckResult{SuccessValue: "podman"}
		},
	}
}

func podmanFailure(err error) DependencyCheckResult {
	return DependencyCheckResult{Failure: &DependencyCheckFailure{
		Severity: SeverityError,
		Message:  err.Error(),
		Fix:      &Fix{Description: "Ensure Podman, its API socket, and the docker-compose provider are available. See " + containerEngineInstallURL},
	}}
}

func targetPodmanFailure(err error, fix string) DependencyCheckResult {
	return DependencyCheckResult{Failure: &DependencyCheckFailure{
		Severity: SeverityError,
		Message:  err.Error(),
		Fix:      &Fix{Description: fix},
	}}
}

func runPodmanCommand(cmd *exec.Cmd) error {
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return withPodmanOutput(command.NewError(cmd, err), output.String())
	}
	return nil
}

func withPodmanOutput(err error, output string) error {
	output = strings.TrimSpace(output)
	if output == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, output)
}

func runPodmanComposeProbe(ctx context.Context, socket podman.Socket, args ...string) error {
	cmd, err := podman.ComposeProbeCommand(ctx, socket, args...)
	if err != nil {
		return err
	}
	return runPodmanCommand(cmd)
}

func NewDependencyOnDockerCompose(r runner.Runner) Dependency {
	return Dependency{
		Label: "Docker Compose",
		Check: func(ctx context.Context) DependencyCheckResult {
			stdout, _, err := r.Run(ctx, "docker compose version --format json")
			if err != nil {
				return DependencyCheckResult{Failure: &DependencyCheckFailure{
					Severity: SeverityError,
					Message:  err.Error(),
					Fix:      &Fix{Description: "Ensure Docker Compose is installed as a plugin for Docker. See " + containerEngineInstallURL},
				}}
			}

			var output struct {
				Version string `json:"version"`
			}
			if err := json.Unmarshal([]byte(stdout), &output); err != nil {
				return DependencyCheckResult{Failure: &DependencyCheckFailure{Message: err.Error()}}
			}
			if !version.IsAtLeastVersion(output.Version, "2.21.0") {
				return DependencyCheckResult{Failure: &DependencyCheckFailure{
					Message: fmt.Sprintf("installed docker compose version %s is older than required version %s", output.Version, "2.21.0"),
					Fix: &Fix{
						Description: fmt.Sprintf("Upgrade Docker Compose to version %s or later. See %s", "2.21.0", containerEngineInstallURL),
					},
				}}
			}

			return DependencyCheckResult{SuccessValue: "docker compose"}
		},
	}
}

func NewConnectivityDependency(target *ssh.Destination, acceptNewHostKeys bool, missingTargetMessage string, missingTargetSeverity CheckSeverity, missingTargetFixMessage string) Dependency {
	return Dependency{
		ID:    DependencyIDConnectivity,
		Label: "Connectivity",
		Check: func(ctx context.Context) DependencyCheckResult {
			if target == nil {
				failure := &DependencyCheckFailure{Severity: missingTargetSeverity, Message: missingTargetMessage}
				if missingTargetFixMessage != "" {
					failure.Fix = &Fix{Description: missingTargetFixMessage}
				}
				return DependencyCheckResult{Failure: failure}
			}

			sshRunner := runner.NewSSH(*target)
			err := probe.SSHAuthentication(ctx, sshRunner, acceptNewHostKeys)
			if err == nil {
				return DependencyCheckResult{SuccessValue: target.String()}
			}

			failure := DependencyCheckFailure{Severity: SeverityError, Message: err.Error()}
			switch {
			case errors.Is(err, ssh.ErrAuthFailed), errors.Is(err, ssh.ErrTooManyAuthFails):
				failure.Fix = &Fix{
					Description: "Configure SSH keys on remote target",
					Command:     fmt.Sprintf("topo setup-keys --target %s", *target),
				}
			case errors.Is(err, ssh.ErrHostKeyUnknown):
				failure.Fix = &Fix{
					Description: "Trust the target's SSH host key",
					Command:     fmt.Sprintf("topo health --target %s --accept-new-host-keys", *target),
				}
			case errors.Is(err, ssh.ErrHostKeyChanged):
				sshConfig, configErr := ssh.LoadConfig(*target)
				fixCommand := ""
				if configErr == nil {
					fixCommand = fmt.Sprintf("ssh-keygen -R %s", command.QuoteArg(sshConfig.AsKnownHostsEntry()))
				}
				failure.Fix = &Fix{
					Description: "Remove the old SSH host key from known_hosts, then retry",
					Command:     fixCommand,
				}
			}
			return DependencyCheckResult{Failure: &failure}
		},
	}
}

func NewDependencyOnRemoteproc(r runner.Runner) Dependency {
	return Dependency{
		ID:    DependencyIDRemoteproc,
		Label: "Processing Domain Driver (remoteproc)",
		Check: func(ctx context.Context) DependencyCheckResult {
			remoteProcessors, err := probe.Remoteproc(ctx, r)
			if err != nil {
				return DependencyCheckResult{
					Failure: &DependencyCheckFailure{
						Severity: SeverityError,
						Message:  err.Error(),
					},
				}
			}
			if len(remoteProcessors) > 0 {
				names := make([]string, len(remoteProcessors))
				for i, remoteProc := range remoteProcessors {
					names[i] = remoteProc.Name
				}
				return DependencyCheckResult{
					SuccessValue: strings.Join(names, ", "),
				}
			}
			return DependencyCheckResult{
				Failure: &DependencyCheckFailure{
					Severity: SeverityInfo,
					Message:  "no remoteproc devices found",
				},
			}
		},
	}
}

func NewDependencyOnRemoteprocRuntime(target ssh.Destination, r runner.Runner) Dependency {
	return Dependency{
		Label: "Remoteproc Runtime",
		Check: func(ctx context.Context) DependencyCheckResult {
			if err := r.BinaryExists(ctx, "remoteproc-runtime"); err != nil {
				return DependencyCheckResult{Failure: &DependencyCheckFailure{
					Severity: SeverityWarning,
					Message:  err.Error(),
					Fix: &Fix{
						Description: "Install the Remoteproc Runtime",
						Command:     fmt.Sprintf("topo install remoteproc-runtime --target %s", target),
					},
				}}
			}
			return DependencyCheckResult{SuccessValue: "remoteproc-runtime"}
		},
	}
}

func NewDependencyOnRemoteprocRuntimeShim(target ssh.Destination, r runner.Runner) Dependency {
	return Dependency{
		Label: "Remoteproc Shim",
		Check: func(ctx context.Context) DependencyCheckResult {
			if err := r.BinaryExists(ctx, "containerd-shim-remoteproc-v1"); err != nil {
				return DependencyCheckResult{Failure: &DependencyCheckFailure{
					Severity: SeverityWarning,
					Message:  err.Error(),
					Fix: &Fix{
						Description: "Install the Remoteproc Runtime",
						Command:     fmt.Sprintf("topo install remoteproc-runtime --target %s", target),
					},
				}}
			}
			return DependencyCheckResult{SuccessValue: "containerd-shim-remoteproc-v1"}
		},
	}
}

func NewDependencyOnLscpu(r runner.Runner) Dependency {
	return Dependency{
		Label: "Hardware Info",
		Check: func(ctx context.Context) DependencyCheckResult {
			if err := r.BinaryExists(ctx, "lscpu"); err != nil {
				return DependencyCheckResult{Failure: &DependencyCheckFailure{Severity: SeverityError, Message: err.Error()}}
			}
			return DependencyCheckResult{SuccessValue: "lscpu"}
		},
	}
}
