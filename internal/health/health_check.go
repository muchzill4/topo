package health

import (
	"context"
	"io"

	"github.com/arm/topo/internal/deploy/docker"
	"github.com/arm/topo/internal/probe"
	"github.com/arm/topo/internal/runner"
	"github.com/arm/topo/internal/ssh"
)

type HealthCheckOptions struct {
	Target                  *ssh.Destination
	MissingTargetFixMessage string
	SkipVersionChecks       bool
	AcceptHostKeys          bool
}

type Checks struct {
	Topo                  Dependency
	SSH                   Dependency
	Docker                Dependency
	DockerCompose         Dependency
	TargetDocker          Dependency
	Connectivity          Dependency
	TargetSpecified       Dependency
	Lscpu                 Dependency
	Remoteproc            Dependency
	RemoteprocRuntime     Dependency
	RemoteprocRuntimeShim Dependency
}

type HealthCheck struct {
	Deployment       ReadinessCheck
	ProjectDiscovery ReadinessCheck
}

func NewHealthCheck(options HealthCheckOptions) HealthCheck {
	return AssembleHealthCheck(newProductionChecks(options))
}

func AssembleHealthCheck(checks Checks) HealthCheck {
	registry := NewDependencyRegistry()
	hostNodes := registerHostChecks(registry, checks)
	targetNodes := registerTargetChecks(registry, checks)

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
		Topo:            NewDependencyOnTopo(options.SkipVersionChecks),
		SSH:             NewDependencyOnSSH(localRunner),
		Docker:          NewDependencyOnDocker(localRunner),
		DockerCompose:   NewDependencyOnDockerCompose(localRunner),
		TargetSpecified: NewTargetSpecifiedDependency(options.Target, options.MissingTargetFixMessage),
		Connectivity: NewConnectivityDependency(options.Target, func(target ssh.Destination) ConnectivityOperations {
			return connectivityOperations(target, options.AcceptHostKeys)
		}),
		TargetDocker: NewDependencyOnRemoteDocker(options.Target, localRunner, func(ctx context.Context, target ssh.Destination) error {
			return docker.RunCommand(ctx, io.Discard, docker.NewHostFromDestination(target), "info")
		}),
		Lscpu:                 NewDependencyOnLscpu(options.Target, runner.For),
		Remoteproc:            NewDependencyOnRemoteproc(options.Target, runner.For),
		RemoteprocRuntime:     NewDependencyOnRemoteprocRuntime(options.Target, runner.For),
		RemoteprocRuntimeShim: NewDependencyOnRemoteprocRuntimeShim(options.Target, runner.For),
	}
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

func registerHostChecks(registry *DependencyRegistry, checks Checks) hostNodes {
	topo := registry.Register(checks.Topo)
	ssh := registry.Register(checks.SSH)
	docker := registry.Register(checks.Docker)
	compose := registry.Register(checks.DockerCompose, docker)

	return hostNodes{
		deployment: []*DependencyNode{topo, ssh, docker, compose},
		discovery:  []*DependencyNode{ssh},
	}
}

type targetNodes struct {
	deployment []*DependencyNode
	discovery  []*DependencyNode
}

func registerTargetChecks(registry *DependencyRegistry, checks Checks) targetNodes {
	specified := registry.Register(checks.TargetSpecified)
	access := registry.Register(checks.Connectivity, specified)
	hardware := registry.Register(checks.Lscpu, access)

	return targetNodes{
		deployment: append([]*DependencyNode{specified, access}, registerTargetContainerEngineChecks(registry, checks, access)...),
		discovery:  []*DependencyNode{specified, access, hardware},
	}
}

func registerTargetContainerEngineChecks(registry *DependencyRegistry, checks Checks, prerequisites ...*DependencyNode) []*DependencyNode {
	docker := registry.Register(checks.TargetDocker, prerequisites...)
	remoteproc := registry.Register(checks.Remoteproc, prerequisites...)
	runtimePrerequisites := append([]*DependencyNode{docker, remoteproc}, prerequisites...)
	runtime := registry.Register(checks.RemoteprocRuntime, runtimePrerequisites...)
	shim := registry.Register(checks.RemoteprocRuntimeShim, runtimePrerequisites...)

	return []*DependencyNode{docker, remoteproc, runtime, shim}
}
