package cli

import (
	"fmt"
	"log/slog"

	"github.com/kobus-v-schoor/dcx/internal/config"
	"github.com/kobus-v-schoor/dcx/internal/flags"
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
	level, err := parseLogLevel(logLevel)
	if err != nil {
		return fmt.Errorf("invalid log level %q: %w", logLevel, err)
	}
	slog.SetLogLoggerLevel(level)

	slog.Info("workspace-folder", "path", workspaceFolder)

	devcontainerPath, err := runner.Find()
	if err != nil {
		return err
	}
	slog.Info("found devcontainer CLI", "path", devcontainerPath)

	cfg, err := config.Load(workspaceFolder)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	slog.Info("config loaded")
	if cfg.SSHForwarding != nil {
		slog.Debug("ssh_forwarding", "enabled", *cfg.SSHForwarding)
	}
	if cfg.GitConfigForwarding != nil {
		slog.Debug("git_config_forwarding", "enabled", *cfg.GitConfigForwarding)
	}

	overrideDir, cleanup, err := override.Create(workspaceFolder)
	if err != nil {
		return fmt.Errorf("creating override config: %w", err)
	}
	defer cleanup()

	slog.Info("override dir", "path", overrideDir)

	dcArgs := flags.Build(workspaceFolder, cfg, overrideDir)

	dcArgs = append(dcArgs, args...)

	slog.Debug("invoking devcontainer", "args", dcArgs)

	return runner.Run(devcontainerPath, dcArgs)
}

// parseLogLevel converts a string log level name to a slog.Level value.
func parseLogLevel(s string) (slog.Level, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(s)); err != nil {
		return 0, err
	}
	return level, nil
}
