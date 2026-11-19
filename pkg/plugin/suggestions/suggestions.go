// Package suggestions defines configuration candidates and explicitly selected actions.
package suggestions

import (
	"context"

	"github.com/arm/topo/pkg/plugin/observations"
)

type Provider interface {
	Suggestions(context.Context, Request) (Response, error)
	RunAction(context.Context, ActionID, Request) (ActionResponse, error)
}

// Request concerns one parameter. CurrentValue includes staged changes;
// nil means absent, while a pointer to "" means present but empty.
type Request struct {
	Hints              Hints
	CurrentValue       *string
	TargetObservations observations.Tree
}

type Response struct {
	Suggestions      []Suggestion
	AvailableActions []Action
}

type ActionID string

type Action struct {
	ID          ActionID
	Label       string
	Description string
}

type Suggestion struct {
	Value string
	Label string
}

type ActionResponse struct {
	Suggestions []Suggestion
}
