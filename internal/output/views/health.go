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
	Report     health.HealthReport
	TargetHint string
	Verbose    bool
}

func NewHealthReport(report health.HealthReport, targetHint string, verbose bool) HealthReport {
	return HealthReport{Report: report, TargetHint: targetHint, Verbose: verbose}
}

const healthReportTemplate = `{{- define "checkRow" -}}
{{ status .Status }}{{ .Name }}{{- if .Value }} ({{ .Value }}){{- end }}
{{- if .Fix }}
   Fix:
     {{ .Fix.Description }}
  {{- if .Fix.Command }}
   Command:
     {{ .Fix.Command }}
  {{- end }}
{{- end -}}
{{- end -}}
{{- range .Report.Functionalities }}
{{ sectionHeading (functionalityHeading .) }}
{{- $host := checksToRender .Host $.Verbose }}
{{- if $host }}
{{ status (groupStatus $host) }}Host
{{- range $host }}
  {{ template "checkRow" . }}
{{- end }}
{{- end }}
{{- $target := checksToRender .Target $.Verbose }}
{{- if $target }}
{{ status (groupStatus $target) }}Target
{{- range $target }}
  {{ template "checkRow" . }}
{{- end }}
{{- end }}
{{ end }}
{{- if and (not .Report.Target) .TargetHint }}
{{ .TargetHint }}
{{- end }}
`

func (r HealthReport) AsPlain(isTTY bool) (string, error) {
	funcMap := getFuncMap(isTTY)
	funcMap["status"] = healthStatusFormatter(isTTY)
	funcMap["sectionHeading"] = func(heading string) string {
		return sectionHeading(heading, isTTY)
	}
	funcMap["checksToRender"] = checksToRender
	funcMap["functionalityHeading"] = func(report health.FunctionalityReport) string {
		return functionalityHeading(report, healthStatusSymbolFormatter(isTTY))
	}
	funcMap["groupStatus"] = groupStatus
	tmpl, err := template.
		New("healthcheck").
		Funcs(funcMap).
		Parse(healthReportTemplate)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, r); err != nil {
		return "", err
	}

	return strings.TrimPrefix(buf.String(), "\n"), nil
}

func checksToRender(checks []health.DependencyReport, verbose bool) []health.DependencyReport {
	if verbose {
		return checks
	}

	failed := make([]health.DependencyReport, 0, len(checks))
	for _, check := range checks {
		if check.Status != health.CheckStatusOK {
			failed = append(failed, check)
		}
	}
	return failed
}

func groupStatus(checks []health.DependencyReport) health.CheckStatus {
	status := health.CheckStatusOK
	for _, check := range checks {
		if check.Status == health.CheckStatusError {
			return health.CheckStatusError
		}
		if check.Status == health.CheckStatusWarning {
			status = health.CheckStatusWarning
		} else if check.Status == health.CheckStatusInfo && status == health.CheckStatusOK {
			status = health.CheckStatusInfo
		}
	}
	return status
}

func functionalityHeading(report health.FunctionalityReport, statusSymbol func(health.CheckStatus) string) string {
	readiness := "ready"
	if report.Status == health.CheckStatusError {
		readiness = "not ready"
	}
	return fmt.Sprintf("%s: %s (%s)", report.Name, readiness, compactCheckSummary(statusSymbol, report.Host, report.Target))
}

func compactCheckSummary(statusSymbol func(health.CheckStatus) string, groups ...[]health.DependencyReport) string {
	counts := map[health.CheckStatus]int{}
	for _, group := range groups {
		for _, check := range group {
			counts[check.Status]++
		}
	}

	parts := make([]string, 0, len(counts))
	for _, status := range []health.CheckStatus{health.CheckStatusOK, health.CheckStatusError, health.CheckStatusWarning, health.CheckStatusInfo} {
		if count := counts[status]; count > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", statusSymbol(status), count))
		}
	}
	if len(parts) == 0 {
		return "no checks"
	}
	return joinCommaSeparated(parts)
}

func healthStatusSymbolFormatter(isTTY bool) func(health.CheckStatus) string {
	return func(status health.CheckStatus) string {
		symbol, color := "✗", term.Red
		switch status {
		case health.CheckStatusOK:
			symbol, color = "✓", term.Green
		case health.CheckStatusWarning:
			symbol, color = "!", term.Yellow
		case health.CheckStatusInfo:
			symbol, color = "i", term.Blue
		}
		if !isTTY {
			return symbol
		}
		return term.Color(color, symbol)
	}
}

func joinCommaSeparated(parts []string) string {
	result := parts[0]
	for _, part := range parts[1:] {
		result += ", " + part
	}
	return result
}

func (r HealthReport) AsJSON() (string, error) {
	return asJSON(toJSONHealthReport(r.Report))
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

func toJSONHealthReport(report health.HealthReport) jsonHealthReport {
	jsonReport := jsonHealthReport{
		Host: jsonHostReport{Dependencies: toJSONDependencyReports(report.Host.Dependencies)},
	}
	if report.Target != nil {
		jsonTarget := toJSONTargetReport(*report.Target)
		jsonReport.Target = &jsonTarget
	}
	return jsonReport
}

func toJSONTargetReport(report health.TargetReport) jsonTargetReport {
	jsonTarget := jsonTargetReport{
		Destination:            report.Destination,
		IsLocalhost:            report.IsLocalhost,
		Dependencies:           make([]jsonDependencyReport, 0, len(report.Dependencies)),
		ProcessingDomainDriver: jsonDependencyReport{Name: "Processing Domain Driver (remoteproc)"},
	}
	for _, check := range report.Dependencies {
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
