package health

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/arm/topo/internal/command"
	"github.com/arm/topo/internal/deploy/docker"
	"github.com/arm/topo/internal/deploy/podman"
	"github.com/arm/topo/internal/probe"
	"github.com/arm/topo/internal/runner"
	"github.com/arm/topo/internal/ssh"
)

type Engine string

const (
	EngineDocker Engine = "docker"
	EnginePodman Engine = "podman"
)

type HealthCheckOptions struct {
	Engine                  Engine
	Target                  *ssh.Destination
	MissingTargetFixMessage string
	SkipVersionChecks       bool
	AcceptHostKeys          bool
}

type Checks struct {
	Host   HostChecks
	Target TargetChecks
}

type HostChecks struct {
	Topo          Dependency
	SSH           Dependency
	Docker        Dependency
	DockerCompose Dependency
	Podman        Dependency
	PodmanCompose Dependency
}

type TargetChecks struct {
	Specified             Dependency
	Connectivity          Dependency
	Docker                Dependency
	Remoteproc            Dependency
	RemoteprocRuntime     Dependency
	RemoteprocRuntimeShim Dependency
	Podman                Dependency
	Hardware              Dependency
}

type HealthCheck struct {
	Deployment       ReadinessCheck
	ProjectDiscovery ReadinessCheck
}

func NewHealthCheck(options HealthCheckOptions) HealthCheck {
	return AssembleHealthCheck(normalizeEngine(options.Engine), newProductionChecks(options))
}

func AssembleHealthCheck(engine Engine, checks Checks) HealthCheck {
	registry := NewDependencyRegistry()
	hostNodes := registerHostChecks(registry, engine, checks.Host)
	targetNodes := registerTargetChecks(registry, engine, checks.Target)

	return HealthCheck{
		Deployment: ReadinessCheck{
			Registry: registry,
			Host:     hostNodes.deployment,
			Target:   targetNodes.deployment,
		},
		ProjectDiscovery: ReadinessCheck{
			Registry: registry,
			Host:     hostNodes.discovery,
			Target:   targetNodes.discovery,
		},
	}
}

func (h HealthCheck) Evaluate(ctx context.Context) EvaluatedHealthCheck {
	return EvaluatedHealthCheck{
		Deployment:       h.Deployment.Evaluate(ctx),
		ProjectDiscovery: h.ProjectDiscovery.Evaluate(ctx),
	}
}

type ReadinessCheck struct {
	Registry *DependencyRegistry
	Host     []*DependencyNode
	Target   []*DependencyNode
}

func (h ReadinessCheck) Evaluate(ctx context.Context) EvaluatedReadinessCheck {
	return EvaluatedReadinessCheck{
		Host:   h.evaluateDependencies(ctx, h.Host),
		Target: h.evaluateDependencies(ctx, h.Target),
	}
}

func (h ReadinessCheck) evaluateDependencies(ctx context.Context, references []*DependencyNode) []EvaluatedDependency {
	statuses := make([]EvaluatedDependency, 0, len(references))
	for _, reference := range references {
		result, checked := h.Registry.Check(ctx, reference)
		if !checked {
			continue
		}
		dependency := reference.Dependency()
		statuses = append(statuses, EvaluatedDependency{ID: dependency.ID, Label: dependency.Label, Result: result})
	}
	return statuses
}

type EvaluatedHealthCheck struct {
	Deployment       EvaluatedReadinessCheck
	ProjectDiscovery EvaluatedReadinessCheck
}

type EvaluatedReadinessCheck struct {
	Host   []EvaluatedDependency
	Target []EvaluatedDependency
}

type EvaluatedDependency struct {
	ID     DependencyID
	Label  string
	Result DependencyCheckResult
}

func newProductionChecks(options HealthCheckOptions) Checks {
	localRunner := runner.NewLocal()
	return Checks{
		Host: HostChecks{
			Topo:          NewDependencyOnTopo(options.SkipVersionChecks),
			SSH:           NewDependencyOnSSH(localRunner),
			Docker:        NewDependencyOnDocker(localRunner),
			DockerCompose: NewDependencyOnDockerCompose(localRunner),
			Podman:        NewDependencyOnPodman(localRunner),
			PodmanCompose: NewDependencyOnPodmanCompose(PodmanComposeOperations{Commands: newPodmanCommands()}),
		},
		Target: TargetChecks{
			Specified: NewTargetSpecifiedDependency(options.Target, options.MissingTargetFixMessage),
			Connectivity: NewConnectivityDependency(options.Target, func(target ssh.Destination) ConnectivityOperations {
				return connectivityOperations(target, options.AcceptHostKeys)
			}),
			Docker: NewDependencyOnRemoteDocker(options.Target, localRunner, func(ctx context.Context, target ssh.Destination) error {
				return docker.RunCommand(ctx, io.Discard, docker.NewHostFromDestination(target), "info")
			}),
			Remoteproc:            NewDependencyOnRemoteproc(options.Target, runner.For),
			RemoteprocRuntime:     NewDependencyOnRemoteprocRuntime(options.Target, runner.For),
			RemoteprocRuntimeShim: NewDependencyOnRemoteprocRuntimeShim(options.Target, runner.For),
			Podman: NewDependencyOnRemotePodman(options.Target, TargetPodmanOperations{
				HostRunner: localRunner,
				Runner:     runner.For,
				ResolveRemoteSocket: func(ctx context.Context, target ssh.Destination) (string, error) {
					return podman.ResolveRemoteSocketPath(ctx, target)
				},
				OpenTunnel: func(ctx context.Context, target ssh.Destination, socketPath string) (SocketTunnel, error) {
					var output bytes.Buffer
					tunnel, err := ssh.OpenTCPToUnixSocketTunnel(ctx, &output, target, socketPath)
					if err != nil {
						return nil, withPodmanDiagnostics(err, output.String())
					}
					return tunnel, nil
				},
				Commands: newPodmanCommands(),
			}),
			Hardware: NewDependencyOnLscpu(options.Target, runner.For),
		},
	}
}

func normalizeEngine(engine Engine) Engine {
	if engine == EnginePodman {
		return EnginePodman
	}
	return EngineDocker
}

func newPodmanCommands() PodmanCommands {
	return PodmanCommands{
		ComposeVersion: func(ctx context.Context) error {
			return runPodmanProbe(podman.ComposeProviderProbeCommand(ctx, "version"))
		},
		Info: func(ctx context.Context, socketURL string) error {
			return runPodmanProbe(podman.Command(ctx, podman.NewSocket(socketURL), "info"))
		},
		Compose: func(ctx context.Context, socketURL string, args ...string) error {
			cmd, err := podman.ComposeProbeCommand(ctx, podman.NewSocket(socketURL), args...)
			if err != nil {
				return err
			}
			return runPodmanProbe(cmd)
		},
	}
}

func runPodmanProbe(cmd *exec.Cmd) error {
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return withPodmanDiagnostics(command.NewError(cmd, err), output.String())
	}
	return nil
}

func connectivityOperations(target ssh.Destination, acceptHostKeys bool) ConnectivityOperations {
	sshRunner := runner.NewSSH(target)
	return ConnectivityOperations{
		Authenticate: func(ctx context.Context) error {
			return probe.SSHAuthentication(ctx, sshRunner, acceptHostKeys)
		},
		KnownHostsEntry: func() (string, error) {
			config, err := ssh.LoadConfig(target)
			if err != nil {
				return "", err
			}
			return config.AsKnownHostsEntry(), nil
		},
	}
}

type hostNodes struct {
	deployment []*DependencyNode
	discovery  []*DependencyNode
}

func registerHostChecks(registry *DependencyRegistry, engine Engine, checks HostChecks) hostNodes {
	topo := registry.Register(checks.Topo)
	ssh := registry.Register(checks.SSH)

	if normalizeEngine(engine) == EnginePodman {
		podman := registry.Register(checks.Podman)
		compose := registry.Register(checks.PodmanCompose, podman)
		return hostNodes{deployment: []*DependencyNode{topo, ssh, podman, compose}, discovery: []*DependencyNode{ssh}}
	}

	docker := registry.Register(checks.Docker)
	compose := registry.Register(checks.DockerCompose, docker)
	return hostNodes{deployment: []*DependencyNode{topo, ssh, docker, compose}, discovery: []*DependencyNode{ssh}}
}

type targetNodes struct {
	deployment []*DependencyNode
	discovery  []*DependencyNode
}

func registerTargetChecks(registry *DependencyRegistry, engine Engine, checks TargetChecks) targetNodes {
	specified := registry.Register(checks.Specified)
	access := registry.Register(checks.Connectivity, specified)

	deployment := []*DependencyNode{specified, access}
	switch normalizeEngine(engine) {
	case EnginePodman:
		deployment = append(deployment, registry.Register(checks.Podman, access))
	case EngineDocker:
		docker := registry.Register(checks.Docker, access)
		remoteproc := registry.Register(checks.Remoteproc, access)
		runtime := registry.Register(checks.RemoteprocRuntime, docker, remoteproc, access)
		shim := registry.Register(checks.RemoteprocRuntimeShim, docker, remoteproc, access)
		deployment = append(deployment, docker, remoteproc, runtime, shim)
	}

	lscpu := registry.Register(checks.Hardware, access)

	return targetNodes{
		deployment: deployment,
		discovery:  []*DependencyNode{specified, access, lscpu},
	}
}

func withPodmanDiagnostics(err error, output string) error {
	output = strings.TrimSpace(output)
	if output == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, output)
}
