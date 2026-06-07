package cli

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kobus-v-schoor/dcx/internal/compose"
	"github.com/kobus-v-schoor/dcx/internal/docker"
	"github.com/spf13/cobra"
)

// newStopCmd creates the "stop" subcommand. It finds the devcontainer for the
// current project and stops it without removing it. If the devcontainer is part
// of a Docker Compose project, all related compose containers are stopped as
// well. Added to the root command tree in Execute().
func newStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the devcontainer for this project without removing it",
		Long:  "Finds the devcontainer for the current project by matching the devcontainer.local_folder label and stops it.\nIf the devcontainer is part of a Docker Compose project, all related compose containers are also stopped.\nUse --log-level info to see what is being stopped.",
		RunE:  runStop,
	}
}

// runStop implements the dcx stop workflow. Called by Cobra when the user
// runs "dcx stop". Config, log level, and Docker daemon reachability are
// already verified by the root command's PersistentPreRunE.
func runStop(cmd *cobra.Command, args []string) error {
	slog.Info("workspace-folder", "path", workspaceFolder)

	cli, err := docker.NewClient(cmd.Context())
	if err != nil {
		return err
	}
	defer func() { _ = cli.Close() }()

	// Discover compose project info before stopping the devcontainer, so
	// we can still read its labels after it is stopped.
	composeProjects, err := findComposeProjects(cmd.Context(), cli, workspaceFolder)
	if err != nil {
		return fmt.Errorf("dcx stop: %w", err)
	}

	if err := docker.Stop(cmd.Context(), cli, workspaceFolder); err != nil {
		return fmt.Errorf("dcx stop: %w", err)
	}

	// If the devcontainer is managed by Docker Compose, also stop the related
	// compose containers so the full stack is brought down together.
	for _, project := range composeProjects {
		slog.Info("stopping compose project", "project", project.Name)
		if err := compose.Stop(cmd.Context(), cli, project); err != nil {
			return fmt.Errorf("dcx stop: %w", err)
		}
	}

	return nil
}

// findComposeProjects lists devcontainers for the workspace and extracts unique
// Docker Compose project names from their labels. Returns nil when no
// devcontainer exists or none are part of a compose project.
func findComposeProjects(ctx context.Context, cli docker.DockerClient, workspaceFolder string) ([]*compose.Project, error) {
	containers, err := docker.FindDevcontainers(ctx, cli, workspaceFolder)
	if err != nil {
		return nil, err
	}

	var projects []*compose.Project
	seen := make(map[string]bool)
	for _, ctr := range containers.Items {
		project := compose.ExtractProject(ctr.Labels)
		if project == nil || seen[project.Name] {
			continue
		}
		seen[project.Name] = true
		projects = append(projects, project)
	}

	return projects, nil
}
