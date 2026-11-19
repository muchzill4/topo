package parameter_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/arm/topo/internal/output/term"
	"github.com/arm/topo/internal/parameter"
	"github.com/arm/topo/pkg/plugin/observations"
	"github.com/arm/topo/pkg/plugin/suggestions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInteractiveResolver(t *testing.T) {
	t.Run("Resolve", func(t *testing.T) {
		t.Run("shows shortcuts and invokes the selected action with the edited value", func(t *testing.T) {
			var prompt term.Prompt
			provider := &actionProvider{}
			resolver := parameter.NewInteractiveResolver(recordPrompt(&prompt), term.NewPalette(false))
			resolver.Providers = []suggestions.Provider{provider}
			resolver.TargetObservations = observations.Tree{"memory": int64(42)}
			parameters := []parameter.Parameter{{
				Name: "MODEL", ExistingValue: new("old"),
				Hints: suggestions.Hints{"model.task": json.RawMessage(`"chat"`)},
			}}
			_, err := resolver.Resolve(parameters)
			require.NoError(t, err)

			got, err := prompt.Actions[1].Run(context.Background(), "edited")

			require.NoError(t, err)
			assert.Equal(t, "selected", got)
			assert.Equal(t, "second", string(provider.actionID))
			assert.Equal(t, suggestions.Request{
				Hints:        suggestions.Hints{"model.task": json.RawMessage(`"chat"`)},
				CurrentValue: new("edited"), TargetObservations: observations.Tree{"memory": int64(42)},
			}, provider.request)
			assert.Equal(t, "   Press Enter to continue · Ctrl+O to Open portal · Ctrl+G to Other action · Ctrl+C to cancel",
				prompt.Hint("value", false))
		})
	})

	t.Run("resolves parameters in order", func(t *testing.T) {
		resolver := parameter.NewInteractiveResolver(promptValues("hello", "8080"), term.NewPalette(false))
		parameters := []parameter.Parameter{{Name: "GREETING", ExistingValue: new("old")}, {Name: "PORT"}}

		got, err := resolver.Resolve(parameters)

		require.NoError(t, err)
		assert.Equal(t, parameter.Changes{"GREETING": new("hello"), "PORT": new("8080")}, got)
	})

	t.Run("shows parameter details", func(t *testing.T) {
		var prompt term.Prompt
		resolver := parameter.NewInteractiveResolver(recordPrompt(&prompt), term.NewPalette(false))
		parameters := []parameter.Parameter{{Name: "GREETING", Description: "The greeting message", Example: "Hello"}}

		_, err := resolver.Resolve(parameters)

		require.NoError(t, err)
		assert.Equal(t, []string{
			"   1/1 GREETING", "", "       The greeting message", `       Example: "Hello"`, "",
		}, prompt.Content)
	})

	t.Run("does not create a change for an empty input on an absent parameter", func(t *testing.T) {
		resolver := parameter.NewInteractiveResolver(promptValues(""), term.NewPalette(false))

		got, err := resolver.Resolve([]parameter.Parameter{{Name: "OPTIONAL"}})

		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("does not create a change for an accepted existing value", func(t *testing.T) {
		resolver := parameter.NewInteractiveResolver(acceptPromptInitial, term.NewPalette(false))
		parameters := []parameter.Parameter{{Name: "GREETING", ExistingValue: new("hello")}}

		got, err := resolver.Resolve(parameters)

		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("retains an existing empty value when accepted", func(t *testing.T) {
		resolver := parameter.NewInteractiveResolver(acceptPromptInitial, term.NewPalette(false))
		parameters := []parameter.Parameter{{Name: "GREETING", ExistingValue: new("")}}

		got, err := resolver.Resolve(parameters)

		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("unsets a cleared nonempty value", func(t *testing.T) {
		resolver := parameter.NewInteractiveResolver(promptValues(""), term.NewPalette(false))
		parameters := []parameter.Parameter{{Name: "GREETING", ExistingValue: new("hello")}}

		got, err := resolver.Resolve(parameters)

		require.NoError(t, err)
		assert.Equal(t, parameter.Changes{"GREETING": nil}, got)
	})

	t.Run("shows reference paths and expressions", func(t *testing.T) {
		var prompt term.Prompt
		resolver := parameter.NewInteractiveResolver(recordPrompt(&prompt), term.NewPalette(false))
		parameters := []parameter.Parameter{{Name: "GREETING", References: []parameter.Reference{
			{Path: "app", Expression: "${GREETING:-default}/${GREETING:-other}"},
		}}}

		_, err := resolver.Resolve(parameters)

		require.NoError(t, err)
		assert.Contains(t, prompt.Content, `         app: "${GREETING:-default}/${GREETING:-other}"`)
	})

	t.Run("allows empty input without required references", func(t *testing.T) {
		var prompt term.Prompt
		resolver := parameter.NewInteractiveResolver(recordPrompt(&prompt), term.NewPalette(false))
		parameters := []parameter.Parameter{{Name: "GREETING", References: []parameter.Reference{
			{Expression: "${GREETING:-hello}/${GREETING:-${OTHER:?required}}"},
		}}}

		_, err := resolver.Resolve(parameters)

		require.NoError(t, err)
		assert.True(t, prompt.Validate(""))
		assert.Equal(t, "   Press Enter to continue · Ctrl+C to cancel", prompt.Hint("", false))
	})

	t.Run("rejects missing required input", func(t *testing.T) {
		var prompt term.Prompt
		resolver := parameter.NewInteractiveResolver(recordPrompt(&prompt), term.NewPalette(false))
		parameters := []parameter.Parameter{{Name: "REQUIRED", References: []parameter.Reference{
			{Expression: "${REQUIRED:?required}"},
		}}}

		_, err := resolver.Resolve(parameters)

		require.NoError(t, err)
		assert.False(t, prompt.Validate(""))
		assert.Equal(t, "   Enter a value to continue · Ctrl+C to cancel", prompt.Hint("", false))
		assert.Equal(t, "   Press Enter to continue · Ctrl+C to cancel", prompt.Hint("value", false))
		assert.Equal(t, "   ✗ REQUIRED is required, enter a value to continue · Ctrl+C to cancel", prompt.Hint("", true))
	})
}

func promptValues(values ...string) func(term.Prompt) (string, error) {
	return func(term.Prompt) (string, error) {
		value := values[0]
		values = values[1:]
		return value, nil
	}
}

func recordPrompt(captured *term.Prompt) func(term.Prompt) (string, error) {
	return func(prompt term.Prompt) (string, error) {
		*captured = prompt
		return acceptPromptInitial(prompt)
	}
}

func acceptPromptInitial(prompt term.Prompt) (string, error) {
	return prompt.Initial, nil
}

type actionProvider struct {
	actionID suggestions.ActionID
	request  suggestions.Request
}

func (*actionProvider) Suggestions(context.Context, suggestions.Request) (suggestions.Response, error) {
	return suggestions.Response{AvailableActions: []suggestions.Action{
		{ID: "first", Label: "Open portal"}, {ID: "second", Label: "Other action"},
	}}, nil
}

func (p *actionProvider) RunAction(
	_ context.Context, id suggestions.ActionID, request suggestions.Request,
) (suggestions.ActionResponse, error) {
	p.actionID, p.request = id, request
	return suggestions.ActionResponse{Suggestions: []suggestions.Suggestion{{Value: "selected"}}}, nil
}
