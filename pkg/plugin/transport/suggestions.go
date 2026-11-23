package transport

import (
	"context"
	"fmt"
	"net/rpc"
	"sync"
	"time"

	"github.com/arm/topo/pkg/plugin/suggestions"
	goplugin "github.com/hashicorp/go-plugin"
)

// SuggestionsRPCPlugin carries the suggestions contract over RPC.
type SuggestionsRPCPlugin struct {
	Impl suggestions.Provider
}

func (p *SuggestionsRPCPlugin) Server(_ *goplugin.MuxBroker) (any, error) {
	return &suggestionsServer{impl: p.Impl, actions: make(map[uint64]actionContext)}, nil
}

func (p *SuggestionsRPCPlugin) Client(_ *goplugin.MuxBroker, client *rpc.Client) (any, error) {
	return &suggestionsClient{client: client}, nil
}

// SuggestionRequest preserves present-but-empty values across gob, which omits zero fields.
type SuggestionRequest struct {
	Project         suggestions.ProjectMetadata
	Hints           suggestions.Hints
	HasCurrentValue bool
	CurrentValue    string
}

func newSuggestionRequest(request suggestions.Request) SuggestionRequest {
	wire := SuggestionRequest{
		Project:         request.Project,
		Hints:           request.Hints,
		HasCurrentValue: request.CurrentValue != nil,
	}
	if request.CurrentValue != nil {
		wire.CurrentValue = *request.CurrentValue
	}
	return wire
}

func (r SuggestionRequest) request() suggestions.Request {
	request := suggestions.Request{Project: r.Project, Hints: r.Hints}
	if r.HasCurrentValue {
		request.CurrentValue = &r.CurrentValue
	}
	return request
}

type SuggestionActionRequest struct {
	CallID  uint64
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
	err := call(ctx, p.client, "Plugin.Suggestions", newSuggestionRequest(request), &result)
	return result, err
}

func (p *suggestionsClient) RunAction(
	ctx context.Context, id suggestions.ActionID, request suggestions.Request,
) (suggestions.ActionResponse, error) {
	var result suggestions.ActionResponse
	if err := ctx.Err(); err != nil {
		return result, err
	}
	var callID uint64
	// Registration must finish even if the prompt closes immediately. Otherwise
	// call would close the shared connection instead of cancelling just this action.
	registration, cancelRegistration := context.WithTimeout(context.Background(), 2*time.Second)
	err := call(registration, p.client, "Plugin.BeginAction", struct{}{}, &callID)
	cancelRegistration()
	if err != nil {
		return result, err
	}
	args := SuggestionActionRequest{CallID: callID, ID: id, Request: newSuggestionRequest(request)}
	pending := p.client.Go("Plugin.RunAction", args, &result, make(chan *rpc.Call, 1))
	select {
	case completed := <-pending.Done:
		return result, completed.Error
	case <-ctx.Done():
		// Keep the connection usable for later parameters, and wait for plugin cleanup.
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		var ignored struct{}
		cancelErr := call(cleanup, p.client, "Plugin.CancelAction", callID, &ignored)
		select {
		case <-pending.Done:
		case <-cleanup.Done():
			_ = p.client.Close()
			<-pending.Done
		}
		if cancelErr != nil {
			return suggestions.ActionResponse{}, fmt.Errorf("cancel action: %w", cancelErr)
		}
		return suggestions.ActionResponse{}, ctx.Err()
	}
}

type suggestionsServer struct {
	impl    suggestions.Provider
	mu      sync.Mutex
	nextID  uint64
	actions map[uint64]actionContext
}

type actionContext struct {
	context.Context
	cancel context.CancelFunc
}

// BeginAction registers cancellation before execution, avoiding a cancel/start race.
func (p *suggestionsServer) BeginAction(_ struct{}, reply *uint64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.nextID++
	ctx, cancel := context.WithCancel(context.Background())
	p.actions[p.nextID] = actionContext{Context: ctx, cancel: cancel}
	*reply = p.nextID
	return nil
}

func (p *suggestionsServer) CancelAction(id uint64, _ *struct{}) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if action, found := p.actions[id]; found {
		action.cancel()
	}
	return nil
}

func (p *suggestionsServer) Suggestions(request SuggestionRequest, reply *suggestions.Response) error {
	result, err := p.impl.Suggestions(context.Background(), request.request())
	*reply = result
	return err
}

func (p *suggestionsServer) RunAction(
	args SuggestionActionRequest, reply *suggestions.ActionResponse,
) error {
	p.mu.Lock()
	ctx, found := p.actions[args.CallID]
	p.mu.Unlock()
	if !found {
		return fmt.Errorf("unknown action call %d", args.CallID)
	}
	defer func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.actions[args.CallID].cancel()
		delete(p.actions, args.CallID)
	}()
	result, err := p.impl.RunAction(ctx, args.ID, args.Request.request())
	*reply = result
	return err
}
