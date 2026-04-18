package main

import (
	"os"

	"github.com/arm/topo/internal/deploy"
	"github.com/arm/topo/internal/deploy/engine"
	"github.com/arm/topo/internal/ssh"

	"github.com/spf13/cobra"
)

var stopEngineFlag string

var topoStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop a currently running deployment",
	Long: `Stop services that are already running on the target host using definitions in the compose file.

Executing this command does not remove the containers.

The compose file (compose.yaml) must be in the current working directory, as this is used to select the containers to be stopped.`,
	Args: cobra.ExactArgs(0),
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true

		targetArg, err := requireTarget(cmd)
		if err != nil {
			return err
		}

		composeFile, err := getComposeFileName()
		if err != nil {
			return err
		}

		engine, err := engine.ParseEngine(stopEngineFlag)
		if err != nil {
			return err
		}

		dest := ssh.NewDestination(targetArg)

		stop := deploy.NewDeploymentStop(engine, composeFile, dest)

		return stop.Run(os.Stdout)
	},
}

func init() {
	addTargetFlag(topoStopCmd)
	topoStopCmd.Flags().StringVar(&stopEngineFlag, "engine", "docker", "Container engine on the target (docker, podman, nerdctl, finch)")
	rootCmd.AddCommand(topoStopCmd)
}
