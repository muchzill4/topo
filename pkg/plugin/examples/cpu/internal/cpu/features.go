package cpu

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/arm/topo/pkg/plugin"
)

type Provider struct{}

func (Provider) Describe(context.Context) (plugin.Descriptor, error) {
	return plugin.Descriptor{Name: "cpu", Contracts: []string{plugin.FactsContract}}, nil
}

func (Provider) Facts(ctx context.Context, runner plugin.TargetRunner) (plugin.FactsResult, error) {
	platform, _, err := runner.Run(ctx, "uname -s")
	if err != nil {
		return plugin.FactsResult{}, fmt.Errorf("querying target OS: %w", err)
	}
	if strings.TrimSpace(platform) != "Linux" {
		return plugin.FactsResult{}, nil
	}
	result, err := inventory(ctx, runner)
	if err != nil {
		return plugin.FactsResult{}, err
	}
	sets, err := armFeatureSets(ctx, runner)
	if err != nil {
		return plugin.FactsResult{}, err
	}
	for index := range result.Groups {
		group := &result.Groups[index]
		group.Facts = append(group.Facts, observations(sets[group.ID])...)
	}
	return result, nil
}

func armFeatureSets(ctx context.Context, runner plugin.TargetRunner) (map[string]map[string]bool, error) {
	architecture, _, err := runner.Run(ctx, "uname -m")
	if err != nil {
		return nil, fmt.Errorf("querying target architecture: %w", err)
	}
	architecture = strings.TrimSpace(architecture)
	if architecture != "aarch64" && !strings.HasPrefix(architecture, "arm") {
		return nil, nil
	}
	output, _, err := runner.Run(ctx, "cat /proc/cpuinfo")
	if err != nil {
		return nil, fmt.Errorf("reading target CPU features: %w", err)
	}
	return featureSets(output)
}

func observations(set map[string]bool) []plugin.Fact {
	var facts []plugin.Fact
	features := []struct {
		key, label string
		flags      []string
	}{
		{"arm.cpu.neon", "NEON", []string{"asimd", "neon"}},
		{"arm.cpu.sve", "Scalable Vector Extension", []string{"sve"}},
		{"arm.cpu.sve2", "Scalable Vector Extension 2", []string{"sve2"}},
		{"arm.cpu.sme", "Scalable Matrix Extension", []string{"sme"}},
		{"arm.cpu.sme2", "Scalable Matrix Extension 2", []string{"sme2"}},
	}
	for _, feature := range features {
		found := false
		for _, flag := range feature.flags {
			found = found || set[flag]
		}
		if !found {
			continue
		}
		facts = append(facts, plugin.Fact{
			Key: feature.key, Label: feature.label,
			Value: plugin.Value{Type: "boolean", Boolean: true},
		})
	}
	return facts
}

func featureSets(text string) (map[string]map[string]bool, error) {
	sets := make(map[string]map[string]bool)
	for _, block := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n") {
		var id string
		var set map[string]bool
		for _, line := range strings.Split(block, "\n") {
			key, value, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			switch strings.TrimSpace(key) {
			case "processor":
				number, err := strconv.ParseUint(strings.TrimSpace(value), 10, 32)
				if err != nil {
					return nil, fmt.Errorf("invalid processor ID: %q", value)
				}
				id = fmt.Sprintf("cpu%d", number)
			case "Features":
				set = make(map[string]bool)
				for _, flag := range strings.Fields(value) {
					set[flag] = true
				}
			}
		}
		if id == "" {
			continue
		}
		if _, exists := sets[id]; exists {
			return nil, fmt.Errorf("duplicate processor ID: %q", id)
		}
		sets[id] = set
	}
	return sets, nil
}
