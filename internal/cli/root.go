package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	logLevel        string
	workspaceFolder string
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
	root.PersistentFlags().StringVar(&logLevel, "log-level", "warn", "log level (debug, info, warn, error)")
	root.PersistentFlags().StringVar(&workspaceFolder, "workspace-folder", ".", "path to the workspace folder")

	root.AddCommand(newUpCmd())

	return root.Execute()
}
