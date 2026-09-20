package health

import (
	"context"
	"slices"
)

type CheckStatus string

const (
	CheckStatusOK           CheckStatus = "ok"
	CheckStatusWarning      CheckStatus = "warning"
	CheckStatusError        CheckStatus = "error"
	CheckStatusInfo         CheckStatus = "info"
	CheckStatusUndetermined CheckStatus = "undetermined"
)

type DependencyReport struct {
	Scope     DependencyScope
	ID        DependencyID
	Name      string
	Status    CheckStatus
	Value     string
	Fix       *Fix
	BlockedBy []DependencyBlocker
}

type DependencyBlocker struct {
	Scope DependencyScope
	Name  string
}

type TargetDetails struct {
	Destination string
	IsLocalhost bool
}

type HealthReport struct {
	TargetDetails    TargetDetails
	Deployment       []DependencyReport
	ProjectDiscovery []DependencyReport
}

func Check(ctx context.Context, options HealthCheckOptions) HealthReport {
	healthCheck := NewHealthCheck(options)
	evaluatedHealthCheck := healthCheck.Evaluate(ctx)
	return evaluatedHealthCheck.Report(targetDetails(options))
}

func (h EvaluatedHealthCheck) Report(target TargetDetails) HealthReport {
	deployment := toDependencyReports(h.Deployment.Dependencies)
	discovery := toDependencyReports(h.ProjectDiscovery.Dependencies)
	downgradeMissingTargetForProjectDiscovery(discovery)
	deployment = removeSuccessfulTargetPrerequisiteReports(deployment, target)
	discovery = removeSuccessfulTargetPrerequisiteReports(discovery, target)
	return HealthReport{
		TargetDetails:    target,
		Deployment:       deployment,
		ProjectDiscovery: discovery,
	}
}

func downgradeMissingTargetForProjectDiscovery(reports []DependencyReport) {
	for i := range reports {
		report := &reports[i]
		if report.ID == DependencyIDTargetSpecified && report.Status == CheckStatusError {
			report.Status = CheckStatusWarning
			report.Value = "target not specified; cannot calculate project compatibility"
			return
		}
	}
}

func removeSuccessfulTargetPrerequisiteReports(reports []DependencyReport, target TargetDetails) []DependencyReport {
	return slices.DeleteFunc(reports, func(report DependencyReport) bool {
		isOK := report.Status == CheckStatusOK
		isTargetSpecifiedCheck := report.ID == DependencyIDTargetSpecified
		isLocalhostConnectivityCheck := report.ID == DependencyIDConnectivity && target.IsLocalhost
		return isOK && (isTargetSpecifiedCheck || isLocalhostConnectivityCheck)
	})
}

func targetDetails(options HealthCheckOptions) TargetDetails {
	if options.Target == nil {
		return TargetDetails{}
	}
	return TargetDetails{
		Destination: options.Target.String(),
		IsLocalhost: options.Target.IsPlainLocalhost(),
	}
}

func ToDependencyReport(dependency EvaluatedDependency) DependencyReport {
	report := DependencyReport{Scope: dependency.Scope, ID: dependency.ID, Name: dependency.Label}
	switch dependency.Evaluation.State {
	case EvaluationBlocked:
		report.Status = CheckStatusUndetermined
		report.BlockedBy = dependencyBlockers(dependency.Evaluation.BlockedBy)
		return report
	case EvaluationOmitted:
		panic("cannot report an omitted health dependency")
	case EvaluationExecuted:
	default:
		panic("unknown health dependency evaluation state")
	}

	result := dependency.Evaluation.Result
	if result.Failure == nil {
		report.Status = CheckStatusOK
		report.Value = result.SuccessValue
		return report
	}

	report.Status = checkStatusFromSeverity(result.Failure.Severity)
	report.Value = result.Failure.Message
	report.Fix = result.Failure.Fix
	return report
}

func dependencyBlockers(blockers []*DependencyNode) []DependencyBlocker {
	references := make([]DependencyBlocker, len(blockers))
	for i, blocker := range blockers {
		references[i] = DependencyBlocker{
			Scope: blocker.Scope(),
			Name:  blocker.Dependency().Label,
		}
	}
	return references
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

func toDependencyReports(statuses []EvaluatedDependency) []DependencyReport {
	reports := make([]DependencyReport, len(statuses))
	for i, status := range statuses {
		reports[i] = ToDependencyReport(status)
	}
	return reports
}
