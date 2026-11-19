package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/rpc"

	"github.com/arm/topo/pkg/plugin/suggestions"
	goplugin "github.com/hashicorp/go-plugin"
)

// SuggestionsRPCPlugin carries the suggestions contract independently of observations.
type SuggestionsRPCPlugin struct {
	Impl suggestions.Provider
}

func (p *SuggestionsRPCPlugin) Server(_ *goplugin.MuxBroker) (any, error) {
	return &suggestionsServer{impl: p.Impl}, nil
}

func (p *SuggestionsRPCPlugin) Client(_ *goplugin.MuxBroker, client *rpc.Client) (any, error) {
	return &suggestionsClient{client: client}, nil
}

// SuggestionRequest preserves present-but-empty values across gob, which omits zero fields.
type SuggestionRequest struct {
	Hints              suggestions.Hints
	HasCurrentValue    bool
	CurrentValue       string
	TargetObservations []byte
}

func newSuggestionRequest(request suggestions.Request) (SuggestionRequest, error) {
	encoded, err := json.Marshal(request.TargetObservations)
	if err != nil {
		return SuggestionRequest{}, fmt.Errorf("encoding target observations: %w", err)
	}
	wire := SuggestionRequest{
		Hints:              request.Hints,
		TargetObservations: encoded,
		HasCurrentValue:    request.CurrentValue != nil,
	}
	if request.CurrentValue != nil {
		wire.CurrentValue = *request.CurrentValue
	}
	return wire, nil
}

func (r SuggestionRequest) request() (suggestions.Request, error) {
	request := suggestions.Request{Hints: r.Hints}
	if err := json.Unmarshal(r.TargetObservations, &request.TargetObservations); err != nil {
		return suggestions.Request{}, fmt.Errorf("decoding target observations: %w", err)
	}
	if r.HasCurrentValue {
		request.CurrentValue = &r.CurrentValue
	}
	return request, nil
}

type SuggestionActionRequest struct {
	ID      suggestions.ActionID
	Request SuggestionRequest
}

type suggestionsClient struct {
	client *rpc.Client
}

func (p *suggestionsClient) Suggestions(
	ctx context.Context, request suggestions.Request,
) (suggestions.Response, error) {
	var result suggestions.Response
	args, err := newSuggestionRequest(request)
	if err != nil {
		return result, err
	}
	err = call(ctx, p.client, "Plugin.Suggestions", args, &result)
	return result, err
}

func (p *suggestionsClient) RunAction(
	ctx context.Context, id suggestions.ActionID, request suggestions.Request,
) (suggestions.ActionResponse, error) {
	var result suggestions.ActionResponse
	wire, err := newSuggestionRequest(request)
	if err != nil {
		return result, err
	}
	args := SuggestionActionRequest{ID: id, Request: wire}
	err = call(ctx, p.client, "Plugin.RunAction", args, &result)
	return result, err
}

type suggestionsServer struct {
	impl suggestions.Provider
}

func (p *suggestionsServer) Suggestions(request SuggestionRequest, reply *suggestions.Response) error {
	decoded, err := request.request()
	if err != nil {
		return err
	}
	result, err := p.impl.Suggestions(context.Background(), decoded)
	*reply = result
	return err
}

func (p *suggestionsServer) RunAction(
	args SuggestionActionRequest, reply *suggestions.ActionResponse,
) error {
	request, err := args.Request.request()
	if err != nil {
		return err
	}
	result, err := p.impl.RunAction(context.Background(), args.ID, request)
	*reply = result
	return err
}
