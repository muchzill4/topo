package plugins

import (
	"context"
	"fmt"
	"slices"

	"github.com/arm/topo/pkg/plugin"
)

type FactReport struct {
	Provider string
	Result   plugin.FactsResult
}

func (p *Plugin) Facts(ctx context.Context, runner plugin.TargetRunner) (FactReport, error) {
	if !slices.Contains(p.descriptor.Contracts, plugin.FactsContract) {
		return FactReport{}, fmt.Errorf("plugin %q does not advertise %s", p.path, plugin.FactsContract)
	}
	provider, ok := p.remote.(plugin.FactsProviderPlugin)
	if !ok {
		return FactReport{}, fmt.Errorf("plugin %q has no facts RPC adapter", p.path)
	}
	result, err := provider.Facts(ctx, runner)
	if err != nil {
		return FactReport{}, fmt.Errorf("facts plugin %q: %w", p.path, err)
	}
	if err := result.Validate(); err != nil {
		return FactReport{}, fmt.Errorf("invalid facts from %q: %w", p.path, err)
	}
	return FactReport{Provider: p.descriptor.Name, Result: result}, nil
}

func (c *Collection) Facts(runner plugin.TargetRunner) ([]FactReport, error) {
	reports := make([]FactReport, 0, len(c.plugins))
	for _, loaded := range c.plugins {
		if !slices.Contains(loaded.descriptor.Contracts, plugin.FactsContract) {
			continue
		}
		report, err := loaded.Facts(c.ctx, runner)
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	return reports, nil
}
