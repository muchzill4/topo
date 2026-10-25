package memoryplugin

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/arm/topo/pkg/plugin"
)

type Provider struct{}

func (Provider) Describe(context.Context) (plugin.Descriptor, error) {
	return plugin.Descriptor{Name: "memory", Contracts: []string{plugin.FactsContract}}, nil
}

func (Provider) Facts(ctx context.Context, runner plugin.TargetRunner) (plugin.FactsResult, error) {
	output, _, err := runner.Run(ctx, "cat /proc/meminfo")
	if err != nil {
		if ctx.Err() != nil {
			return plugin.FactsResult{}, ctx.Err()
		}
		// Unavailable memory information is not evidence of zero memory.
		return plugin.FactsResult{}, nil
	}
	bytes, err := totalMemoryBytes(output)
	if err != nil {
		return plugin.FactsResult{}, fmt.Errorf("failed to read /proc/meminfo: %w", err)
	}
	return plugin.FactsResult{
		Groups: []plugin.FactGroup{{
			ID: "system-memory",
			Facts: []plugin.Fact{{
				Key: "memory.total", Value: plugin.Value{Type: "integer", Integer: bytes},
				Label:       "Total memory",
				Description: "Kernel-reported MemTotal in bytes; not a workload's cgroup memory limit.",
			}},
		}},
	}, nil
}

func totalMemoryBytes(text string) (int64, error) {
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 || fields[0] != "MemTotal:" {
			continue
		}
		if len(fields) != 3 || fields[2] != "kB" {
			return 0, fmt.Errorf("MemTotal must use Linux kB units (1024 bytes)")
		}
		kibibytes, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0, err
		}
		if kibibytes < 0 || kibibytes > math.MaxInt64/1024 {
			return 0, fmt.Errorf("MemTotal is negative or overflows bytes: %q", fields[1])
		}
		return kibibytes * 1024, nil
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return 0, fmt.Errorf("field MemTotal not found")
}
