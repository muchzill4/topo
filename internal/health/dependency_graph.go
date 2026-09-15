package health

import (
	"context"
	"fmt"
	"sync"

	"github.com/arm/topo/internal/runner"
	"github.com/arm/topo/internal/ssh"
)

type dependencyNode struct {
	dependency Dependency
	once       sync.Once
	result     DependencyCheckResult
}

type DependencyRegistry struct {
	dependencies map[DependencyID]*dependencyNode
}

func NewDependencyRegistry(dependencies []Dependency) *DependencyRegistry {
	registered := make(map[DependencyID]*dependencyNode, len(dependencies))
	for _, dependency := range dependencies {
		if _, exists := registered[dependency.ID]; exists {
			panic(fmt.Sprintf("duplicate health dependency ID: %q", dependency.ID))
		}
		registered[dependency.ID] = &dependencyNode{dependency: dependency}
	}
	return &DependencyRegistry{dependencies: registered}
}

func (r *DependencyRegistry) Check(ctx context.Context, id DependencyID) DependencyCheckResult {
	dependency := r.dependency(id)

	dependency.once.Do(func() {
		if dependency.dependency.Check != nil {
			dependency.result = dependency.dependency.Check(ctx)
		}
	})
	return dependency.result
}

func (r *DependencyRegistry) dependency(id DependencyID) *dependencyNode {
	dependency, exists := r.dependencies[id]
	if !exists {
		panic(fmt.Sprintf("health dependency not registered: %q", id))
	}
	return dependency
}

type DependencyGraphOptions struct {
	Target            *ssh.Destination
	SkipVersionChecks bool
	AcceptHostKeys    bool
}

type FunctionalityGroup struct {
	Name   string
	Host   []DependencyID
	Target []DependencyID
}

type DependencyGraph struct {
	Registry        *DependencyRegistry
	Host            []DependencyID
	Target          []DependencyID
	Functionalities []FunctionalityGroup
}

type DependencyStatus struct {
	ID     DependencyID
	Label  string
	Result DependencyCheckResult
}

type EvaluatedFunctionalityGroup struct {
	Name   string
	Host   []DependencyStatus
	Target []DependencyStatus
}

type EvaluatedDependencyGraph struct {
	Host            []DependencyStatus
	Target          []DependencyStatus
	Functionalities []EvaluatedFunctionalityGroup
}

func NewDependencyGraph(options DependencyGraphOptions) DependencyGraph {
	localRunner := runner.NewLocal()
	topo := NewDependencyOnTopo(options.SkipVersionChecks)
	hostSSH := NewDependencyOnSSH(localRunner)
	hostDocker := NewDependencyOnDocker("host-docker", localRunner)
	dockerCompose := NewDependencyOnDockerCompose(localRunner, hostDocker.ID)
	hostDependencies := []Dependency{topo, hostSSH, hostDocker, dockerCompose}

	targetDependencies := []Dependency(nil)
	if options.Target != nil {
		targetRunner := runner.For(*options.Target)
		remoteTargetPrerequisites := []DependencyID(nil)
		if !options.Target.IsPlainLocalhost() {
			connectivity := NewConnectivityDependency(*options.Target, options.AcceptHostKeys)
			targetDependencies = append(targetDependencies, connectivity)
			remoteTargetPrerequisites = []DependencyID{connectivity.ID}
		}
		targetDocker := NewDependencyOnDocker("target-docker", targetRunner, remoteTargetPrerequisites...)
		remoteproc := NewDependencyOnRemoteproc(targetRunner, remoteTargetPrerequisites...)
		runtimePrerequisites := append([]DependencyID{targetDocker.ID, remoteproc.ID}, remoteTargetPrerequisites...)
		remoteprocRuntime := NewDependencyOnRemoteprocRuntime(*options.Target, targetRunner, runtimePrerequisites...)
		remoteprocRuntimeShim := NewDependencyOnRemoteprocRuntimeShim(*options.Target, targetRunner, runtimePrerequisites...)
		lscpu := NewDependencyOnLscpu(targetRunner, remoteTargetPrerequisites...)
		targetDependencies = append(targetDependencies, targetDocker, remoteproc, remoteprocRuntime, remoteprocRuntimeShim, lscpu)
	}

	toRegister := make([]Dependency, 0, len(hostDependencies)+len(targetDependencies))
	toRegister = append(toRegister, hostDependencies...)
	toRegister = append(toRegister, targetDependencies...)

	return DependencyGraph{
		Registry: NewDependencyRegistry(toRegister),
		// Compat with existing view data assembly
		Host:   dependencyIDs(hostDependencies),
		Target: dependencyIDs(targetDependencies),
		Functionalities: []FunctionalityGroup{
			{
				Name:   "Deployment",
				Host:   dependencyIDs(hostDependencies),
				Target: dependencyIDs(targetDependencies),
			},
		},
	}
}

func (g DependencyGraph) Evaluate(ctx context.Context) EvaluatedDependencyGraph {
	functionalities := make([]EvaluatedFunctionalityGroup, len(g.Functionalities))
	for index, functionality := range g.Functionalities {
		functionalities[index] = EvaluatedFunctionalityGroup{
			Name:   functionality.Name,
			Host:   g.evaluateDependencies(ctx, functionality.Host),
			Target: g.evaluateDependencies(ctx, functionality.Target),
		}
	}
	return EvaluatedDependencyGraph{
		Host:            g.evaluateDependencies(ctx, g.Host),
		Target:          g.evaluateDependencies(ctx, g.Target),
		Functionalities: functionalities,
	}
}

func (g DependencyGraph) evaluateDependencies(ctx context.Context, dependencies []DependencyID) []DependencyStatus {
	statuses := make([]DependencyStatus, 0, len(dependencies))
	for _, id := range dependencies {
		dependency := g.Registry.dependency(id).dependency
		if !prerequisitesFulfilled(ctx, g.Registry, dependency.Prerequisites) {
			continue
		}
		statuses = append(statuses, DependencyStatus{
			ID:     dependency.ID,
			Label:  dependency.Label,
			Result: g.Registry.Check(ctx, id),
		})
	}
	return statuses
}

func prerequisitesFulfilled(ctx context.Context, registry *DependencyRegistry, prerequisites []DependencyID) bool {
	for _, prerequisite := range prerequisites {
		dependency := registry.dependency(prerequisite).dependency
		if !prerequisitesFulfilled(ctx, registry, dependency.Prerequisites) {
			return false
		}
		if registry.Check(ctx, prerequisite).Failure != nil {
			return false
		}
	}
	return true
}

func dependencyIDs(dependencies []Dependency) []DependencyID {
	ids := make([]DependencyID, len(dependencies))
	for index, dependency := range dependencies {
		ids[index] = dependency.ID
	}
	return ids
}
