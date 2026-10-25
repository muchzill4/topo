package main

import (
	"github.com/arm/topo/pkg/plugin"
	"github.com/arm/topo/pkg/plugin/examples/memory/internal/memoryplugin"
	goplugin "github.com/hashicorp/go-plugin"
)

func main() {
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: plugin.Handshake,
		Plugins: map[string]goplugin.Plugin{
			"provider": &plugin.RPCPlugin{Impl: memoryplugin.Provider{}},
		},
	})
}
