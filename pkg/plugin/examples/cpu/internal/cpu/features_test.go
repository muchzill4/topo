package cpu_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/arm/topo/pkg/plugin"
	"github.com/arm/topo/pkg/plugin/examples/cpu/internal/cpu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvider(t *testing.T) {
	t.Run("Describe advertises facts without running commands", func(t *testing.T) {
		got, err := (cpu.Provider{}).Describe(context.Background())

		require.NoError(t, err)
		assert.Equal(t, plugin.Descriptor{Name: "cpu", Contracts: []string{plugin.FactsContract}}, got)
	})

	t.Run("Facts reports per-processor features", func(t *testing.T) {
		runner := linuxRunner("processor: 0\nFeatures: asimd sve\n\nprocessor: 1\nFeatures: asimd sve\n")

		got, err := (cpu.Provider{}).Facts(context.Background(), runner)

		require.NoError(t, err)
		require.NoError(t, got.Validate())
		assert.Equal(t, plugin.Fact{
			Key:   "arm.cpu.sve",
			Value: plugin.Value{Type: "boolean", Boolean: true},
			Label: "Scalable Vector Extension",
		}, factsFor(got, "cpu0")["arm.cpu.sve"])
	})

	t.Run("Facts does not promote one processor's feature to every processor", func(t *testing.T) {
		runner := linuxRunner("processor: 0\nFeatures: asimd sve\n\nprocessor: 1\nFeatures: asimd\n")

		got, err := (cpu.Provider{}).Facts(context.Background(), runner)

		require.NoError(t, err)
		assert.NotContains(t, factsFor(got, "cpu1"), "arm.cpu.sve")
	})

	t.Run("Facts omits features when a processor record lacks evidence", func(t *testing.T) {
		runner := linuxRunner("processor: 0\nFeatures: asimd sve\n\nprocessor: 1\nCPU implementer: 0x41\n")

		got, err := (cpu.Provider{}).Facts(context.Background(), runner)

		require.NoError(t, err)
		assert.NotContains(t, factsFor(got, "cpu1"), "arm.cpu.neon")
	})

	t.Run("Facts recognizes the Arm32 NEON spelling", func(t *testing.T) {
		runner := linuxRunner("processor: 0\nFeatures: neon\n")
		runner.outputs["uname -m"] = "armv7l\n"
		runner.outputs["lscpu --json --extended=CPU,MODELNAME"] = `{"cpus":[{"cpu":0,"modelname":"Test CPU"}]}`

		got, err := (cpu.Provider{}).Facts(context.Background(), runner)

		require.NoError(t, err)
		assert.Equal(t, plugin.Value{Type: "boolean", Boolean: true},
			factsFor(got, "cpu0")["arm.cpu.neon"].Value)
	})

	t.Run("Facts omits unreported features instead of claiming hardware absence", func(t *testing.T) {
		runner := linuxRunner("processor: 0\nFeatures: asimd\n")

		got, err := (cpu.Provider{}).Facts(context.Background(), runner)

		require.NoError(t, err)
		assert.NotContains(t, factsFor(got, "cpu0"), "arm.cpu.sme")
	})

	t.Run("Facts omits unsupported OS observations without invoking Linux commands", func(t *testing.T) {
		runner := &fakeRunner{outputs: map[string]string{"uname -s": "Darwin\n"}}

		got, err := (cpu.Provider{}).Facts(context.Background(), runner)

		require.NoError(t, err)
		assert.Empty(t, got.Groups)
		assert.NoError(t, got.Validate())
	})

	t.Run("Facts inventories non-Arm CPUs without reading procfs features", func(t *testing.T) {
		runner := linuxRunner("")
		runner.outputs["uname -m"] = "x86_64\n"
		delete(runner.outputs, "cat /proc/cpuinfo")

		got, err := (cpu.Provider{}).Facts(context.Background(), runner)

		require.NoError(t, err)
		assert.NotContains(t, factsFor(got, "cpu0"), "arm.cpu.neon")
		assert.Equal(t, plugin.Value{Type: "string", Text: "Test CPU"},
			factsFor(got, "cpu0")["cpu.model"].Value)
	})

	t.Run("Facts returns target execution failures", func(t *testing.T) {
		failure := errors.New("target disconnected")
		runner := &fakeRunner{err: failure}

		_, err := (cpu.Provider{}).Facts(context.Background(), runner)

		assert.ErrorIs(t, err, failure)
	})
}

func factsFor(result plugin.FactsResult, id string) map[string]plugin.Fact {
	facts := make(map[string]plugin.Fact)
	for _, group := range result.Groups {
		if group.ID != id {
			continue
		}
		for _, fact := range group.Facts {
			facts[fact.Key] = fact
		}
	}
	return facts
}

func linuxRunner(cpuinfo string) *fakeRunner {
	return &fakeRunner{outputs: map[string]string{
		"uname -s": "Linux\n", "uname -m": "aarch64\n", "cat /proc/cpuinfo": cpuinfo,
		"lscpu --json --extended=CPU,MODELNAME": `{"cpus":[{"cpu":0,"modelname":"Test CPU"},{"cpu":1,"modelname":"Test CPU"}]}`,
	}}
}

type fakeRunner struct {
	outputs map[string]string
	err     error
}

func (r *fakeRunner) Run(_ context.Context, command string) (string, string, error) {
	if r.err != nil {
		return "", "", r.err
	}
	output, ok := r.outputs[command]
	if !ok {
		return "", "", fmt.Errorf("unexpected command: %s", command)
	}
	return output, "", nil
}
