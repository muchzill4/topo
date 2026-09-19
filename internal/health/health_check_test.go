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
				Deployment: health.EvaluatedReadinessCheck{
					Host: []health.EvaluatedDependency{{
						ID:     deployment.ID,
						Label:  deployment.Label,
						Result: deployment.Check(context.Background()),
					}},
					Target: []health.EvaluatedDependency{},
				},
				ProjectDiscovery: health.EvaluatedReadinessCheck{
					Host: []health.EvaluatedDependency{},
					Target: []health.EvaluatedDependency{{
						ID:     projectDiscovery.ID,
						Label:  projectDiscovery.Label,
						Result: projectDiscovery.Check(context.Background()),
					}},
				},
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
				Topo: passing("Topo"), SSH: passing("OpenSSH"), Docker: passing("Container Engine"), DockerCompose: passing("Docker Compose"),
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
			Label:  "Target specified",
			Result: checks.Target.Specified.Check(context.Background()),
		}}
		assert.Equal(t, want, got.Deployment.Target)
		assert.Equal(t, want, got.ProjectDiscovery.Target)
	})

	t.Run("runs target checks after their prerequisites succeed", func(t *testing.T) {
		checks := newPassingChecks()
		healthCheck := health.AssembleHealthCheck(checks)

		got := healthCheck.Evaluate(context.Background())

		wantDeploymentResults := []health.EvaluatedDependency{
			{Label: "Target specified", Result: checks.Target.Specified.Check(context.Background())},
			{Label: "Target access", Result: checks.Target.Connectivity.Check(context.Background())},
			{Label: "Target Docker", Result: checks.Target.Docker.Check(context.Background())},
			{Label: "Remoteproc", Result: checks.Target.Remoteproc.Check(context.Background())},
			{Label: "Remoteproc Runtime", Result: checks.Target.RemoteprocRuntime.Check(context.Background())},
			{Label: "Remoteproc Shim", Result: checks.Target.RemoteprocRuntimeShim.Check(context.Background())},
		}
		wantDiscoveryResults := []health.EvaluatedDependency{
			{Label: "Target specified", Result: checks.Target.Specified.Check(context.Background())},
			{Label: "Target access", Result: checks.Target.Connectivity.Check(context.Background())},
			{Label: "Hardware Info", Result: checks.Target.Hardware.Check(context.Background())},
		}
		assert.Equal(t, wantDeploymentResults, got.Deployment.Target)
		assert.Equal(t, wantDiscoveryResults, got.ProjectDiscovery.Target)
	})

	t.Run("shares connectivity and suppresses its dependent target checks", func(t *testing.T) {
		checks := newPassingChecks()
		checks.Target.Connectivity.Check = failingCheck
		healthCheck := health.AssembleHealthCheck(checks)

		got := healthCheck.Evaluate(context.Background())

		wantResults := []health.EvaluatedDependency{
			{Label: "Target specified", Result: checks.Target.Specified.Check(context.Background())},
			{Label: "Target access", Result: checks.Target.Connectivity.Check(context.Background())},
		}
		assert.Equal(t, wantResults, got.Deployment.Target)
		assert.Equal(t, wantResults, got.ProjectDiscovery.Target)
	})

	t.Run("suppresses runtime checks when remoteproc fails", func(t *testing.T) {
		checks := newPassingChecks()
		checks.Target.Remoteproc.Check = failingCheck
		healthCheck := health.AssembleHealthCheck(checks)

		got := healthCheck.Evaluate(context.Background())

		wantDeploymentResults := []health.EvaluatedDependency{
			{Label: "Target specified", Result: checks.Target.Specified.Check(context.Background())},
			{Label: "Target access", Result: checks.Target.Connectivity.Check(context.Background())},
			{Label: "Target Docker", Result: checks.Target.Docker.Check(context.Background())},
			{Label: "Remoteproc", Result: checks.Target.Remoteproc.Check(context.Background())},
		}
		assert.Equal(t, wantDeploymentResults, got.Deployment.Target)
	})

	t.Run("keeps hardware discovery independent from container engine readiness", func(t *testing.T) {
		checks := newPassingChecks()
		checks.Target.Docker.Check = failingCheck
		healthCheck := health.AssembleHealthCheck(checks)

		got := healthCheck.Evaluate(context.Background())

		wantDeploymentResults := []health.EvaluatedDependency{
			{Label: "Target specified", Result: checks.Target.Specified.Check(context.Background())},
			{Label: "Target access", Result: checks.Target.Connectivity.Check(context.Background())},
			{Label: "Target Docker", Result: checks.Target.Docker.Check(context.Background())},
			{Label: "Remoteproc", Result: checks.Target.Remoteproc.Check(context.Background())},
		}
		wantDiscoveryResults := []health.EvaluatedDependency{
			{Label: "Target specified", Result: checks.Target.Specified.Check(context.Background())},
			{Label: "Target access", Result: checks.Target.Connectivity.Check(context.Background())},
			{Label: "Hardware Info", Result: checks.Target.Hardware.Check(context.Background())},
		}
		assert.Equal(t, wantDeploymentResults, got.Deployment.Target)
		assert.Equal(t, wantDiscoveryResults, got.ProjectDiscovery.Target)
	})
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
				{ID: bartek.ID, Label: bartek.Label, Result: bartek.Check(context.Background())},
				{ID: virus.ID, Label: virus.Label, Result: virus.Check(context.Background())},
			}
			assert.Equal(t, want, got.Host)
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
				{ID: flour.ID, Label: flour.Label, Result: flour.Check(context.Background())},
			}
			assert.Equal(t, want, got.Host)
		})
	})
}
