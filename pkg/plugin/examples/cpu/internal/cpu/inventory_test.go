package cpu_test

import (
	"context"
	"testing"

	"github.com/arm/topo/pkg/plugin/examples/cpu/internal/cpu"
	"github.com/stretchr/testify/assert"
)

func TestProviderInventory(t *testing.T) {
	t.Run("returns malformed JSON errors", func(t *testing.T) {
		runner := linuxRunner("")
		runner.outputs["lscpu --json --extended=CPU,MODELNAME"] = "not JSON"

		_, err := (cpu.Provider{}).Facts(context.Background(), runner)

		assert.ErrorContains(t, err, "parsing CPU inventory")
	})

	t.Run("returns missing lscpu errors instead of silently dropping inventory", func(t *testing.T) {
		runner := linuxRunner("")
		delete(runner.outputs, "lscpu --json --extended=CPU,MODELNAME")

		_, err := (cpu.Provider{}).Facts(context.Background(), runner)

		assert.ErrorContains(t, err, "querying CPU inventory")
	})
}
