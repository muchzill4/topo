package printable

import (
	"bytes"
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"text/template"

	"github.com/arm/topo/internal/deploy"
)

type PS struct {
	Containers []deploy.Container `json:"containers"`
}

const psTemplate = `{{if .}}Image	Status	Processing Domain	Address
{{- range .}}
{{.Image}}	{{.Status}}	{{.ProcessingDomain}}	{{.Address}}
{{- end }}{{else}}No containers deployed from this project are running.{{end}}`

func (r PS) AsPlain(isTTY bool) (string, error) {
	tmpl, err := template.
		New("ps").
		Parse(psTemplate)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	const columnPadding = 3
	w := tabwriter.NewWriter(&buf, 0, 0, columnPadding, ' ', 0)
	if err := tmpl.Execute(w, r.Containers); err != nil {
		return "", err
	}
	err = w.Flush()
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

func (r PS) AsJSON() (string, error) {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode report as json: %w", err)
	}
	return string(b), nil
}
