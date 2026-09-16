package views_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/arm/topo/internal/health"
	"github.com/arm/topo/internal/output/term"
	"github.com/arm/topo/internal/output/views"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthReport(t *testing.T) {
	t.Run("PlainFormat", func(t *testing.T) {
		t.Run("summarizes functionality checks and hides passing details", func(t *testing.T) {
			report := health.HealthReport{Functionalities: []health.FunctionalityReport{{
				Name:    "Deployment",
				Summary: "Ready to deploy with Docker to pi@edge-a",
				Status:  health.CheckStatusOK,
				Host: []health.DependencyReport{
					{Name: "Topo", Status: health.CheckStatusOK},
					{Name: "Docker Compose", Status: health.CheckStatusWarning, Value: "out of date"},
				},
				Target: []health.DependencyReport{{Name: "Connectivity", Status: health.CheckStatusOK}},
			}}}
			toPrint := views.NewHealthReport(report, "", false)
			var out bytes.Buffer

			err := views.Print(toPrint, &out, term.Plain)

			require.NoError(t, err)
			assert.False(t, strings.HasPrefix(out.String(), "\n"))
			assert.Contains(t, out.String(), "── Deployment: ready (✓ 2, ! 1) ")
			assert.Contains(t, out.String(), " ! Docker Compose (out of date)")
			assert.NotContains(t, out.String(), " ✓ Topo")
			assert.NotContains(t, out.String(), "✓ Target")
		})

		t.Run("renders all direct checks when verbose", func(t *testing.T) {
			report := health.HealthReport{Functionalities: []health.FunctionalityReport{{
				Name:    "Project discovery",
				Summary: "Ready to discover projects on pi@edge-a",
				Status:  health.CheckStatusOK,
				Host:    []health.DependencyReport{{Name: "OpenSSH", Status: health.CheckStatusOK}},
				Target:  []health.DependencyReport{{Name: "Hardware Info", Status: health.CheckStatusOK}},
			}}}
			toPrint := views.NewHealthReport(report, "", true)
			var out bytes.Buffer

			err := views.Print(toPrint, &out, term.Plain)

			require.NoError(t, err)
			assert.Contains(t, out.String(), " ✓ Host")
			assert.Contains(t, out.String(), " ✓ OpenSSH")
			assert.Contains(t, out.String(), " ✓ Target")
			assert.Contains(t, out.String(), " ✓ Hardware Info")
		})

		t.Run("renders failures and fixes", func(t *testing.T) {
			report := health.HealthReport{Functionalities: []health.FunctionalityReport{{
				Name:    "Deployment",
				Summary: "Not ready to deploy with Docker to pi@edge-a",
				Status:  health.CheckStatusError,
				Host: []health.DependencyReport{{
					Name:   "Docker Compose",
					Status: health.CheckStatusError,
					Value:  "not found",
					Fix:    &health.Fix{Description: "Install Docker Compose", Command: "docker compose"},
				}},
			}}}
			toPrint := views.NewHealthReport(report, "", false)
			var out bytes.Buffer

			err := views.Print(toPrint, &out, term.Plain)

			require.NoError(t, err)
			assert.Contains(t, out.String(), "── Deployment: not ready (✗ 1) ")
			assert.Contains(t, out.String(), "Fix:\n     Install Docker Compose")
			assert.Contains(t, out.String(), "Command:\n     docker compose")
		})
	})

	t.Run("JSONFormat", func(t *testing.T) {
		t.Run("preserves the legacy JSON shape", func(t *testing.T) {
			report := health.HealthReport{
				Host: health.HostReport{Dependencies: []health.DependencyReport{{Name: "Time Circuit", Status: health.CheckStatusOK}}},
				Target: &health.TargetReport{
					Destination: "ssh://user@my-target",
					Dependencies: []health.DependencyReport{
						{ID: health.DependencyIDConnectivity, Name: "Connected", Status: health.CheckStatusOK, Value: "ssh://user@my-target"},
						{ID: health.DependencyIDRemoteproc, Name: "Processing Domain Driver (remoteproc)", Status: health.CheckStatusOK, Value: "m4_0"},
						{Name: "Container Engine", Status: health.CheckStatusOK, Value: "docker"},
					},
				},
			}
			toPrint := views.NewHealthReport(report, "", false)
			var out bytes.Buffer

			err := views.Print(toPrint, &out, term.JSON)

			require.NoError(t, err)
			want := `{"host":{"dependencies":[{"name":"Time Circuit","status":"ok","value":""}]},"target":{"destination":"ssh://user@my-target","isLocalhost":false,"connectivity":{"name":"Connected","status":"ok","value":"ssh://user@my-target"},"dependencies":[{"name":"Container Engine","status":"ok","value":"docker"}],"processingDomainDriver":{"name":"Processing Domain Driver (remoteproc)","status":"ok","value":"m4_0"}}}`
			assert.JSONEq(t, want, out.String())
		})
	})
}
