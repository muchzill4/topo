package main

import (
	"context"
	"fmt"
	"time"

	"github.com/arm/topo/pkg/plugin/suggestions"
)

const chooseModel suggestions.ActionID = "choose_model"

type Provider struct {
	PortalURL string
}

func (Provider) Suggestions(ctx context.Context, _ suggestions.Request) (suggestions.Response, error) {
	if err := ctx.Err(); err != nil {
		return suggestions.Response{}, err
	}
	return suggestions.Response{
		AvailableActions: []suggestions.Action{{
			ID:          chooseModel,
			Label:       "Choose a model in AI Portal",
			Description: "Open a browser filtered by project hints and observed target memory.",
		}},
	}, nil
}

func (p Provider) RunAction(
	ctx context.Context, id suggestions.ActionID, request suggestions.Request,
) (response suggestions.ActionResponse, err error) {
	if err := ctx.Err(); err != nil {
		return suggestions.ActionResponse{}, err
	}
	if id != chooseModel {
		return suggestions.ActionResponse{}, fmt.Errorf("unknown action %q", id)
	}
	// RPC does not forward context deadlines, so bound the handoff inside the plugin too.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	query := BuildModelQuery(request)
	callbackServer, err := NewCallbackServer()
	if err != nil {
		return suggestions.ActionResponse{}, err
	}
	defer func() {
		if closeErr := callbackServer.Close(); closeErr != nil && err == nil {
			response, err = suggestions.ActionResponse{}, closeErr
		}
	}()
	portalClient := NewBrowserClient(p.PortalURL)
	if err := portalClient.OpenModelSelectionPage(ctx, callbackServer.CallbackURL(), query); err != nil {
		return suggestions.ActionResponse{}, err
	}
	selection, err := callbackServer.WaitForCallback(ctx)
	if err != nil {
		return suggestions.ActionResponse{}, fmt.Errorf("Portal handoff ended: %w", err)
	}
	return suggestions.ActionResponse{
		Suggestions: []suggestions.Suggestion{{Value: selection.RepoID, Label: selection.RepoID}},
	}, nil
}
