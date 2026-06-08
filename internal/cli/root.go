package cli

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/kobus-v-schoor/dcx/internal/config"
	"github.com/kobus-v-schoor/dcx/internal/docker"
	"github.com/spf13/cobra"
)

var (
	workspaceFolder string
	activeCfg       *config.Config
)

// Execute creates and executes the root command tree. The version string v is
// injected at build time via -ldflags. Called from main.go.
func Execute(v string) error {
	var showVersion bool

	root := &cobra.Command{
		Use:           "dcx",
		Short:         "DevContainer Extended — wraps devcontainer CLI with user-level persistence",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if showVersion {
				fmt.Println("dcx " + v)
				return nil
			}
			return cmd.Help()
		},
	}

	root.Flags().BoolVar(&showVersion, "version", false, "print the version")
	root.PersistentFlags().String("log-level", "", "log level (debug, info, warn, error)")
	root.PersistentFlags().StringVar(&workspaceFolder, "workspace-folder", ".", "path to the workspace folder")

	// Sets up the logging level and verifies the Docker daemon is reachable
	// before any subcommand runs. Since every dcx command depends on Docker,
	// this healthcheck is performed once at the root level rather than in each
	// subcommand individually.
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(workspaceFolder)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		activeCfg = cfg

		// CLI flag takes highest precedence: if --log-level was explicitly set
		// it overrides config file and env var values.
		logLevel := cfg.LogLevel
		if flagVal, _ := cmd.Flags().GetString("log-level"); flagVal != "" {
			logLevel = flagVal
		}

		effective := resolveLogLevel(logLevel)
		level, err := parseLogLevel(effective)
		if err != nil {
			return fmt.Errorf("invalid log level %q: %w", effective, err)
		}

		handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
		slog.SetDefault(slog.New(handler))

		if err := checkDockerDaemon(cmd); err != nil {
			return err
		}

		return nil
	}
	root.AddCommand(newUpCmd())
	root.AddCommand(newDownCmd())
	root.AddCommand(newStopCmd())
	root.AddCommand(newPsCmd())
	root.AddCommand(newExecCmd())

	return root.Execute()
}

// resolveLogLevel picks the effective log level. cfgLevel is the merged value
// from config file and env var sources (with CLI flag already resolved by the
// caller). An empty string means nothing was set at any level, so "warn" is
// used as default.
func resolveLogLevel(cfgLevel string) string {
	if cfgLevel != "" {
		return cfgLevel
	}
	return "warn"
}

// parseLogLevel converts a string log level name to a slog.Level value.
// Delegates to slog.Level.UnmarshalText which accepts DEBUG, INFO, WARN, ERROR
// (case-insensitive).
func parseLogLevel(s string) (slog.Level, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(s)); err != nil {
		return 0, err
	}
	return level, nil
}

// checkDockerDaemon creates a Docker client to confirm the daemon is reachable.
// The docker go-sdk performs a health check (ping with retries) during client
// construction, so if NewClient returns no error the daemon is already verified.
// Called from PersistentPreRunE so that every dcx command fails early with a
// clear message if Docker is not running, rather than deeper in the command's
// own logic.
func checkDockerDaemon(cmd *cobra.Command) error {
	cli, err := docker.NewClient(cmd.Context())
	if err != nil {
		return err
	}
	defer func() { _ = cli.Close() }()

	slog.Info("Docker daemon reachable")
	return nil
}
