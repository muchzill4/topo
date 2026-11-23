// Package plugin provides registration and serving for Go-only Topo plugins.
package plugin

import (
	"context"

	"github.com/arm/topo/pkg/plugin/suggestions"
	"github.com/arm/topo/pkg/plugin/transport"
	goplugin "github.com/hashicorp/go-plugin"
)

func ServeSuggestions(identity string, provider suggestions.Provider) {
	descriptor := transport.Descriptor{ID: identity, Capability: transport.Suggestions}
	if err := descriptor.Validate(); err != nil {
		panic(err)
	}
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: transport.Handshake,
		Plugins: goplugin.PluginSet{
			transport.DescribeKey:         &transport.DescribeRPCPlugin{Impl: registration{descriptor: descriptor}},
			string(transport.Suggestions): &transport.SuggestionsRPCPlugin{Impl: provider},
		},
	})
}

type registration struct {
	descriptor transport.Descriptor
}

func (r registration) Describe(context.Context) (transport.Descriptor, error) {
	return r.descriptor, nil
}
