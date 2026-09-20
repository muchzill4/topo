package health_test

import (
	"context"
	"testing"

	"github.com/arm/topo/internal/health"
	"github.com/stretchr/testify/assert"
)

func TestHealthCheck(t *testing.T) {
	t.Run("Evaluate", func(t *testing.T) {
		t.Run("evaluates deployment and project discovery checks", func(t *testing.T) {
			registry := health.NewDependencyRegistry()
			deployment := health.Dependency{ID: "deployment", Check: passingCheck}
			deploymentRef := registry.Register(deployment, health.DependencyRequirements{}, health.DependencyScopeHost)
			projectDiscovery := health.Dependency{ID: "project-discovery", Check: failingCheck}
			projectDiscoveryRef := registry.Register(
				projectDiscovery,
				health.DependencyRequirements{},
				health.DependencyScopeTarget,
			)
			healthCheck := health.HealthCheck{
				Deployment: health.ReadinessCheck{
					Registry:     registry,
					Dependencies: []*health.DependencyNode{deploymentRef},
				},
				ProjectDiscovery: health.ReadinessCheck{
					Registry:     registry,
					Dependencies: []*health.DependencyNode{projectDiscoveryRef},
				},
			}

			got := healthCheck.Evaluate(context.Background())

			want := health.EvaluatedHealthCheck{
				Deployment: health.EvaluatedReadinessCheck{Dependencies: []health.EvaluatedDependency{{
					Scope:  health.DependencyScopeHost,
					ID:     deployment.ID,
					Label:  deployment.Label,
					Result: deployment.Check(context.Background()),
				}}},
				ProjectDiscovery: health.EvaluatedReadinessCheck{Dependencies: []health.EvaluatedDependency{{
					Scope:  health.DependencyScopeTarget,
					ID:     projectDiscovery.ID,
					Label:  projectDiscovery.Label,
					Result: projectDiscovery.Check(context.Background()),
				}}},
			}
			assert.Equal(t, want, got)
		})
	})
}

func TestAssembleHealthCheck(t *testing.T) {
	newPassingChecks := func() health.Checks {
		passing := func(label string) health.Dependency {
			return health.Dependency{Label: label, Check: passingCheck}
		}
		return health.Checks{
			Host: health.HostChecks{
				Topo: passing("Topo"), SSH: passing("OpenSSH"), DockerCLI: passing("Docker CLI"),
				Docker: passing("Container Engine"), DockerCompose: passing("Docker Compose"),
			},
			Target: health.TargetChecks{
				Specified: passing("Target specified"), Connectivity: passing("Target access"), Docker: passing("Target Docker"),
				Hardware: passing("Hardware Info"), Remoteproc: passing("Remoteproc"), RemoteprocRuntime: passing("Remoteproc Runtime"),
				RemoteprocRuntimeShim: passing("Remoteproc Shim"),
			},
		}
	}

	t.Run("shares a failed target prerequisite with dependent target checks", func(t *testing.T) {
		checks := newPassingChecks()
		checks.Target.Specified.Check = failingCheck
		healthCheck := health.AssembleHealthCheck(checks)

		got := healthCheck.Evaluate(context.Background())

		want := []health.EvaluatedDependency{{
			Scope:  health.DependencyScopeTarget,
			Label:  "Target specified",
			Result: checks.Target.Specified.Check(context.Background()),
		}}
		assert.Equal(t, want, targetDependencies(got.Deployment.Dependencies))
		assert.Equal(t, want, targetDependencies(got.ProjectDiscovery.Dependencies))
	})

	t.Run("does not probe the target engine when Docker CLI is unavailable", func(t *testing.T) {
		checks := newPassingChecks()
		checks.Host.DockerCLI.Check = failingCheck
		targetDockerChecks := 0
		checks.Target.Docker.Check = func(context.Context) health.DependencyCheckResult {
			targetDockerChecks++
			return passingCheck(context.Background())
		}
		healthCheck := health.AssembleHealthCheck(checks)

		healthCheck.Evaluate(context.Background())

		assert.Zero(t, targetDockerChecks)
	})

	t.Run("runs target engine and Compose checks when the host daemon fails", func(t *testing.T) {
		checks := newPassingChecks()
		checks.Host.Docker.Check = failingCheck
		composeChecks := 0
		checks.Host.DockerCompose.Check = func(context.Context) health.DependencyCheckResult {
			composeChecks++
			return passingCheck(context.Background())
		}
		targetDockerChecks := 0
		checks.Target.Docker.Check = func(context.Context) health.DependencyCheckResult {
			targetDockerChecks++
			return passingCheck(context.Background())
		}
		healthCheck := health.AssembleHealthCheck(checks)

		healthCheck.Evaluate(context.Background())

		assert.Equal(t, 1, composeChecks)
		assert.Equal(t, 1, targetDockerChecks)
	})

	t.Run("runs target checks after their prerequisites succeed", func(t *testing.T) {
		checks := newPassingChecks()
		healthCheck := health.AssembleHealthCheck(checks)

		got := healthCheck.Evaluate(context.Background())

		wantDeploymentResults := []health.EvaluatedDependency{
			{Scope: health.DependencyScopeTarget, Label: "Target specified", Result: checks.Target.Specified.Check(context.Background())},
			{Scope: health.DependencyScopeTarget, Label: "Target access", Result: checks.Target.Connectivity.Check(context.Background())},
			{Scope: health.DependencyScopeTarget, Label: "Target Docker", Result: checks.Target.Docker.Check(context.Background())},
			{Scope: health.DependencyScopeTarget, Label: "Remoteproc", Result: checks.Target.Remoteproc.Check(context.Background())},
			{Scope: health.DependencyScopeTarget, Label: "Remoteproc Runtime", Result: checks.Target.RemoteprocRuntime.Check(context.Background())},
			{Scope: health.DependencyScopeTarget, Label: "Remoteproc Shim", Result: checks.Target.RemoteprocRuntimeShim.Check(context.Background())},
		}
		wantDiscoveryResults := []health.EvaluatedDependency{
			{Scope: health.DependencyScopeTarget, Label: "Target specified", Result: checks.Target.Specified.Check(context.Background())},
			{Scope: health.DependencyScopeTarget, Label: "Target access", Result: checks.Target.Connectivity.Check(context.Background())},
			{Scope: health.DependencyScopeTarget, Label: "Hardware Info", Result: checks.Target.Hardware.Check(context.Background())},
		}
		assert.Equal(t, wantDeploymentResults, targetDependencies(got.Deployment.Dependencies))
		assert.Equal(t, wantDiscoveryResults, targetDependencies(got.ProjectDiscovery.Dependencies))
	})

	t.Run("shares connectivity and suppresses its dependent target checks", func(t *testing.T) {
		checks := newPassingChecks()
		checks.Target.Connectivity.Check = failingCheck
		healthCheck := health.AssembleHealthCheck(checks)

		got := healthCheck.Evaluate(context.Background())

		wantResults := []health.EvaluatedDependency{
			{Scope: health.DependencyScopeTarget, Label: "Target specified", Result: checks.Target.Specified.Check(context.Background())},
			{Scope: health.DependencyScopeTarget, Label: "Target access", Result: checks.Target.Connectivity.Check(context.Background())},
		}
		assert.Equal(t, wantResults, targetDependencies(got.Deployment.Dependencies))
		assert.Equal(t, wantResults, targetDependencies(got.ProjectDiscovery.Dependencies))
	})

	t.Run("suppresses runtime checks when remoteproc fails", func(t *testing.T) {
		checks := newPassingChecks()
		checks.Target.Remoteproc.Check = failingCheck
		healthCheck := health.AssembleHealthCheck(checks)

		got := healthCheck.Evaluate(context.Background())

		wantDeploymentResults := []health.EvaluatedDependency{
			{Scope: health.DependencyScopeTarget, Label: "Target specified", Result: checks.Target.Specified.Check(context.Background())},
			{Scope: health.DependencyScopeTarget, Label: "Target access", Result: checks.Target.Connectivity.Check(context.Background())},
			{Scope: health.DependencyScopeTarget, Label: "Target Docker", Result: checks.Target.Docker.Check(context.Background())},
			{Scope: health.DependencyScopeTarget, Label: "Remoteproc", Result: checks.Target.Remoteproc.Check(context.Background())},
		}
		assert.Equal(t, wantDeploymentResults, targetDependencies(got.Deployment.Dependencies))
	})

	t.Run("keeps hardware discovery independent from container engine readiness", func(t *testing.T) {
		checks := newPassingChecks()
		checks.Target.Docker.Check = failingCheck
		healthCheck := health.AssembleHealthCheck(checks)

		got := healthCheck.Evaluate(context.Background())

		wantDeploymentResults := []health.EvaluatedDependency{
			{Scope: health.DependencyScopeTarget, Label: "Target specified", Result: checks.Target.Specified.Check(context.Background())},
			{Scope: health.DependencyScopeTarget, Label: "Target access", Result: checks.Target.Connectivity.Check(context.Background())},
			{Scope: health.DependencyScopeTarget, Label: "Target Docker", Result: checks.Target.Docker.Check(context.Background())},
			{Scope: health.DependencyScopeTarget, Label: "Remoteproc", Result: checks.Target.Remoteproc.Check(context.Background())},
		}
		wantDiscoveryResults := []health.EvaluatedDependency{
			{Scope: health.DependencyScopeTarget, Label: "Target specified", Result: checks.Target.Specified.Check(context.Background())},
			{Scope: health.DependencyScopeTarget, Label: "Target access", Result: checks.Target.Connectivity.Check(context.Background())},
			{Scope: health.DependencyScopeTarget, Label: "Hardware Info", Result: checks.Target.Hardware.Check(context.Background())},
		}
		assert.Equal(t, wantDeploymentResults, targetDependencies(got.Deployment.Dependencies))
		assert.Equal(t, wantDiscoveryResults, targetDependencies(got.ProjectDiscovery.Dependencies))
	})
}

func targetDependencies(dependencies []health.EvaluatedDependency) []health.EvaluatedDependency {
	targets := make([]health.EvaluatedDependency, 0, len(dependencies))
	for _, dependency := range dependencies {
		if dependency.Scope == health.DependencyScopeTarget {
			targets = append(targets, dependency)
		}
	}
	return targets
}

func TestReadinessCheck(t *testing.T) {
	t.Run("Evaluate", func(t *testing.T) {
		t.Run("reports successful dependencies in the group", func(t *testing.T) {
			registry := health.NewDependencyRegistry()
			virus := health.Dependency{ID: "virus", Check: passingCheck}
			virusRef := registry.Register(virus, health.DependencyRequirements{}, health.DependencyScopeHost)
			bartek := health.Dependency{ID: "bartek", Check: passingCheck}
			bartekRef := registry.Register(
				bartek,
				health.DependencyRequirements{Prerequisites: []*health.DependencyNode{virusRef}},
				health.DependencyScopeHost,
			)

			healthCheck := health.ReadinessCheck{
				Registry:     registry,
				Dependencies: []*health.DependencyNode{bartekRef, virusRef},
			}

			got := healthCheck.Evaluate(context.Background())

			want := []health.EvaluatedDependency{
				{Scope: health.DependencyScopeHost, ID: bartek.ID, Label: bartek.Label, Result: bartek.Check(context.Background())},
				{Scope: health.DependencyScopeHost, ID: virus.ID, Label: virus.Label, Result: virus.Check(context.Background())},
			}
			assert.Equal(t, want, got.Dependencies)
		})

		t.Run("reports a failed prerequisite and omits its dependent", func(t *testing.T) {
			registry := health.NewDependencyRegistry()
			flour := health.Dependency{ID: "flour", Check: failingCheck}
			flourRef := registry.Register(flour, health.DependencyRequirements{}, health.DependencyScopeHost)
			pizza := health.Dependency{ID: "pizza", Check: passingCheck}
			pizzaRef := registry.Register(
				pizza,
				health.DependencyRequirements{Prerequisites: []*health.DependencyNode{flourRef}},
				health.DependencyScopeHost,
			)

			healthCheck := health.ReadinessCheck{
				Registry:     registry,
				Dependencies: []*health.DependencyNode{flourRef, pizzaRef},
			}

			got := healthCheck.Evaluate(context.Background())

			want := []health.EvaluatedDependency{
				{Scope: health.DependencyScopeHost, ID: flour.ID, Label: flour.Label, Result: flour.Check(context.Background())},
			}
			assert.Equal(t, want, got.Dependencies)
		})
	})
}
