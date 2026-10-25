// Package plugin defines the experimental Go-only Topo plugin protocol.
package plugin

import (
	"context"
	"net/rpc"

	goplugin "github.com/hashicorp/go-plugin"
)

// The magic cookie identifies the plugin family; it is not authentication.
var Handshake = goplugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "TOPO_PLUGIN",
	MagicCookieValue: "topo-plugin-v1",
}

type Descriptor struct {
	Name      string
	Contracts []string
}

// Plugin describes its identity and supported contracts without probing.
type Plugin interface {
	Describe(context.Context) (Descriptor, error)
}

// RPCPlugin implements go-plugin's net/rpc adapters. Impl is set only by the server.
type RPCPlugin struct {
	Impl Plugin
}

func (p *RPCPlugin) Server(broker *goplugin.MuxBroker) (interface{}, error) {
	return &pluginServer{impl: p.Impl, broker: broker}, nil
}

func (p *RPCPlugin) Client(broker *goplugin.MuxBroker, client *rpc.Client) (interface{}, error) {
	return &pluginClient{client: client, broker: broker}, nil
}
