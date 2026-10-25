package plugin

import (
	"context"
	"errors"
	"fmt"
	"net/rpc"
)

func (p *pluginServer) Facts(callbackID uint32, reply *FactsResult) (err error) {
	provider, ok := p.impl.(FactsProviderPlugin)
	if !ok {
		return fmt.Errorf("plugin does not implement %s", FactsContract)
	}
	connection, err := p.broker.Dial(callbackID)
	if err != nil {
		return err
	}
	client := rpc.NewClient(connection)
	defer func() { err = errors.Join(err, client.Close()) }()
	result, err := provider.Facts(context.Background(), &runnerClient{client: client})
	*reply = result
	return err
}
