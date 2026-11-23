package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/arm/topo/internal/parameter"
	"github.com/arm/topo/internal/plugins"
	"github.com/spf13/cobra"
)

func addPluginDirectoryFlag(cmd *cobra.Command) {
	cmd.Flags().String("plugin-dir", "", "plugin discovery directory (default: topo/plugins under the user configuration directory)")
}

func pluginDirectory(cmd *cobra.Command) (string, error) {
	directory, err := cmd.Flags().GetString("plugin-dir")
	if err != nil {
		return "", err
	}
	if directory != "" {
		return directory, nil
	}
	return plugins.Directory()
}

// Keep plugin processes scoped to interactive resolution, including all active actions.
type pluginInteractiveResolver struct {
	cmd           *cobra.Command
	output        *os.File
	factsResolver parameter.Resolver
}

func (r pluginInteractiveResolver) Resolve(parameters []parameter.Parameter) (parameter.Changes, error) {
	if len(parameters) == 0 {
		return nil, nil
	}
	directory, err := pluginDirectory(r.cmd)
	if err != nil {
		return nil, err
	}
	collection, failures, err := plugins.LoadAll(r.cmd.Context(), directory)
	if collection != nil {
		defer collection.Close()
	}
	if err := errors.Join(err, reportPluginFailures(r.output, failures)); err != nil {
		return nil, err
	}
	interactive := parameter.NewInteractiveResolverForTerminal(os.Stdin, r.output, r.factsResolver)
	interactive.Context = r.cmd.Context()
	interactive.Providers = collection.Suggestions()
	return interactive.Resolve(parameters)
}

func reportPluginFailures(writer io.Writer, failures []error) error {
	for _, failure := range failures {
		if _, err := fmt.Fprintf(writer, "Plugin failure: %v\n", failure); err != nil {
			return fmt.Errorf("reporting plugin failure: %w", err)
		}
	}
	return nil
}
