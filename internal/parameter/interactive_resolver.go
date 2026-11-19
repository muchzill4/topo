package parameter

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/arm/topo/internal/output/term"
	"github.com/arm/topo/pkg/plugin/observations"
	"github.com/arm/topo/pkg/plugin/suggestions"
)

// InteractiveResolver resolves parameter definitions to changes by prompting for values.
type InteractiveResolver struct {
	Context            context.Context
	Providers          []suggestions.Provider
	TargetObservations observations.Tree
	readPrompt         func(term.Prompt) (string, error)
	palette            term.Palette
}

func NewInteractiveResolverForTerminal(in *os.File, out *os.File) *InteractiveResolver {
	readPrompt := func(prompt term.Prompt) (string, error) {
		return term.ReadPrompt(in, out, prompt)
	}
	return NewInteractiveResolver(readPrompt, term.NewPaletteFor(out))
}

func NewInteractiveResolver(readPrompt func(term.Prompt) (string, error), palette term.Palette) *InteractiveResolver {
	return &InteractiveResolver{readPrompt: readPrompt, palette: palette}
}

func (r *InteractiveResolver) Resolve(parameters []Parameter) (Changes, error) {
	changes := Changes{}

	for i, parameter := range parameters {
		initial := ""
		if parameter.ExistingValue != nil {
			initial = *parameter.ExistingValue
		}

		actions, labels, err := r.parameterActions(parameter)
		if err != nil {
			return nil, err
		}
		value, err := r.readPrompt(term.Prompt{
			Actions: actions,
			Validate: func(input string) bool {
				return parameter.AssertSatisfiedBy(contentfulStringOrNil(input)) == nil
			},
			Content: formatParameterPromptContent(parameter, i+1, len(parameters), r.palette),
			Hint: func(input string, invalid bool) string {
				return formatParameterPromptHint(parameter, input, invalid, labels, r.palette)
			},
			Initial: initial,
			Prefix:  "   " + r.palette.Color(term.Magenta, ">") + " ",
		})
		if err != nil {
			return nil, err
		}

		if value == "" && parameter.ExistingValue == nil {
			continue
		}
		if parameter.ExistingValue != nil && value == *parameter.ExistingValue {
			continue
		}
		changes[parameter.Name] = contentfulStringOrNil(value)
	}
	return changes, nil
}

func formatParameterPromptContent(parameter Parameter, number, total int, palette term.Palette) []string {
	progress := palette.Color(term.Dim, fmt.Sprintf("%d/%d", number, total))
	lines := []string{fmt.Sprintf("   %s %s", progress, parameter.Name), ""}
	if description := strings.TrimSpace(parameter.Description); description != "" {
		for line := range strings.SplitSeq(description, "\n") {
			lines = append(lines, fmt.Sprintf("       %s", line))
		}
	}
	if example := strings.TrimSpace(parameter.Example); example != "" {
		lines = append(lines, fmt.Sprintf("       %s %q", palette.Color(term.Dim, "Example:"), example))
	}
	if len(parameter.References) > 0 {
		lines = append(lines, "       "+palette.Color(term.Dim, "References:"))
		for _, reference := range parameter.References {
			pathLabel := palette.Color(term.Magenta, reference.Path+":")
			lines = append(lines, fmt.Sprintf("         %s %q", pathLabel, reference.Expression))
		}
	}
	if len(lines) > 2 {
		lines = append(lines, "")
	}
	if number > 1 {
		lines = append([]string{""}, lines...)
	}
	return lines
}

func formatParameterPromptHint(
	parameter Parameter, input string, invalid bool, actionHints string, palette term.Palette,
) string {
	message := "Enter a value to continue"
	if input != "" || parameter.AssertSatisfiedBy(nil) == nil {
		message = "Press Enter to continue"
	}
	message = palette.Color(term.Dim, message)

	if invalid {
		message = fmt.Sprintf("%s %s is required, enter a value to continue", palette.Color(term.Red, "✗"), parameter.Name)
	}
	return "   " + message + actionHints + palette.Color(term.Dim, " · Ctrl+C to cancel")
}

func contentfulStringOrNil(input string) *string {
	if input == "" {
		return nil
	}
	return &input
}
