package health

import (
	"context"

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

type HealthCheck struct {
	Deployment       ReadinessCheck
	ProjectDiscovery ReadinessCheck
}

type ReadinessCheck struct {
	Registry *DependencyRegistry
	Host     []*DependencyNode
	Target   []*DependencyNode
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

func NewHealthCheck(options HealthCheckOptions) HealthCheck {
	registry := NewDependencyRegistry()

	dependencyTopo := registry.Register(NewDependencyOnTopo(options.SkipVersionChecks))
	localRunner := runner.NewLocal()
	dependencySSH := registry.Register(NewDependencyOnSSH(localRunner))
	deploymentHostDependencies := []*DependencyNode{dependencyTopo, dependencySSH}
	projectDiscoveryHostDependencies := []*DependencyNode{dependencySSH}

	engine := options.Engine
	if engine == "" {
		engine = EngineDocker
	}
	var deploymentEngine *DependencyNode
	switch engine {
	case EnginePodman:
		dependencyPodman := registry.Register(NewDependencyOnPodman(localRunner))
		deploymentEngine = registry.Register(NewDependencyOnPodmanCompose(), dependencyPodman)
		deploymentHostDependencies = append(deploymentHostDependencies, dependencyPodman, deploymentEngine)
	default:
		dependencyDocker := registry.Register(NewDependencyOnDocker(localRunner))
		deploymentEngine = registry.Register(NewDependencyOnDockerCompose(localRunner), dependencyDocker)
		deploymentHostDependencies = append(deploymentHostDependencies, dependencyDocker, deploymentEngine)
	}

	targetPrerequisites := []*DependencyNode(nil)
	deploymentTargetDependencies := []*DependencyNode(nil)
	projectDiscoveryTargetDependencies := []*DependencyNode(nil)
	if options.Target == nil {
		deploymentConnectivity := registry.Register(NewConnectivityDependency(
			nil,
			options.AcceptHostKeys,
			"target not specified",
			SeverityError,
			options.MissingTargetFixMessage,
		))
		projectDiscoveryConnectivity := registry.Register(NewConnectivityDependency(
			nil,
			options.AcceptHostKeys,
			"target not specified; cannot calculate project compatibility",
			SeverityWarning,
			options.MissingTargetFixMessage,
		))
		deploymentTargetDependencies = append(deploymentTargetDependencies, deploymentConnectivity)
		projectDiscoveryTargetDependencies = append(projectDiscoveryTargetDependencies, projectDiscoveryConnectivity)
	} else if !options.Target.IsPlainLocalhost() {
		dependencyConnectivity := registry.Register(NewConnectivityDependency(
			options.Target,
			options.AcceptHostKeys,
			"target not specified",
			SeverityError,
			options.MissingTargetFixMessage,
		))
		targetPrerequisites = []*DependencyNode{dependencyConnectivity}
		deploymentTargetDependencies = append(deploymentTargetDependencies, dependencyConnectivity)
		projectDiscoveryTargetDependencies = append(projectDiscoveryTargetDependencies, dependencyConnectivity)
	}
	if options.Target != nil {
		targetRunner := runner.For(*options.Target)
		dependencyLscpu := registry.Register(NewDependencyOnLscpu(targetRunner), targetPrerequisites...)
		projectDiscoveryTargetDependencies = append(projectDiscoveryTargetDependencies, dependencyLscpu)

		if engine == EnginePodman {
			prerequisites := append([]*DependencyNode{deploymentEngine}, targetPrerequisites...)
			dependencyPodmanTarget := registry.Register(NewDependencyOnTargetPodman(*options.Target), prerequisites...)
			deploymentTargetDependencies = append(deploymentTargetDependencies, dependencyPodmanTarget)
		} else {
			dependencyDocker := registry.Register(NewDependencyOnRemoteDocker(*options.Target), targetPrerequisites...)
			dependencyRemoteproc := registry.Register(NewDependencyOnRemoteproc(targetRunner), targetPrerequisites...)
			dependencyRemoteprocRuntime := registry.Register(
				NewDependencyOnRemoteprocRuntime(*options.Target, targetRunner),
				append([]*DependencyNode{dependencyDocker, dependencyRemoteproc}, targetPrerequisites...)...,
			)
			dependencyRemoteprocRuntimeShim := registry.Register(
				NewDependencyOnRemoteprocRuntimeShim(*options.Target, targetRunner),
				append([]*DependencyNode{dependencyDocker, dependencyRemoteproc}, targetPrerequisites...)...,
			)
			deploymentTargetDependencies = append(deploymentTargetDependencies, dependencyDocker, dependencyRemoteproc, dependencyRemoteprocRuntime, dependencyRemoteprocRuntimeShim)
		}
	}

	return HealthCheck{
		Deployment: ReadinessCheck{
			Registry: registry,
			Host:     deploymentHostDependencies,
			Target:   deploymentTargetDependencies,
		},
		ProjectDiscovery: ReadinessCheck{
			Registry: registry,
			Host:     projectDiscoveryHostDependencies,
			Target:   projectDiscoveryTargetDependencies,
		},
	}
}

func (h HealthCheck) Evaluate(ctx context.Context) EvaluatedHealthCheck {
	return EvaluatedHealthCheck{
		Deployment:       h.Deployment.Evaluate(ctx),
		ProjectDiscovery: h.ProjectDiscovery.Evaluate(ctx),
	}
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
		statuses = append(statuses, EvaluatedDependency{
			ID:     dependency.ID,
			Label:  dependency.Label,
			Result: result,
		})
	}
	return statuses
}
