package plugins

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/arm/topo/pkg/plugin/observations"
	"github.com/arm/topo/pkg/plugin/suggestions"
	"github.com/arm/topo/pkg/plugin/transport"
	goplugin "github.com/hashicorp/go-plugin"
)

type process struct {
	path         string
	descriptor   transport.Descriptor
	client       *goplugin.Client
	observations observations.Provider
	suggestions  suggestions.Provider
}

func load(ctx context.Context, path string) (_ *process, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig: transport.Handshake,
		Plugins: goplugin.PluginSet{
			transport.DescribeKey:          &transport.DescribeRPCPlugin{},
			string(transport.Observations): &transport.ObservationsRPCPlugin{},
			string(transport.Suggestions):  &transport.SuggestionsRPCPlugin{},
		},
		Cmd:              exec.CommandContext(ctx, path),
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		StartTimeout:     10 * time.Second,
		// The brokered target runner must only accept calls from this plugin process.
		AutoMTLS: true,
	})
	defer func() {
		if err != nil {
			client.Kill()
		}
	}()
	connection, err := client.Client()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("starting plugin %q: %w", path, err)
	}

	raw, err := connection.Dispense(transport.DescribeKey)
	if err != nil {
		return nil, fmt.Errorf("obtaining describe adapter from %q: %w", path, err)
	}
	descriptor, err := raw.(*transport.DescribeClient).Describe(ctx)
	if err != nil {
		return nil, fmt.Errorf("describing plugin %q: %w", path, err)
	}
	if err := descriptor.Validate(); err != nil {
		return nil, fmt.Errorf("invalid descriptor from plugin %q: %w", path, err)
	}
	loaded := &process{path: path, descriptor: descriptor, client: client}
	if err := loaded.dispense(connection); err != nil {
		return nil, err
	}
	return loaded, nil
}

func (p *process) dispense(connection goplugin.ClientProtocol) error {
	capability := p.descriptor.Capability
	if capability != transport.Observations && capability != transport.Suggestions {
		return nil
	}
	raw, err := connection.Dispense(string(capability))
	if err != nil {
		return fmt.Errorf("plugin %q advertised %s but its adapter failed: %w", p.path, capability, err)
	}
	switch capability {
	case transport.Observations:
		p.observations = raw.(observations.Provider)
	case transport.Suggestions:
		p.suggestions = raw.(suggestions.Provider)
	}
	return nil
}

func (p *process) Close() {
	p.client.Kill()
}

func (p *process) ObserveTarget(ctx context.Context, runner observations.TargetRunner) (observations.Tree, error) {
	result, err := p.observations.ObserveTarget(ctx, runner)
	if err != nil {
		return nil, fmt.Errorf("observations plugin %q: %w", p.path, err)
	}
	return result, nil
}

func (p *process) Suggestions(ctx context.Context, request suggestions.Request) (suggestions.Response, error) {
	result, err := p.suggestions.Suggestions(ctx, request)
	if err != nil {
		return suggestions.Response{}, fmt.Errorf("suggestions plugin %q: %w", p.path, err)
	}
	return result, nil
}

func (p *process) RunAction(
	ctx context.Context, id suggestions.ActionID, request suggestions.Request,
) (suggestions.ActionResponse, error) {
	result, err := p.suggestions.RunAction(ctx, id, request)
	if err != nil {
		return suggestions.ActionResponse{}, fmt.Errorf("plugin %q action %q: %w", p.path, id, err)
	}
	return result, nil
}
