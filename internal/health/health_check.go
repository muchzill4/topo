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
	Host   HostChecks
	Target TargetChecks
}

type HostChecks struct {
	Topo          Dependency
	SSH           Dependency
	DockerCLI     Dependency
	Docker        Dependency
	DockerCompose Dependency
}

type TargetChecks struct {
	Specified             Dependency
	Connectivity          Dependency
	Docker                Dependency
	Remoteproc            Dependency
	RemoteprocRuntime     Dependency
	RemoteprocRuntimeShim Dependency
	Hardware              Dependency
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
	hostNodes := registerHostChecks(registry, checks.Host)
	targetNodes := registerTargetChecks(registry, checks.Target, hostNodes.dockerCLI)

	return HealthCheck{
		Deployment: ReadinessCheck{
			Registry:     registry,
			Dependencies: append(hostNodes.deployment, targetNodes.deployment...),
		},
		ProjectDiscovery: ReadinessCheck{
			Registry:     registry,
			Dependencies: append(hostNodes.discovery, targetNodes.discovery...),
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
	Registry     *DependencyRegistry
	Dependencies []*DependencyNode
}

func (h ReadinessCheck) Evaluate(ctx context.Context) EvaluatedReadinessCheck {
	evaluated := EvaluatedReadinessCheck{Dependencies: make([]EvaluatedDependency, 0, len(h.Dependencies))}
	for _, reference := range h.Dependencies {
		evaluation := h.Registry.Check(ctx, reference)
		if evaluation.State == EvaluationOmitted {
			continue
		}
		dependency := reference.Dependency()
		evaluated.Dependencies = append(evaluated.Dependencies, EvaluatedDependency{
			Scope:      reference.Scope(),
			ID:         dependency.ID,
			Label:      dependency.Label,
			Evaluation: evaluation,
		})
	}
	return evaluated
}

type EvaluatedHealthCheck struct {
	Deployment       EvaluatedReadinessCheck
	ProjectDiscovery EvaluatedReadinessCheck
}

type EvaluatedReadinessCheck struct {
	Dependencies []EvaluatedDependency
}

type EvaluatedDependency struct {
	Scope      DependencyScope
	ID         DependencyID
	Label      string
	Evaluation DependencyEvaluation
}

func newProductionChecks(options HealthCheckOptions) Checks {
	localRunner := runner.NewLocal()
	return Checks{
		Host: HostChecks{
			Topo:          NewDependencyOnTopo(options.SkipVersionChecks),
			SSH:           NewDependencyOnSSH(localRunner),
			DockerCLI:     NewDependencyOnDockerCLI(localRunner),
			Docker:        NewDependencyOnDocker(localRunner),
			DockerCompose: NewDependencyOnDockerCompose(localRunner),
		},
		Target: TargetChecks{
			Specified: NewTargetSpecifiedDependency(options.Target, options.MissingTargetFixMessage),
			Connectivity: NewConnectivityDependency(options.Target, ConnectivityOperations{
				Authenticate: func(ctx context.Context, target ssh.Destination) error {
					return probe.SSHAuthentication(ctx, runner.NewSSH(target), options.AcceptHostKeys)
				},
				KnownHostsEntry: func(target ssh.Destination) (string, error) {
					config, err := ssh.LoadConfig(target)
					if err != nil {
						return "", err
					}
					return config.AsKnownHostsEntry(), nil
				},
			}),
			Docker: NewDependencyOnRemoteDocker(options.Target, func(ctx context.Context, target ssh.Destination) error {
				return docker.RunCommand(ctx, io.Discard, docker.NewHostFromDestination(target), "info")
			}),
			Remoteproc:            NewDependencyOnRemoteproc(options.Target, runner.For),
			RemoteprocRuntime:     NewDependencyOnRemoteprocRuntime(options.Target, runner.For),
			RemoteprocRuntimeShim: NewDependencyOnRemoteprocRuntimeShim(options.Target, runner.For),
			Hardware:              NewDependencyOnLscpu(options.Target, runner.For),
		},
	}
}

type hostNodes struct {
	dockerCLI  *DependencyNode
	deployment []*DependencyNode
	discovery  []*DependencyNode
}

func registerHostChecks(registry *DependencyRegistry, checks HostChecks) hostNodes {
	topo := registry.Register(checks.Topo, DependencyRequirements{}, DependencyScopeHost)
	ssh := registry.Register(checks.SSH, DependencyRequirements{}, DependencyScopeHost)
	dockerCLI := registry.Register(checks.DockerCLI, DependencyRequirements{}, DependencyScopeHost)
	docker := registry.Register(
		checks.Docker,
		DependencyRequirements{Prerequisites: []*DependencyNode{dockerCLI}},
		DependencyScopeHost,
	)
	compose := registry.Register(
		checks.DockerCompose,
		DependencyRequirements{Prerequisites: []*DependencyNode{dockerCLI}},
		DependencyScopeHost,
	)

	return hostNodes{
		dockerCLI:  dockerCLI,
		deployment: []*DependencyNode{topo, ssh, dockerCLI, docker, compose},
		discovery:  []*DependencyNode{ssh},
	}
}

type targetNodes struct {
	deployment []*DependencyNode
	discovery  []*DependencyNode
}

func registerTargetChecks(registry *DependencyRegistry, checks TargetChecks, dockerCLI *DependencyNode) targetNodes {
	specified := registry.Register(checks.Specified, DependencyRequirements{}, DependencyScopeTarget)
	access := registry.Register(
		checks.Connectivity,
		DependencyRequirements{Prerequisites: []*DependencyNode{specified}},
		DependencyScopeTarget,
	)
	docker := registry.Register(
		checks.Docker,
		DependencyRequirements{Prerequisites: []*DependencyNode{dockerCLI, access}},
		DependencyScopeTarget,
	)
	remoteproc := registry.Register(
		checks.Remoteproc,
		DependencyRequirements{Prerequisites: []*DependencyNode{access}},
		DependencyScopeTarget,
	)
	runtimeRequirements := DependencyRequirements{
		Conditions:    []*DependencyNode{remoteproc},
		Prerequisites: []*DependencyNode{docker, access},
	}
	runtime := registry.Register(checks.RemoteprocRuntime, runtimeRequirements, DependencyScopeTarget)
	shim := registry.Register(checks.RemoteprocRuntimeShim, runtimeRequirements, DependencyScopeTarget)
	hardware := registry.Register(
		checks.Hardware,
		DependencyRequirements{Prerequisites: []*DependencyNode{access}},
		DependencyScopeTarget,
	)

	return targetNodes{
		deployment: []*DependencyNode{specified, access, docker, remoteproc, runtime, shim},
		discovery:  []*DependencyNode{specified, access, hardware},
	}
}
