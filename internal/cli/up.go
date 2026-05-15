package cli

import (
	"fmt"

	"github.com/kobus-v-schoor/dcx/internal/config"
	"github.com/kobus-v-schoor/dcx/internal/flags"
	"github.com/kobus-v-schoor/dcx/internal/logging"
	"github.com/kobus-v-schoor/dcx/internal/override"
	"github.com/kobus-v-schoor/dcx/internal/runner"
	"github.com/spf13/cobra"
)

// newUpCmd creates the "up" subcommand. It loads config, creates the override
// directory, assembles devcontainer CLI flags, and delegates execution.
// Added to the root command tree in Execute().
func newUpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "up",
		Short: "Start a devcontainer using dcx configuration",
		Long:  "Start a devcontainer by delegating to the devcontainer CLI with dcx-assembled flags.\nAny flags after -- are passed through to devcontainer up unchanged.",
		Args:  cobra.ArbitraryArgs,
		RunE:  runUp,
	}
}

// runUp implements the dcx up workflow. Called by Cobra when the user
// runs "dcx up".
func runUp(cmd *cobra.Command, args []string) error {
	if err := logging.SetLevel(logLevel); err != nil {
		return fmt.Errorf("invalid log level %q: %w", logLevel, err)
	}

	log := logging.L()

	log.Infof("workspace-folder = %s", workspaceFolder)

	devcontainerPath, err := runner.Find()
	if err != nil {
		return err
	}
	log.Infof("found devcontainer CLI at %s", devcontainerPath)

	cfg, err := config.Load(workspaceFolder)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	log.Info("config loaded")
	if cfg.SSHForwarding != nil {
		log.Debugf("ssh_forwarding = %v", *cfg.SSHForwarding)
	}
	if cfg.GitConfigForwarding != nil {
		log.Debugf("git_config_forwarding = %v", *cfg.GitConfigForwarding)
	}

	overrideDir, cleanup, err := override.Create(workspaceFolder)
	if err != nil {
		return fmt.Errorf("creating override config: %w", err)
	}
	defer cleanup()

	log.Infof("override dir = %s", overrideDir)

	dcArgs := flags.Build(workspaceFolder, cfg, overrideDir)

	dcArgs = append(dcArgs, args...)

	log.Debugf("invoking devcontainer with args: %v", dcArgs)

	return runner.Run(devcontainerPath, dcArgs)
}
