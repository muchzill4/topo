package podman_test

import (
	"testing"

	"github.com/arm/topo/internal/deploy/podman"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseContainers(t *testing.T) {
	t.Run("decodes the JSON stream emitted by podman compose ps", func(t *testing.T) {
		input := `{"ID":"abc123","Names":"project-web-1","Image":"web","State":"running","Status":"Up 5 minutes","Ports":"0.0.0.0:8080->80/tcp"}
{"ID":"def456","Names":"project-db-1","Image":"db","State":"running","Status":"Up 5 minutes","Ports":""}`

		got, err := podman.ParseContainers(input)

		require.NoError(t, err)
		want := []podman.PSContainer{
			{ID: "abc123", Names: "project-web-1", Image: "web", State: "running", Status: "Up 5 minutes", Ports: "0.0.0.0:8080->80/tcp"},
			{ID: "def456", Names: "project-db-1", Image: "db", State: "running", Status: "Up 5 minutes", Ports: ""},
		}
		assert.Equal(t, want, got)
	})

	t.Run("returns an empty slice for empty input", func(t *testing.T) {
		got, err := podman.ParseContainers("")

		require.NoError(t, err)
		assert.Equal(t, []podman.PSContainer{}, got)
	})

	t.Run("returns an error on malformed JSON", func(t *testing.T) {
		_, err := podman.ParseContainers("{not json")

		assert.Error(t, err)
	})
}

func TestParseProcessingDomains(t *testing.T) {
	t.Run("uses annotations with full and short IDs without a runtime", func(t *testing.T) {
		input := `[{"Id":"adeee04f088edc22fb35cdb671123f7f826b25d257b3d6d28d40803acf5f7a47",
			"Config":{"Annotations":{"remoteproc.name":" m0 "}}}]`

		got, err := podman.ParseProcessingDomains(input)

		require.NoError(t, err)
		assert.Equal(t, map[string]string{
			"adeee04f088edc22fb35cdb671123f7f826b25d257b3d6d28d40803acf5f7a47": "m0",
			"adeee04f088e": "m0",
		}, got)
	})

	t.Run("uses the declared domain regardless of runtime", func(t *testing.T) {
		input := `[{"Id":"host", "OCIRuntime":"crun",
			"Config":{"Annotations":{"remoteproc.name":"m0"}}}]`

		got, err := podman.ParseProcessingDomains(input)

		require.NoError(t, err)
		assert.Equal(t, map[string]string{"host": "m0"}, got)
	})

	t.Run("ignores missing or blank names and missing IDs", func(t *testing.T) {
		input := `[
			{"Id":"missing", "OCIRuntime":"io.containerd.remoteproc.v1", "Config":{}},
			{"Id":"blank", "OCIRuntime":"io.containerd.remoteproc.v1",
			 "Config":{"Annotations":{"remoteproc.name":" "}}},
			{"OCIRuntime":"io.containerd.remoteproc.v1", "Config":{"Annotations":{"remoteproc.name":"m0"}}}
		]`

		got, err := podman.ParseProcessingDomains(input)

		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("accepts an empty container list", func(t *testing.T) {
		got, err := podman.ParseProcessingDomains(`[]`)

		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("rejects malformed inspection output", func(t *testing.T) {
		_, err := podman.ParseProcessingDomains(`{invalid`)

		assert.Error(t, err)
	})
}

func TestRemapAddresses(t *testing.T) {
	t.Run("strips container ports, remaps the hostname, and sets Linux Host", func(t *testing.T) {
		input := []podman.PSContainer{{Ports: "0.0.0.0:8080->80/tcp, 0.0.0.0:8443->443/tcp"}}

		got := podman.RemapAddresses(input, "myhost")

		want := []podman.Container{{
			ProcessingDomain: "Linux Host",
			Address:          "myhost:8080, myhost:8443",
		}}
		assert.Equal(t, want, got)
	})

	t.Run("leaves ports untouched when hostname is empty", func(t *testing.T) {
		input := []podman.PSContainer{{Ports: "0.0.0.0:8080->80/tcp"}}

		got := podman.RemapAddresses(input, "")

		want := []podman.Container{{
			ProcessingDomain: "Linux Host",
			Address:          "0.0.0.0:8080->80/tcp",
		}}
		assert.Equal(t, want, got)
	})
}
