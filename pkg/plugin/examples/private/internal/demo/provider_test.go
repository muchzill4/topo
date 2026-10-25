package demo_test

import (
	"context"
	"testing"

	"github.com/arm/topo/pkg/plugin"
	"github.com/arm/topo/pkg/plugin/examples/private/internal/demo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvider(t *testing.T) {
	t.Run("Describe advertises the facts contract", func(t *testing.T) {
		provider := demo.Provider{}

		got, err := provider.Describe(context.Background())

		require.NoError(t, err)
		assert.Equal(t, plugin.Descriptor{Name: "private-demo", Contracts: []string{plugin.FactsContract}}, got)
	})

	t.Run("Facts returns synthetic observations without target execution", func(t *testing.T) {
		provider := demo.Provider{}

		got, err := provider.Facts(context.Background(), unusedRunner{})

		require.NoError(t, err)
		require.NoError(t, got.Validate())
		require.Len(t, got.Groups, 1)
		assert.Equal(t, "demo", got.Groups[0].ID)
		assert.Equal(t, plugin.Value{Type: "boolean", Boolean: false}, got.Groups[0].Facts[1].Value)
	})
}

type unusedRunner struct{}

func (unusedRunner) Run(context.Context, string) (string, string, error) {
	panic("synthetic facts must not execute target commands")
}
