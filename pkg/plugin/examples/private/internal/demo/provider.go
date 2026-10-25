package demo

import (
	"context"

	"github.com/arm/topo/pkg/plugin"
)

// Provider is synthetic. It detects no real hardware and contains no private IP.
type Provider struct{}

func (Provider) Describe(context.Context) (plugin.Descriptor, error) {
	return plugin.Descriptor{Name: "private-demo", Contracts: []string{plugin.FactsContract}}, nil
}

func (Provider) Facts(context.Context, plugin.TargetRunner) (plugin.FactsResult, error) {
	return plugin.FactsResult{
		Groups: []plugin.FactGroup{{
			ID: "demo",
			Facts: []plugin.Fact{
				{
					Key:         "example.private.demo",
					Value:       plugin.Value{Type: "boolean", Boolean: true},
					Label:       "Private plugin loaded",
					Description: "Demonstrates an unknown capability key without changes to the Topo host.",
				},
				{
					Key:   "example.private.enabled",
					Value: plugin.Value{Type: "boolean", Boolean: false},
					Label: "Synthetic disabled feature",
				},
			},
		}},
	}, nil
}
