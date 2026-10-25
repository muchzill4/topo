package plugin

import (
	"context"

	goplugin "github.com/hashicorp/go-plugin"
)

type pluginServer struct {
	impl   Plugin
	broker *goplugin.MuxBroker
}

func (p *pluginServer) Describe(_ struct{}, reply *Descriptor) error {
	result, err := p.impl.Describe(context.Background())
	*reply = result
	return err
}
