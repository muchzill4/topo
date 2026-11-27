package transport

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/arm/topo/pkg/plugin/suggestions"
	"github.com/arm/topo/pkg/plugin/transport/pluginpb"
	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

type SuggestionsRPCPlugin struct {
	goplugin.NetRPCUnsupportedPlugin
	Impl suggestions.Provider
}

func (p *SuggestionsRPCPlugin) GRPCServer(_ *goplugin.GRPCBroker, server *grpc.Server) error {
	pluginpb.RegisterSuggestionsServiceServer(server, &suggestionsServer{impl: p.Impl})
	return nil
}

func (p *SuggestionsRPCPlugin) GRPCClient(
	_ context.Context, _ *goplugin.GRPCBroker, conn *grpc.ClientConn,
) (any, error) {
	return &suggestionsClient{rpc: pluginpb.NewSuggestionsServiceClient(conn)}, nil
}

type suggestionsClient struct {
	rpc pluginpb.SuggestionsServiceClient
}

func (p *suggestionsClient) Suggestions(
	ctx context.Context, request suggestions.Request,
) (suggestions.Response, error) {
	wire, err := serialiseSuggestionRequest(request)
	if err != nil {
		return suggestions.Response{}, err
	}
	reply, err := p.rpc.Suggestions(ctx, wire)
	if err != nil {
		return suggestions.Response{}, callError(ctx, err)
	}
	result := suggestions.Response{Suggestions: deserialiseSuggestions(reply.GetSuggestions())}
	for _, action := range reply.GetAvailableActions() {
		result.AvailableActions = append(result.AvailableActions, suggestions.Action{
			ID: suggestions.ActionID(action.GetId()), Label: action.GetLabel(), Description: action.GetDescription(),
		})
	}
	return result, nil
}

func (p *suggestionsClient) RunAction(
	ctx context.Context, id suggestions.ActionID, request suggestions.Request,
) (suggestions.ActionResponse, error) {
	wire, err := serialiseSuggestionRequest(request)
	if err != nil {
		return suggestions.ActionResponse{}, err
	}
	reply, err := p.rpc.RunAction(ctx, &pluginpb.ActionRequest{Id: string(id), Request: wire})
	if err != nil {
		return suggestions.ActionResponse{}, callError(ctx, err)
	}
	return suggestions.ActionResponse{Suggestions: deserialiseSuggestions(reply.GetSuggestions())}, nil
}

type suggestionsServer struct {
	pluginpb.UnimplementedSuggestionsServiceServer
	impl suggestions.Provider
}

func (p *suggestionsServer) Suggestions(
	ctx context.Context, wire *pluginpb.SuggestionRequest,
) (*pluginpb.SuggestionResponse, error) {
	request, err := deserialiseSuggestionRequest(wire)
	if err != nil {
		return nil, err
	}
	result, err := p.impl.Suggestions(ctx, request)
	if err != nil {
		return nil, err
	}
	reply := &pluginpb.SuggestionResponse{Suggestions: serialiseSuggestions(result.Suggestions)}
	for _, action := range result.AvailableActions {
		reply.AvailableActions = append(reply.AvailableActions, &pluginpb.Action{
			Id: string(action.ID), Label: action.Label, Description: action.Description,
		})
	}
	return reply, nil
}

func (p *suggestionsServer) RunAction(
	ctx context.Context, wire *pluginpb.ActionRequest,
) (*pluginpb.ActionResponse, error) {
	request, err := deserialiseSuggestionRequest(wire.GetRequest())
	if err != nil {
		return nil, err
	}
	result, err := p.impl.RunAction(ctx, suggestions.ActionID(wire.GetId()), request)
	if err != nil {
		return nil, err
	}
	return &pluginpb.ActionResponse{Suggestions: serialiseSuggestions(result.Suggestions)}, nil
}

func serialiseSuggestionRequest(request suggestions.Request) (*pluginpb.SuggestionRequest, error) {
	tree, err := json.Marshal(request.TargetObservations)
	if err != nil {
		return nil, fmt.Errorf("serialising target observations: %w", err)
	}
	var hints map[string][]byte
	if request.Hints != nil {
		hints = make(map[string][]byte, len(request.Hints))
		for key, value := range request.Hints {
			hints[key] = value
		}
	}
	return &pluginpb.SuggestionRequest{
		ProjectName: request.Project.Name, Hints: hints, CurrentValue: request.CurrentValue, TargetObservations: tree,
	}, nil
}

func deserialiseSuggestionRequest(wire *pluginpb.SuggestionRequest) (suggestions.Request, error) {
	tree, err := deserialiseObservations(wire.GetTargetObservations())
	if err != nil {
		return suggestions.Request{}, err
	}
	var hints suggestions.Hints
	if len(wire.GetHints()) > 0 {
		hints = make(suggestions.Hints, len(wire.GetHints()))
		for key, value := range wire.GetHints() {
			hints[key] = value
		}
	}
	return suggestions.Request{
		Project: suggestions.ProjectMetadata{Name: wire.GetProjectName()},
		Hints:   hints, CurrentValue: wire.CurrentValue, TargetObservations: tree,
	}, nil
}

func serialiseSuggestions(values []suggestions.Suggestion) []*pluginpb.Suggestion {
	var wire []*pluginpb.Suggestion
	for _, value := range values {
		wire = append(wire, &pluginpb.Suggestion{Value: value.Value, Label: value.Label})
	}
	return wire
}

func deserialiseSuggestions(wire []*pluginpb.Suggestion) []suggestions.Suggestion {
	var values []suggestions.Suggestion
	for _, value := range wire {
		values = append(values, suggestions.Suggestion{Value: value.GetValue(), Label: value.GetLabel()})
	}
	return values
}
