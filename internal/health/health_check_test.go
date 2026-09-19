package health_test

import (
	"context"
	"testing"

	"github.com/arm/topo/internal/health"
	"github.com/arm/topo/internal/ssh"
	"github.com/stretchr/testify/assert"
)

func TestHealthCheck(t *testing.T) {
	t.Run("Evaluate", func(t *testing.T) {
		t.Run("evaluates deployment and project discovery checks", func(t *testing.T) {
			registry := health.NewDependencyRegistry()
			deployment := health.Dependency{ID: "deployment", Check: passingCheck}
			deploymentRef := registry.Register(deployment)
			projectDiscovery := health.Dependency{ID: "project-discovery", Check: failingCheck}
			projectDiscoveryRef := registry.Register(projectDiscovery)
			healthCheck := health.HealthCheck{
				Deployment:       health.ReadinessCheck{Registry: registry, Host: []*health.DependencyNode{deploymentRef}},
				ProjectDiscovery: health.ReadinessCheck{Registry: registry, Target: []*health.DependencyNode{projectDiscoveryRef}},
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

func TestNewHealthCheck(t *testing.T) {
	t.Run("evaluates a missing target without constructing target operations", func(t *testing.T) {
		check := health.NewHealthCheck(health.HealthCheckOptions{MissingTargetFixMessage: "Choose a target"})
		check.Deployment.Host = nil

		got := check.Deployment.Evaluate(context.Background())

		assert.Equal(t, []health.EvaluatedDependency{{
			ID:    health.DependencyIDTargetSpecified,
			Label: "Target specified",
			Result: health.DependencyCheckResult{Failure: &health.DependencyCheckFailure{
				Severity: health.SeverityError,
				Message:  "target not specified",
				Fix:      &health.Fix{Description: "Choose a target"},
			}},
		}}, got.Target)
	})

	t.Run("plain localhost access succeeds without SSH", func(t *testing.T) {
		target := ssh.NewDestination("localhost")
		check := health.NewHealthCheck(health.HealthCheckOptions{Target: &target})
		// Evaluate only target selection and access, not machine-dependent probes.
		access := check.Deployment.Target[1]

		got, checked := check.Deployment.Registry.Check(context.Background(), access)

		assert.True(t, checked)
		assert.Equal(t, health.DependencyCheckResult{SuccessValue: "local"}, got)
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
				Podman: passing("Host Podman"), PodmanCompose: passing("Host Compose"),
			},
			Target: health.TargetChecks{
				Specified: passing("Target specified"), Connectivity: passing("Target access"), Docker: passing("Target Docker"),
				Podman: passing("Target Podman"), Hardware: passing("Hardware Info"), Remoteproc: passing("Remoteproc"),
				RemoteprocRuntime: passing("Remoteproc Runtime"), RemoteprocRuntimeShim: passing("Remoteproc Shim"),
			},
		}
	}

	t.Run("uses the Docker graph", func(t *testing.T) {
		checks := newPassingChecks()
		healthCheck := health.AssembleHealthCheck(health.EngineDocker, checks)

		got := healthCheck.Evaluate(context.Background())

		assert.Equal(t, []string{"Topo", "OpenSSH", "Container Engine", "Docker Compose"}, dependencyLabels(got.Deployment.Host))
		assert.Equal(t, []string{"Target specified", "Target access", "Target Docker", "Remoteproc", "Remoteproc Runtime", "Remoteproc Shim"}, dependencyLabels(got.Deployment.Target))
		assert.Equal(t, []string{"Target specified", "Target access", "Hardware Info"}, dependencyLabels(got.ProjectDiscovery.Target))
	})

	t.Run("shares a failed target prerequisite without executing target checks", func(t *testing.T) {
		checks := newPassingChecks()
		checks.Target.Specified.Check = failingCheck
		healthCheck := health.AssembleHealthCheck(health.EngineDocker, checks)

		got := healthCheck.Evaluate(context.Background())

		assert.Equal(t, []string{"Target specified"}, dependencyLabels(got.Deployment.Target))
		assert.Equal(t, []string{"Target specified"}, dependencyLabels(got.ProjectDiscovery.Target))
	})

	t.Run("keeps hardware discovery independent from Docker readiness", func(t *testing.T) {
		checks := newPassingChecks()
		checks.Target.Docker.Check = failingCheck
		healthCheck := health.AssembleHealthCheck(health.EngineDocker, checks)

		got := healthCheck.Evaluate(context.Background())

		assert.Equal(t, []string{"Target specified", "Target access", "Target Docker", "Remoteproc"}, dependencyLabels(got.Deployment.Target))
		assert.Equal(t, []string{"Target specified", "Target access", "Hardware Info"}, dependencyLabels(got.ProjectDiscovery.Target))
	})

	t.Run("uses the Podman graph", func(t *testing.T) {
		checks := newPassingChecks()
		healthCheck := health.AssembleHealthCheck(health.EnginePodman, checks)

		got := healthCheck.Evaluate(context.Background())

		assert.Equal(t, []string{"Topo", "OpenSSH", "Host Podman", "Host Compose"}, dependencyLabels(got.Deployment.Host))
		assert.Equal(t, []string{"Target specified", "Target access", "Target Podman"}, dependencyLabels(got.Deployment.Target))
		assert.Equal(t, []string{"Target specified", "Target access", "Hardware Info"}, dependencyLabels(got.ProjectDiscovery.Target))
	})

	t.Run("evaluates target Podman when host Compose fails", func(t *testing.T) {
		checks := newPassingChecks()
		checks.Host.PodmanCompose.Check = failingCheck
		healthCheck := health.AssembleHealthCheck(health.EnginePodman, checks)

		got := healthCheck.Evaluate(context.Background())

		assert.Equal(t, []string{"Target specified", "Target access", "Target Podman"}, dependencyLabels(got.Deployment.Target))
		assert.Equal(t, []string{"Target specified", "Target access", "Hardware Info"}, dependencyLabels(got.ProjectDiscovery.Target))
	})

	t.Run("falls back to Docker for an unrecognized engine", func(t *testing.T) {
		checks := newPassingChecks()
		healthCheck := health.AssembleHealthCheck("other", checks)

		got := healthCheck.Evaluate(context.Background())

		assert.Equal(t, []string{"Target specified", "Target access", "Target Docker", "Remoteproc", "Remoteproc Runtime", "Remoteproc Shim"}, dependencyLabels(got.Deployment.Target))
	})
}

func dependencyLabels(dependencies []health.EvaluatedDependency) []string {
	labels := make([]string, len(dependencies))
	for i, dependency := range dependencies {
		labels[i] = dependency.Label
	}
	return labels
}

func TestReadinessCheck(t *testing.T) {
	t.Run("Evaluate", func(t *testing.T) {
		t.Run("reports successful dependencies in the group", func(t *testing.T) {
			registry := health.NewDependencyRegistry()
			virus := health.Dependency{ID: "virus", Check: passingCheck}
			virusRef := registry.Register(virus)
			bartek := health.Dependency{ID: "bartek", Check: passingCheck}
			bartekRef := registry.Register(bartek, virusRef)

			healthCheck := health.ReadinessCheck{
				Registry: registry,
				Host:     []*health.DependencyNode{bartekRef, virusRef},
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
			flourRef := registry.Register(flour)
			pizza := health.Dependency{ID: "pizza", Check: passingCheck}
			pizzaRef := registry.Register(pizza, flourRef)

			healthCheck := health.ReadinessCheck{
				Registry: registry,
				Host:     []*health.DependencyNode{flourRef, pizzaRef},
			}

			got := healthCheck.Evaluate(context.Background())

			want := []health.EvaluatedDependency{
				{ID: flour.ID, Label: flour.Label, Result: flour.Check(context.Background())},
			}
			assert.Equal(t, want, got.Host)
		})
	})
}
