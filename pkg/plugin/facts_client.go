package plugin

import "context"

func (p *pluginClient) Facts(ctx context.Context, runner TargetRunner) (FactsResult, error) {
	var result FactsResult
	if err := ctx.Err(); err != nil {
		return result, err
	}
	callbackID := p.broker.NextId()
	go p.broker.AcceptAndServe(callbackID, &runnerServer{ctx: ctx, impl: runner})
	err := call(ctx, p.client, "Plugin.Facts", callbackID, &result)
	return result, err
}
