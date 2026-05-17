package cli

import (
	"fmt"
	"log/slog"

	"github.com/kobus-v-schoor/dcx/internal/docker"
	"github.com/spf13/cobra"
)

// newDownCmd creates the "down" subcommand. It finds the devcontainer for the
// current project, stops and removes it, and cleans up the associated image.
// Added to the root command tree in Execute().
func newDownCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "down",
		Short: "Stop, remove, and clean up the devcontainer for this project",
		Long:  "Finds the devcontainer for the current project by matching the devcontainer.local_folder label, then stops and removes the container and its image.\nUse --log-level info to see what is being stopped and removed.",
		RunE:  runDown,
	}
}

// runDown implements the dcx down workflow. Called by Cobra when the user
// runs "dcx down". Config, log level, and Docker daemon reachability are
// already verified by the root command's PersistentPreRunE.
func runDown(cmd *cobra.Command, args []string) error {
	slog.Info("workspace-folder", "path", workspaceFolder)

	cli, err := docker.NewClient(cmd.Context())
	if err != nil {
		return err
	}
	defer func() { _ = cli.Close() }()

	if err := docker.Down(cmd.Context(), cli, workspaceFolder); err != nil {
		return fmt.Errorf("dcx down: %w", err)
	}

	return nil
}
