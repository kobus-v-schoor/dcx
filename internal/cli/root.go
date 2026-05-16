package cli

import (
	"fmt"
	"log/slog"

	"github.com/kobus-v-schoor/dcx/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
	root.PersistentFlags().String("log-level", "", "log level (debug, info, warn, error)")
	root.PersistentFlags().StringVar(&workspaceFolder, "workspace-folder", ".", "path to the workspace folder")

	// Bind --log-level flag to viper so the CLI flag takes precedence over
	// config file and env var values, matching viper's precedence chain.
	if err := viper.BindPFlag("log_level", root.PersistentFlags().Lookup("log-level")); err != nil {
		return fmt.Errorf("binding log-level flag: %w", err)
	}

	// Sets up the logging level for each command. Config is loaded first, then
	// the effective log level is resolved from viper (which already applies the
	// precedence: flag → env → config → default).
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(workspaceFolder)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		activeCfg = cfg

		effective := resolveLogLevel(cfg.LogLevel)
		level, err := parseLogLevel(effective)
		if err != nil {
			return fmt.Errorf("invalid log level %q: %w", effective, err)
		}

		slog.SetLogLoggerLevel(level)

		return nil
	}

	root.AddCommand(newUpCmd())

	return root.Execute()
}

// resolveLogLevel picks the effective log level. Viper has already resolved
// flag → env → config precedence, so cfgLevel is the merged value. An empty
// string means nothing was set at any level, so "warn" is used as default.
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
