package health

import (
	"context"
	"fmt"

	"github.com/arm/topo/internal/ssh"
)

type CheckStatus string

const (
	CheckStatusOK      CheckStatus = "ok"
	CheckStatusWarning CheckStatus = "warning"
	CheckStatusError   CheckStatus = "error"
	CheckStatusInfo    CheckStatus = "info"
)

type DependencyReport struct {
	ID     DependencyID
	Name   string
	Status CheckStatus
	Value  string
	Fix    *Fix
}

type HostReport struct {
	Dependencies []DependencyReport
}

type TargetReport struct {
	Destination  string
	IsLocalhost  bool
	Dependencies []DependencyReport
}

type FunctionalityReport struct {
	Name    string
	Summary string
	Status  CheckStatus
	Host    []DependencyReport
	Target  []DependencyReport
}

type HealthReport struct {
	Functionalities []FunctionalityReport

	// Host and Target preserve the JSON output contract during the transition to
	// functionality-centric health reporting.
	Host   HostReport
	Target *TargetReport
}

func Check(ctx context.Context, options DependencyGraphOptions) HealthReport {
	graph := NewDependencyGraph(options)
	evaluatedGraph := graph.Evaluate(ctx)
	report := HealthReport{
		Functionalities: toFunctionalityReports(evaluatedGraph.Functionalities, options.Target),
		Host:            HostReport{Dependencies: toDependencyReports(evaluatedGraph.Host)},
	}
	if options.Target == nil {
		return report
	}

	targetReport := TargetReport{
		Destination:  options.Target.String(),
		IsLocalhost:  options.Target.IsPlainLocalhost(),
		Dependencies: toDependencyReports(evaluatedGraph.Target),
	}
	report.Target = &targetReport
	return report
}

func toFunctionalityReports(groups []EvaluatedFunctionalityGroup, target *ssh.Destination) []FunctionalityReport {
	reports := make([]FunctionalityReport, len(groups))
	for index, group := range groups {
		host := toDependencyReports(group.Host)
		targetReports := toDependencyReports(group.Target)
		status := functionalityStatus(host, targetReports)
		reports[index] = FunctionalityReport{
			Name:    group.Name,
			Summary: functionalitySummary(group.Name, status, target),
			Status:  status,
			Host:    host,
			Target:  targetReports,
		}
	}
	return reports
}

func functionalityStatus(groups ...[]DependencyReport) CheckStatus {
	for _, group := range groups {
		for _, dependency := range group {
			if dependency.Status == CheckStatusError {
				return CheckStatusError
			}
		}
	}
	return CheckStatusOK
}

func functionalitySummary(name string, status CheckStatus, target *ssh.Destination) string {
	ready := "Ready"
	if status == CheckStatusError {
		ready = "Not ready"
	}

	switch name {
	case "Deployment":
		if target == nil {
			return ready + " to deploy with Docker"
		}
		return fmt.Sprintf("%s to deploy with Docker to %s", ready, target)
	case "Project discovery":
		if target == nil {
			return ready + " to discover projects"
		}
		return fmt.Sprintf("%s to discover projects on %s", ready, target)
	default:
		return ready + " for " + name
	}
}

func ToDependencyReport(status DependencyStatus) DependencyReport {
	report := DependencyReport{ID: status.ID, Name: status.Label}
	if status.Result.Failure == nil {
		report.Status = CheckStatusOK
		report.Value = status.Result.SuccessValue
		return report
	}

	report.Status = checkStatusFromSeverity(status.Result.Failure.Severity)
	report.Value = status.Result.Failure.Message
	report.Fix = status.Result.Failure.Fix
	return report
}

func checkStatusFromSeverity(severity CheckSeverity) CheckStatus {
	switch severity {
	case SeverityWarning:
		return CheckStatusWarning
	case SeverityInfo:
		return CheckStatusInfo
	default:
		return CheckStatusError
	}
}

func toDependencyReports(statuses []DependencyStatus) []DependencyReport {
	reports := make([]DependencyReport, len(statuses))
	for i, status := range statuses {
		reports[i] = ToDependencyReport(status)
	}
	return reports
}
