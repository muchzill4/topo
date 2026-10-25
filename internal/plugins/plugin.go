package plugins

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/arm/topo/pkg/plugin"
	goplugin "github.com/hashicorp/go-plugin"
)

// Plugin owns one local plugin process and its metadata.
type Plugin struct {
	path       string
	descriptor plugin.Descriptor
	remote     plugin.Plugin
	client     *goplugin.Client
}

// Load starts one executable and validates its descriptor without probing.
func Load(ctx context.Context, path string) (result *Plugin, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig: plugin.Handshake,
		Plugins: map[string]goplugin.Plugin{
			"provider": &plugin.RPCPlugin{},
		},
		Cmd:              exec.CommandContext(ctx, path),
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolNetRPC},
		StartTimeout:     10 * time.Second,
	})
	defer func() {
		if err != nil {
			client.Kill()
		}
	}()
	remote, err := start(client)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("starting plugin %q: %w", path, err)
	}

	descriptor, err := remote.Describe(ctx)
	if err != nil {
		return nil, fmt.Errorf("describing plugin %q: %w", path, err)
	}
	if descriptor.Name == "" {
		return nil, fmt.Errorf("plugin %q has an empty name", path)
	}
	return &Plugin{path: path, descriptor: descriptor, remote: remote, client: client}, nil
}

func (p *Plugin) Close() {
	p.client.Kill()
}

func start(client *goplugin.Client) (plugin.Plugin, error) {
	connection, err := client.Client()
	if err != nil {
		return nil, err
	}

	raw, err := connection.Dispense("provider")
	if err != nil {
		return nil, err
	}
	remote, ok := raw.(plugin.Plugin)
	if !ok {
		return nil, fmt.Errorf("unexpected plugin interface")
	}
	return remote, nil
}
