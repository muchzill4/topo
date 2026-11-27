// Package plugin provides registration and serving for Go-only Topo plugins.
package plugin

import (
	"github.com/arm/topo/pkg/plugin/observations"
	"github.com/arm/topo/pkg/plugin/suggestions"
	"github.com/arm/topo/pkg/plugin/transport"
	goplugin "github.com/hashicorp/go-plugin"
)

func ServeObservations(identity string, provider observations.Provider) {
	serve(identity, transport.Observations, &transport.ObservationsRPCPlugin{Impl: provider})
}

func ServeSuggestions(identity string, provider suggestions.Provider) {
	serve(identity, transport.Suggestions, &transport.SuggestionsRPCPlugin{Impl: provider})
}

func serve(identity string, capability transport.Capability, adapter goplugin.Plugin) {
	descriptor := transport.Descriptor{ID: identity, Capability: capability}
	if err := descriptor.Validate(); err != nil {
		panic(err)
	}
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: transport.Handshake,
		GRPCServer:      goplugin.DefaultGRPCServer,
		Plugins: goplugin.PluginSet{
			transport.DescribeKey: &transport.DescribeRPCPlugin{Descriptor: descriptor},
			string(capability):    adapter,
		},
	})
}
