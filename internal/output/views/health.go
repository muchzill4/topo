package views

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/arm/topo/internal/health"
	"github.com/arm/topo/internal/output/term"
)

type HealthReport struct {
	TargetDetails    health.TargetDetails
	Deployment       []health.DependencyReport
	ProjectDiscovery []health.DependencyReport
}

const functionalityHealthReportTemplate = `
{{- define "checkRow" -}}
{{ "  " }}{{ status .Status }}{{ .Name }}{{- if .Value }} ({{ .Value }}){{- end }}
{{- if .Fix }}
     Fix:
       {{ .Fix.Description }}
  {{- if .Fix.Command }}
     Command:
       {{ .Fix.Command }}
  {{- end }}
{{- end -}}
{{- end -}}

{{- define "functionality" -}}
{{ functionalityHeading .Name .Report }}
{{ status (dependencyGroupStatus .Host) }}Host
{{- range .Host }}
{{ template "checkRow" . }}
{{- end }}
{{ status (dependencyGroupStatus .Target) }}Target
{{- range .Target }}
{{ template "checkRow" . }}
{{- end }}
{{- end -}}

{{ template "functionality" (buildFunctionalityTemplateData "Deployment" .Deployment) }}

{{ template "functionality" (buildFunctionalityTemplateData "Project management" .ProjectDiscovery) }}
`

type functionalityTemplateData struct {
	Name   string
	Report []health.DependencyReport
	Host   []health.DependencyReport
	Target []health.DependencyReport
}

func (r HealthReport) AsPlain(isTTY bool) (string, error) {
	funcMap := getFuncMap(isTTY)
	funcMap["status"] = healthStatusFormatter(isTTY)
	funcMap["buildFunctionalityTemplateData"] = func(name string, report []health.DependencyReport) functionalityTemplateData {
		return functionalityTemplateData{
			Name:   name,
			Report: report,
			Host:   dependenciesInScope(report, health.DependencyScopeHost),
			Target: dependenciesInScope(report, health.DependencyScopeTarget),
		}
	}
	funcMap["functionalityHeading"] = func(name string, report []health.DependencyReport) string {
		return functionalityHeading(name, report, isTTY)
	}
	funcMap["dependencyGroupStatus"] = dependencyGroupStatus
	tmpl, err := template.New("functionality-healthcheck").Funcs(funcMap).Parse(functionalityHealthReportTemplate)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, r); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (r HealthReport) AsJSON() (string, error) {
	targetDependencies := legacyTargetDependencies(
		dependenciesInScope(r.Deployment, health.DependencyScopeTarget),
		dependenciesInScope(r.ProjectDiscovery, health.DependencyScopeTarget),
	)
	return asJSON(toJSONHealthReport(legacyHealthReport{
		HostDependencies:   dependenciesInScope(r.Deployment, health.DependencyScopeHost),
		TargetDependencies: targetDependencies,
		TargetDetails:      r.TargetDetails,
	}))
}

type legacyHealthReport struct {
	HostDependencies   []health.DependencyReport
	TargetDependencies []health.DependencyReport
	TargetDetails      health.TargetDetails
}

func legacyTargetDependencies(deployment, projectDiscovery []health.DependencyReport) []health.DependencyReport {
	targetDependencies := append([]health.DependencyReport(nil), deployment...)
	for _, dependency := range projectDiscovery {
		if dependency.ID != health.DependencyIDConnectivity {
			targetDependencies = append(targetDependencies, dependency)
		}
	}
	return targetDependencies
}

func functionalityHeading(name string, report []health.DependencyReport, isTTY bool) string {
	statusCount := countStatuses(report)
	if statusCount.errors == 0 && statusCount.warnings == 0 {
		return sectionHeading(name+": ready", isTTY)
	}

	readiness := "ready"
	if statusCount.errors > 0 {
		readiness = "not ready"
	}

	indicators := make([]string, 0, 2)
	if statusCount.errors > 0 {
		indicators = append(indicators, statusIndicator("✗", term.Red, statusCount.errors, isTTY))
	}
	if statusCount.warnings > 0 {
		indicators = append(indicators, statusIndicator("!", term.Yellow, statusCount.warnings, isTTY))
	}

	heading := fmt.Sprintf("%s: %s (%s)", name, readiness, strings.Join(indicators, " "))
	return sectionHeading(heading, isTTY)
}

func statusIndicator(symbol, color string, count uint, isTTY bool) string {
	if isTTY {
		symbol = term.Color(color, symbol)
	}
	return fmt.Sprintf("%s %d", symbol, count)
}

func countStatuses(report []health.DependencyReport) (statusCount struct{ warnings, errors uint }) {
	for _, dependency := range report {
		switch dependency.Status {
		case health.CheckStatusWarning:
			statusCount.warnings++
		case health.CheckStatusError:
			statusCount.errors++
		}
	}
	return
}

func dependenciesInScope(dependencies []health.DependencyReport, scope health.DependencyScope) []health.DependencyReport {
	matching := make([]health.DependencyReport, 0, len(dependencies))
	for _, dependency := range dependencies {
		if dependency.Scope == scope {
			matching = append(matching, dependency)
		}
	}
	return matching
}

func dependencyGroupStatus(dependencies []health.DependencyReport) health.CheckStatus {
	status := health.CheckStatusOK
	for _, dependency := range dependencies {
		if dependency.Status == health.CheckStatusError {
			return health.CheckStatusError
		}
		if dependency.Status == health.CheckStatusWarning {
			status = health.CheckStatusWarning
		}
	}
	return status
}

func sectionHeading(heading string, isTTY bool) string {
	return term.Header(heading, isTTY)
}

func healthStatusFormatter(isTTY bool) func(health.CheckStatus) string {
	return func(status health.CheckStatus) string {
		label, color := " ✗ ", term.Red
		switch status {
		case health.CheckStatusOK:
			label, color = " ✓ ", term.Green
		case health.CheckStatusWarning:
			label, color = " ! ", term.Yellow
		case health.CheckStatusInfo:
			label, color = " i ", term.Blue
		}
		if !isTTY {
			return label
		}
		return term.Color(color, label)
	}
}

type jsonHealthReport struct {
	Host   jsonHostReport    `json:"host"`
	Target *jsonTargetReport `json:"target,omitempty"`
}

type jsonHostReport struct {
	Dependencies []jsonDependencyReport `json:"dependencies"`
}

type jsonTargetReport struct {
	Destination            string                 `json:"destination"`
	IsLocalhost            bool                   `json:"isLocalhost"`
	Connectivity           jsonDependencyReport   `json:"connectivity"`
	Dependencies           []jsonDependencyReport `json:"dependencies"`
	ProcessingDomainDriver jsonDependencyReport   `json:"processingDomainDriver"`
}

type jsonDependencyReport struct {
	Name   string             `json:"name"`
	Status health.CheckStatus `json:"status"`
	Value  string             `json:"value"`
	Fix    *jsonFix           `json:"fix,omitempty"`
}

type jsonFix struct {
	Description string `json:"description"`
	Command     string `json:"command,omitempty"`
}

func toJSONHealthReport(report legacyHealthReport) jsonHealthReport {
	jsonReport := jsonHealthReport{
		Host: jsonHostReport{Dependencies: toJSONDependencyReports(report.HostDependencies)},
	}
	if report.TargetDetails.Destination != "" {
		jsonTarget := toJSONTargetReport(report.TargetDependencies, report.TargetDetails)
		jsonReport.Target = &jsonTarget
	}
	return jsonReport
}

func toJSONTargetReport(dependencies []health.DependencyReport, details health.TargetDetails) jsonTargetReport {
	jsonTarget := jsonTargetReport{
		Destination:            details.Destination,
		IsLocalhost:            details.IsLocalhost,
		Dependencies:           make([]jsonDependencyReport, 0, len(dependencies)),
		ProcessingDomainDriver: jsonDependencyReport{Name: "Processing Domain Driver (remoteproc)"},
	}
	for _, check := range dependencies {
		jsonCheck := toJSONDependencyReport(check)
		switch check.ID {
		case health.DependencyIDConnectivity:
			jsonTarget.Connectivity = jsonCheck
		case health.DependencyIDRemoteproc:
			jsonTarget.ProcessingDomainDriver = jsonCheck
		default:
			jsonTarget.Dependencies = append(jsonTarget.Dependencies, jsonCheck)
		}
	}
	return jsonTarget
}

func toJSONDependencyReports(checks []health.DependencyReport) []jsonDependencyReport {
	jsonChecks := make([]jsonDependencyReport, len(checks))
	for index, check := range checks {
		jsonChecks[index] = toJSONDependencyReport(check)
	}
	return jsonChecks
}

func toJSONDependencyReport(check health.DependencyReport) jsonDependencyReport {
	jsonCheck := jsonDependencyReport{Name: check.Name, Status: check.Status, Value: check.Value}
	if check.Fix != nil {
		jsonCheck.Fix = &jsonFix{Description: check.Fix.Description, Command: check.Fix.Command}
	}
	return jsonCheck
}
