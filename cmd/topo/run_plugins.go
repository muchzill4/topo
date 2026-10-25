package main

import (
	"encoding/json"

	"github.com/arm/topo/internal/plugins"
	"github.com/arm/topo/internal/runner"
	"github.com/arm/topo/internal/ssh"
	"github.com/spf13/cobra"
)

func newRunPluginsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run-plugins",
		Short: "Run installed plugins and present target facts grouped by provider",
		Long: "Discover plugins in --plugin-dir (default: the user configuration directory) and run them in filename order. " +
			"Stop on the first error. Plugin facts do not affect built-in hardware probing or project matching.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := requireTarget(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := contextWithTimeout(cmd)
			defer cancel()

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
			providers, err := plugins.LoadAll(ctx, directory)
			if err != nil {
				return err
			}
			defer providers.Close()

			reports, err := providers.Facts(runner.For(ssh.NewDestination(target.value)))
			if err != nil {
				return err
			}
			encoder := json.NewEncoder(cmd.OutOrStdout())
			encoder.SetIndent("", "  ")
			return encoder.Encode(reports)
		},
	}
	cmd.Flags().String("plugin-dir", "", "plugin discovery directory (default: topo/plugins under the user configuration directory)")
	addTargetFlag(cmd)
	addTimeoutFlag(cmd, defaultTimeout)
	return cmd
}

func init() {
	rootCmd.AddCommand(newRunPluginsCommand())
}
