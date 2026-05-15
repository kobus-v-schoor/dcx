package cli

import (
	"fmt"
	"os"

	"github.com/kobus-v-schoor/dcx/internal/config"
	"github.com/kobus-v-schoor/dcx/internal/flags"
	"github.com/kobus-v-schoor/dcx/internal/override"
	"github.com/kobus-v-schoor/dcx/internal/runner"
	"github.com/spf13/cobra"
)

// newUpCmd creates the "up" subcommand. It loads config, creates the override
// directory, assembles devcontainer CLI flags, and delegates execution.
// Scope: CLI subcommand. Added to the root command tree in Execute().
func newUpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "up",
		Short: "Start a devcontainer using dcx configuration",
		Long:  "Start a devcontainer by delegating to the devcontainer CLI with dcx-assembled flags.\nAny flags after -- are passed through to devcontainer up unchanged.",
		Args:  cobra.ArbitraryArgs,
		RunE:  runUp,
	}
}

// runUp implements the dcx up workflow. Scope: command handler. Called by
// Cobra when the user runs "dcx up".
func runUp(cmd *cobra.Command, args []string) error {
	if verbose {
		fmt.Fprintf(os.Stderr, "dcx: workspace-folder = %s\n", workspaceFolder)
	}

	devcontainerPath, err := runner.Find()
	if err != nil {
		return err
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "dcx: found devcontainer CLI at %s\n", devcontainerPath)
	}

	cfg, err := config.Load(workspaceFolder)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "dcx: config loaded\n")
		if cfg.SSHForwarding != nil {
			fmt.Fprintf(os.Stderr, "dcx:   ssh_forwarding = %v\n", *cfg.SSHForwarding)
		}
		if cfg.GitConfigForwarding != nil {
			fmt.Fprintf(os.Stderr, "dcx:   git_config_forwarding = %v\n", *cfg.GitConfigForwarding)
		}
	}

	overrideDir, cleanup, err := override.Create(workspaceFolder)
	if err != nil {
		return fmt.Errorf("creating override config: %w", err)
	}
	defer cleanup()

	if verbose {
		fmt.Fprintf(os.Stderr, "dcx: override dir = %s\n", overrideDir)
	}

	dcArgs := flags.Build(workspaceFolder, cfg, overrideDir)

	dcArgs = append(dcArgs, args...)

	if verbose {
		fmt.Fprintf(os.Stderr, "dcx: invoking devcontainer with args: %v\n", dcArgs)
	}

	return runner.Run(devcontainerPath, dcArgs)
}
