package cli

import (
	"fmt"
	"log/slog"

	"github.com/kobus-v-schoor/dcx/internal/docker"
	"github.com/spf13/cobra"
)

// newStopCmd creates the "stop" subcommand. It finds the devcontainer for the
// current project and stops it without removing it. Added to the root command
// tree in Execute().
func newStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the devcontainer for this project without removing it",
		Long:  "Finds the devcontainer for the current project by matching the devcontainer.local_folder label and stops it.\nUse --log-level info to see what is being stopped.",
		RunE:  runStop,
	}
}

// runStop implements the dcx stop workflow. Called by Cobra when the user
// runs "dcx stop". Config and log level are already initialised by the
// root command's PersistentPreRunE.
func runStop(cmd *cobra.Command, args []string) error {
	slog.Info("workspace-folder", "path", workspaceFolder)

	cli, err := docker.NewClient()
	if err != nil {
		return err
	}
	defer cli.Close()

	if err := docker.CheckDaemon(cmd.Context(), cli); err != nil {
		return err
	}

	slog.Info("Docker daemon reachable")

	if err := docker.Stop(cmd.Context(), cli, workspaceFolder); err != nil {
		return fmt.Errorf("dcx stop: %w", err)
	}

	return nil
}
