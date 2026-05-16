package cli

import (
	"fmt"
	"log/slog"

	"github.com/kobus-v-schoor/dcx/internal/config"
	"github.com/spf13/cobra"
)

var (
	logLevel        string
	workspaceFolder string
	activeCfg       *config.Config
)

// Execute creates and executes the root command tree. The version string v is
// injected at build time via -ldflags. Called from main.go.
func Execute(v string) error {
	var showVersion bool

	root := &cobra.Command{
		Use:   "dcx",
		Short: "DevContainer Extended — wraps devcontainer CLI with user-level persistence",
		RunE: func(cmd *cobra.Command, args []string) error {
			if showVersion {
				fmt.Println("dcx " + v)
				return nil
			}
			return cmd.Help()
		},
	}

	root.Flags().BoolVar(&showVersion, "version", false, "print the version")
	root.PersistentFlags().StringVar(&logLevel, "log-level", "", "log level (debug, info, warn, error)")
	root.PersistentFlags().StringVar(&workspaceFolder, "workspace-folder", ".", "path to the workspace folder")

	// sets up the logging level for each command
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// load the effective config
		cfg, err := config.Load(workspaceFolder)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		activeCfg = cfg

		// get the log level from the config and parse it
		effective := resolveLogLevel(logLevel, cfg.LogLevel)
		level, err := parseLogLevel(effective)
		if err != nil {
			return fmt.Errorf("invalid log level %q: %w", effective, err)
		}

		// set the logger to use the configured log level
		slog.SetLogLoggerLevel(level)

		return nil
	}

	root.AddCommand(newUpCmd())

	return root.Execute()
}

// resolveLogLevel picks the effective log level using the standard precedence
// chain: CLI flag (highest) → config (which already includes env overrides) →
// "warn" default (lowest). An empty string at any level means "not set" and
// falls through to the next level.
func resolveLogLevel(cliFlag, cfgLevel string) string {
	if cliFlag != "" {
		return cliFlag
	}
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
