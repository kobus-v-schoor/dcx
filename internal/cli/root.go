package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	verbose        bool
	workspaceFolder string
)

// Execute creates and executes the root command tree. The version string v is
// injected at build time via -ldflags. Scope: CLI entry point. Called from
// main.go.
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
	root.PersistentFlags().BoolVar(&verbose, "verbose", false, "print detailed information about what dcx is doing")
	root.PersistentFlags().StringVar(&workspaceFolder, "workspace-folder", ".", "path to the workspace folder")

	root.AddCommand(newUpCmd())

	return root.Execute()
}
