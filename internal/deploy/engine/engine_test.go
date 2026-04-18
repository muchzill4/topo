package engine_test

import (
	"testing"

	"github.com/arm/topo/internal/deploy/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEngine(t *testing.T) {
	t.Run("parses known engines", func(t *testing.T) {
		tests := []struct {
			name string
			want engine.Engine
		}{
			{"docker", engine.Docker},
			{"podman", engine.Podman},
			{"nerdctl", engine.Nerdctl},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := engine.ParseEngine(tt.name)

				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			})
		}
	})

	t.Run("returns error for unknown engine", func(t *testing.T) {
		_, err := engine.ParseEngine("unknown")

		assert.EqualError(t, err, `unknown engine "unknown": supported engines are docker, podman, nerdctl`)
	})
}

func TestEngineString(t *testing.T) {
	assert.Equal(t, "docker", engine.Docker.String())
	assert.Equal(t, "podman", engine.Podman.String())
}
