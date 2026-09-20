package health_test

import (
	"context"
	"testing"

	"github.com/arm/topo/internal/health"
	"github.com/stretchr/testify/assert"
)

func TestEvaluatedHealthCheck(t *testing.T) {
	t.Run("Report", func(t *testing.T) {
		t.Run("projects missing target severity without changing the shared result", func(t *testing.T) {
			dependency := health.EvaluatedDependency{
				ID:    health.DependencyIDTargetSpecified,
				Label: "Target specified",
				Evaluation: health.DependencyEvaluation{Result: health.DependencyCheckResult{Failure: &health.DependencyCheckFailure{
					Severity: health.SeverityError,
					Message:  "target not specified",
					Fix:      &health.Fix{Description: "Choose a target"},
				}}},
			}
			healthCheck := health.EvaluatedHealthCheck{
				Deployment:       health.EvaluatedReadinessCheck{Dependencies: []health.EvaluatedDependency{dependency}},
				ProjectDiscovery: health.EvaluatedReadinessCheck{Dependencies: []health.EvaluatedDependency{dependency}},
			}

			got := healthCheck.Report(health.TargetDetails{})

			assert.Equal(t, []health.DependencyReport{{
				ID: health.DependencyIDTargetSpecified, Name: "Target specified",
				Status: health.CheckStatusError, Value: "target not specified",
				Fix: &health.Fix{Description: "Choose a target"},
			}}, got.Deployment)
			assert.Equal(t, []health.DependencyReport{{
				ID: health.DependencyIDTargetSpecified, Name: "Target specified",
				Status: health.CheckStatusWarning, Value: "target not specified; cannot calculate project compatibility",
				Fix: &health.Fix{Description: "Choose a target"},
			}}, got.ProjectDiscovery)
			assert.Equal(t, health.SeverityError, dependency.Evaluation.Result.Failure.Severity)
		})

		t.Run("hides successful selection and local access", func(t *testing.T) {
			healthCheck := health.EvaluatedHealthCheck{
				Deployment: health.EvaluatedReadinessCheck{Dependencies: []health.EvaluatedDependency{
					{ID: health.DependencyIDTargetSpecified, Label: "Target specified"},
					{ID: health.DependencyIDConnectivity, Label: "Target access"},
					{Label: "Hardware Info", Evaluation: health.DependencyEvaluation{Result: health.DependencyCheckResult{SuccessValue: "lscpu"}}},
				}},
			}

			got := healthCheck.Report(health.TargetDetails{Destination: "localhost", IsLocalhost: true})

			assert.Equal(t, []health.DependencyReport{{
				Name: "Hardware Info", Status: health.CheckStatusOK, Value: "lscpu",
			}}, got.Deployment)
		})

		t.Run("retains remote access results", func(t *testing.T) {
			healthCheck := health.EvaluatedHealthCheck{
				Deployment: health.EvaluatedReadinessCheck{Dependencies: []health.EvaluatedDependency{
					{ID: health.DependencyIDTargetSpecified, Label: "Target specified"},
					{ID: health.DependencyIDConnectivity, Label: "Target access", Evaluation: health.DependencyEvaluation{Result: health.DependencyCheckResult{SuccessValue: "user@example.com"}}},
				}},
			}

			got := healthCheck.Report(health.TargetDetails{Destination: "user@example.com"})

			assert.Equal(t, []health.DependencyReport{{
				ID: health.DependencyIDConnectivity, Name: "Target access", Status: health.CheckStatusOK, Value: "user@example.com",
			}}, got.Deployment)
		})
	})
}

func TestToDependencyReport(t *testing.T) {
	t.Run("propagates scope", func(t *testing.T) {
		dependency := health.EvaluatedDependency{Scope: health.DependencyScopeTarget}

		got := health.ToDependencyReport(dependency)

		want := health.DependencyReport{Scope: health.DependencyScopeTarget, Status: health.CheckStatusOK}
		assert.Equal(t, want, got)
	})

	t.Run("returns successful dependency result", func(t *testing.T) {
		dependency := health.EvaluatedDependency{
			ID:         "docker",
			Label:      "Container Engine",
			Evaluation: health.DependencyEvaluation{Result: health.DependencyCheckResult{SuccessValue: "docker"}},
		}

		got := health.ToDependencyReport(dependency)

		want := health.DependencyReport{
			ID:     "docker",
			Name:   "Container Engine",
			Status: health.CheckStatusOK,
			Value:  "docker",
		}
		assert.Equal(t, want, got)
	})

	t.Run("returns an undetermined report for a blocked dependency", func(t *testing.T) {
		registry := health.NewDependencyRegistry()
		blocker := registry.Register(
			health.Dependency{Label: "Docker CLI", Check: failingCheck},
			health.DependencyRequirements{},
			health.DependencyScopeHost,
		)
		registry.Check(context.Background(), blocker)
		dependency := health.EvaluatedDependency{
			Scope: health.DependencyScopeTarget,
			Label: "Docker daemon",
			Evaluation: health.DependencyEvaluation{
				State:     health.EvaluationBlocked,
				BlockedBy: []*health.DependencyNode{blocker},
			},
		}

		got := health.ToDependencyReport(dependency)

		want := health.DependencyReport{
			Scope:  health.DependencyScopeTarget,
			Name:   "Docker daemon",
			Status: health.CheckStatusUndetermined,
			BlockedBy: []health.DependencyBlocker{{
				Scope: health.DependencyScopeHost,
				Name:  "Docker CLI",
			}},
		}
		assert.Equal(t, want, got)
	})

	t.Run("rejects omitted dependencies", func(t *testing.T) {
		dependency := health.EvaluatedDependency{Evaluation: health.DependencyEvaluation{State: health.EvaluationOmitted}}

		assert.PanicsWithValue(t, "cannot report an omitted health dependency", func() {
			health.ToDependencyReport(dependency)
		})
	})

	t.Run("rejects unknown evaluation states", func(t *testing.T) {
		dependency := health.EvaluatedDependency{Evaluation: health.DependencyEvaluation{State: health.EvaluationState(99)}}

		assert.PanicsWithValue(t, "unknown health dependency evaluation state", func() {
			health.ToDependencyReport(dependency)
		})
	})

	t.Run("returns error dependency result", func(t *testing.T) {
		dependency := health.EvaluatedDependency{
			Label: "Rube Goldberg",
			Evaluation: health.DependencyEvaluation{Result: health.DependencyCheckResult{Failure: &health.DependencyCheckFailure{
				Severity: health.SeverityError,
				Message:  "whatever not found on path",
			}}},
		}

		got := health.ToDependencyReport(dependency)

		want := health.DependencyReport{
			Name:   "Rube Goldberg",
			Status: health.CheckStatusError,
			Value:  "whatever not found on path",
		}
		assert.Equal(t, want, got)
	})

	t.Run("returns warning dependency result", func(t *testing.T) {
		dependency := health.EvaluatedDependency{
			Label: "Remoteproc Runtime",
			Evaluation: health.DependencyEvaluation{Result: health.DependencyCheckResult{Failure: &health.DependencyCheckFailure{
				Severity: health.SeverityWarning,
				Message:  "remoteproc-runtime not found on path",
			}}},
		}

		got := health.ToDependencyReport(dependency)

		want := health.DependencyReport{
			Name:   "Remoteproc Runtime",
			Status: health.CheckStatusWarning,
			Value:  "remoteproc-runtime not found on path",
		}
		assert.Equal(t, want, got)
	})

	t.Run("returns informational dependency result", func(t *testing.T) {
		dependency := health.EvaluatedDependency{
			Label: "Remoteproc Runtime",
			Evaluation: health.DependencyEvaluation{Result: health.DependencyCheckResult{Failure: &health.DependencyCheckFailure{
				Severity: health.SeverityInfo,
				Message:  "no remoteproc devices found",
			}}},
		}

		got := health.ToDependencyReport(dependency)

		want := health.DependencyReport{
			Name:   "Remoteproc Runtime",
			Status: health.CheckStatusInfo,
			Value:  "no remoteproc devices found",
		}
		assert.Equal(t, want, got)
	})

	t.Run("propagates fix from failed dependency", func(t *testing.T) {
		dependency := health.EvaluatedDependency{
			Label: "Food",
			Evaluation: health.DependencyEvaluation{Result: health.DependencyCheckResult{Failure: &health.DependencyCheckFailure{
				Severity: health.SeverityWarning,
				Message:  "not enough pineapple",
				Fix: &health.Fix{
					Description: "add more pineapple",
					Command:     "pizza --pineapple",
				},
			}}},
		}

		got := health.ToDependencyReport(dependency)

		want := health.DependencyReport{
			Name:   "Food",
			Status: health.CheckStatusWarning,
			Value:  "not enough pineapple",
			Fix: &health.Fix{
				Description: "add more pineapple",
				Command:     "pizza --pineapple",
			},
		}
		assert.Equal(t, want, got)
	})
}
