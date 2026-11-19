package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/arm/topo/internal/plugins"
	"github.com/arm/topo/internal/runner"
	"github.com/arm/topo/internal/ssh"
	"github.com/arm/topo/pkg/plugin/observations"
	"github.com/arm/topo/pkg/plugin/suggestions"
	"github.com/spf13/cobra"
)

func newRunPluginsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run-plugins",
		Short: "Run installed plugin observations or an explicitly selected action",
		Long: "Discover plugins in --plugin-dir (default: the user configuration directory) and run them in filename order. " +
			"Report plugin failures and continue with successful observations. " +
			"Plugin observations do not affect built-in hardware probing or project matching. " +
			"Discover suggestion actions and offer to run them after confirmation. " +
			"Action results are printed as JSON and do not change project configuration.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := contextWithTimeout(cmd)
			defer cancel()
			ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
			defer stop()

			directory, err := cmd.Flags().GetString("plugin-dir")
			if err != nil {
				return err
			}
			if directory == "" {
				directory, err = plugins.Directory()
				if err != nil {
					return err
				}
			}
			allPlugins, failures, err := plugins.LoadAll(ctx, directory)
			if allPlugins != nil {
				defer allPlugins.Close()
			}
			if err := errors.Join(err, reportPluginFailures(cmd.ErrOrStderr(), failures)); err != nil {
				return err
			}

			reports := make(observations.Tree)
			if target, exists := lookupTarget(cmd); exists {
				observationsPlugins := allPlugins.Observations()
				reports, err = observePluginTargets(
					ctx, cmd.ErrOrStderr(), observationsPlugins, runner.For(ssh.NewDestination(target.value)),
				)
				if err != nil {
					return err
				}
			}
			suggestionsPlugins := allPlugins.Suggestions()
			request := suggestions.Request{
				// Sample parameter hints from topo-examples/topo-image-segmentation.
				Hints: suggestions.Hints{
					"model.task":         json.RawMessage(`"image-segmentation"`),
					"model.format":       json.RawMessage(`"pte"`),
					"executorch.backend": json.RawMessage(`"xnnpack"`),
				},
				TargetObservations: reports,
			}
			if err := offerPluginActions(ctx, cmd, suggestionsPlugins, request); err != nil {
				return err
			}
			if len(reports) == 0 {
				return nil
			}
			encoder := json.NewEncoder(cmd.OutOrStdout())
			encoder.SetIndent("", "  ")
			return encoder.Encode(reports)
		},
	}
	cmd.Flags().String("plugin-dir", "", "plugin discovery directory (default: topo/plugins under the user configuration directory)")
	addTargetFlag(cmd)
	addTimeoutFlag(cmd, 5*time.Minute)
	return cmd
}

func observePluginTargets(
	ctx context.Context, failures io.Writer, plugins []observations.Provider, runner observations.TargetRunner,
) (observations.Tree, error) {
	reports := make(observations.Tree)
	for _, plugin := range plugins {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		result, err := plugin.ObserveTarget(ctx, runner)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err == nil {
			if err := mergeObservationTrees(reports, result); err != nil {
				return nil, fmt.Errorf("merging observations: %w", err)
			}
			continue
		}
		if err := reportPluginFailures(failures, []error{err}); err != nil {
			return nil, err
		}
	}
	return reports, ctx.Err()
}

func mergeObservationTrees(destination, source observations.Tree) error {
	type branch struct {
		destination observations.Tree
		source      observations.Tree
		path        string
	}
	pending := []branch{{destination: destination, source: source}}
	for len(pending) > 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		for key, value := range current.source {
			existing, found := current.destination[key]
			if !found {
				current.destination[key] = value
				continue
			}
			left, leftOK := observationObject(existing)
			right, rightOK := observationObject(value)
			path := current.path + key
			if !leftOK || !rightOK {
				return fmt.Errorf("conflicting observation %q", path)
			}
			pending = append(pending, branch{destination: left, source: right, path: path + "."})
		}
	}
	return nil
}

func observationObject(value any) (observations.Tree, bool) {
	switch value := value.(type) {
	case observations.Tree:
		return value, value != nil
	case map[string]any:
		return observations.Tree(value), value != nil
	default:
		return nil, false
	}
}

func offerPluginActions(
	ctx context.Context, cmd *cobra.Command, plugins []suggestions.Provider, request suggestions.Request,
) error {
	input := bufio.NewReader(cmd.InOrStdin())
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	for _, plugin := range plugins {
		if err := ctx.Err(); err != nil {
			return err
		}
		result, err := plugin.Suggestions(ctx, request)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			if err := reportPluginFailures(cmd.ErrOrStderr(), []error{err}); err != nil {
				return err
			}
			continue
		}
		if len(result.Suggestions) > 0 {
			if err := encoder.Encode(result.Suggestions); err != nil {
				return err
			}
		}
		for _, action := range result.AvailableActions {
			if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "%s\n%s\nRun this action? [y/N] ",
				action.Label, action.Description); err != nil {
				return err
			}
			answer, err := input.ReadString('\n')
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return err
			}
			answer = strings.ToLower(strings.TrimSpace(answer))
			if answer != "y" && answer != "yes" {
				continue
			}
			response, err := plugin.RunAction(ctx, action.ID, request)
			if err != nil {
				return err
			}
			if err := encoder.Encode(response); err != nil {
				return err
			}
		}
	}
	return ctx.Err()
}

func reportPluginFailures(writer io.Writer, failures []error) error {
	for _, failure := range failures {
		if _, err := fmt.Fprintf(writer, "Plugin failure: %v\n", failure); err != nil {
			return fmt.Errorf("reporting plugin failure: %w", err)
		}
	}
	return nil
}

func init() {
	rootCmd.AddCommand(newRunPluginsCommand())
}
