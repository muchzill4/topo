package compose_test

import (
	"strings"
	"testing"

	"github.com/arm/topo/internal/compose"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPullableServices(t *testing.T) {
	t.Run("returns services without a build key", func(t *testing.T) {
		got, err := compose.PullableServices(strings.NewReader(`
services:
  redis:
    image: redis:7
  postgres:
    image: postgres:16
`))

		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"redis", "postgres"}, got)
	})

	t.Run("excludes services with a build key", func(t *testing.T) {
		got, err := compose.PullableServices(strings.NewReader(`
services:
  app:
    build: .
    image: myapp
  redis:
    image: redis:7
`))

		require.NoError(t, err)
		assert.Equal(t, []string{"redis"}, got)
	})

	t.Run("returns empty slice when all services are buildable", func(t *testing.T) {
		got, err := compose.PullableServices(strings.NewReader(`
services:
  app:
    build: .
  worker:
    build:
      context: .
      dockerfile: Dockerfile.worker
`))

		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("returns error for invalid yaml", func(t *testing.T) {
		_, err := compose.PullableServices(strings.NewReader(`{invalid`))

		assert.Error(t, err)
	})
}

func TestImageNames(t *testing.T) {
	t.Run("uses explicit image name", func(t *testing.T) {
		got, err := compose.ImageNames(strings.NewReader(`
services:
  app:
    image: myapp:latest
  db:
    image: postgres:16
`), "myproject")

		require.NoError(t, err)
		assert.Equal(t, []string{"myapp:latest", "postgres:16"}, got)
	})

	t.Run("derives image name from project and service when no image key", func(t *testing.T) {
		got, err := compose.ImageNames(strings.NewReader(`
services:
  app:
    build: .
  worker:
    build: .
`), "myproject")

		require.NoError(t, err)
		assert.Equal(t, []string{"myproject-app", "myproject-worker"}, got)
	})

	t.Run("uses top-level name as project name", func(t *testing.T) {
		got, err := compose.ImageNames(strings.NewReader(`
name: custom
services:
  app:
    build: .
`), "fallback")

		require.NoError(t, err)
		assert.Equal(t, []string{"custom-app"}, got)
	})

	t.Run("prefers explicit image over derived name", func(t *testing.T) {
		got, err := compose.ImageNames(strings.NewReader(`
services:
  app:
    build: .
    image: myapp:v2
`), "myproject")

		require.NoError(t, err)
		assert.Equal(t, []string{"myapp:v2"}, got)
	})

	t.Run("returns error for invalid yaml", func(t *testing.T) {
		_, err := compose.ImageNames(strings.NewReader(`{invalid`), "p")

		assert.Error(t, err)
	})
}

func TestProjectName(t *testing.T) {
	t.Run("returns directory name from absolute path", func(t *testing.T) {
		got := compose.ProjectName("/home/user/topo-welcome/compose.yaml")

		assert.Equal(t, "topo-welcome", got)
	})

	t.Run("resolves relative path before extracting directory", func(t *testing.T) {
		got := compose.ProjectName("compose.yaml")

		assert.NotEqual(t, ".", got)
	})
}
