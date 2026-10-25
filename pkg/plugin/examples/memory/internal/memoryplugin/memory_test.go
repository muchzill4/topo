package memoryplugin_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/arm/topo/pkg/plugin"
	"github.com/arm/topo/pkg/plugin/examples/memory/internal/memoryplugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvider(t *testing.T) {
	t.Run("Describe advertises the shared facts contract without probing", func(t *testing.T) {
		got, err := (memoryplugin.Provider{}).Describe(context.Background())

		require.NoError(t, err)
		assert.Equal(t, plugin.Descriptor{Name: "memory", Contracts: []string{plugin.FactsContract}}, got)
	})

	for _, test := range []struct {
		name   string
		output string
		bytes  int64
	}{
		{"converts Linux kB to bytes", "MemTotal: 16384000 kB\nMemFree: 8192000 kB\n", 16384000 * 1024},
		{"preserves zero", "MemTotal: 0 kB\n", 0},
		{"accepts the largest nonoverflowing total", "MemTotal: 9007199254740991 kB\n", math.MaxInt64 / 1024 * 1024},
	} {
		t.Run("Facts "+test.name, func(t *testing.T) {
			runner := &fakeRunner{output: test.output}

			got, err := (memoryplugin.Provider{}).Facts(context.Background(), runner)

			require.NoError(t, err)
			require.NoError(t, got.Validate())
			assert.Equal(t, plugin.FactsResult{
				Groups: []plugin.FactGroup{{ID: "system-memory", Facts: []plugin.Fact{{
					Key: "memory.total", Value: plugin.Value{Type: "integer", Integer: test.bytes},
					Label:       "Total memory",
					Description: "Kernel-reported MemTotal in bytes; not a workload's cgroup memory limit.",
				}}}},
			}, got)
		})
	}

	for _, test := range []struct {
		name, output string
	}{
		{"missing total", "MemFree: 8192000 kB\n"},
		{"invalid total", "MemTotal: notanumber kB\n"},
		{"negative total", "MemTotal: -1 kB\n"},
		{"overflowing total", "MemTotal: 9007199254740992 kB\n"},
		{"missing units", "MemTotal: 123\n"},
		{"unexpected units", "MemTotal: 123 MB\n"},
	} {
		t.Run("Facts rejects "+test.name, func(t *testing.T) {
			runner := &fakeRunner{output: test.output}

			_, err := (memoryplugin.Provider{}).Facts(context.Background(), runner)

			assert.ErrorContains(t, err, "failed to read /proc/meminfo")
		})
	}

	t.Run("Facts omits unavailable memory information", func(t *testing.T) {
		failure := errors.New("target command denied")
		runner := &fakeRunner{err: failure}

		got, err := (memoryplugin.Provider{}).Facts(context.Background(), runner)

		require.NoError(t, err)
		assert.Empty(t, got.Groups)
	})
}

type fakeRunner struct {
	output string
	err    error
}

func (r *fakeRunner) Run(_ context.Context, command string) (string, string, error) {
	if command != "cat /proc/meminfo" {
		return "", "", fmt.Errorf("unexpected command: %s", command)
	}
	return r.output, "", r.err
}
