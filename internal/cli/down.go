package cli

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kobus-v-schoor/dcx/internal/compose"
	"github.com/kobus-v-schoor/dcx/internal/docker"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"github.com/spf13/cobra"
)

// newDownCmd creates the "down" subcommand. It finds the devcontainer for the
// current project, stops and removes it, and cleans up the associated image.
// If the devcontainer is part of a Docker Compose project, all related compose
// containers are brought down as well. The --volumes flag also removes named
// volumes used by the compose stack. Added to the root command tree in
// Execute().
func newDownCmd() *cobra.Command {
	var removeVolumes bool

	cmd := &cobra.Command{
		Use:   "down",
		Short: "Stop, remove, and clean up the devcontainer for this project",
		Long:  "Finds the devcontainer for the current project by matching the devcontainer.local_folder label, then stops and removes the container and its image.\nIf the devcontainer is part of a Docker Compose project, all related compose containers are also removed.\nUse --volumes to also remove named volumes declared by the compose project.\nUse --log-level info to see what is being stopped and removed.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDown(cmd.Context(), removeVolumes)
		},
	}

	cmd.Flags().BoolVar(&removeVolumes, "volumes", false, "also remove named volumes used by the compose project")

	return cmd
}

// runDown implements the dcx down workflow. Called by Cobra when the user
// runs "dcx down". The removeVolumes parameter maps to the --volumes CLI flag.
// Config, log level, and Docker daemon reachability are already verified by
// the root command's PersistentPreRunE.
func runDown(ctx context.Context, removeVolumes bool) error {
	slog.Info("workspace-folder", "path", workspaceFolder)

	cli, err := docker.NewClient(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = cli.Close() }()

	// Discover compose project info and volume names before removing the
	// devcontainer, because its labels and mount metadata disappear once
	// the container is gone.
	composeProjects, composeVolumes, err := findComposeProjectsAndVolumes(ctx, cli, workspaceFolder)
	if err != nil {
		return fmt.Errorf("dcx down: %w", err)
	}

	if err := docker.Down(ctx, cli, workspaceFolder); err != nil {
		return fmt.Errorf("dcx down: %w", err)
	}

	// If the devcontainer is managed by Docker Compose, also down the related
	// compose containers (and optionally their volumes) so the full stack is
	// cleaned up together.
	for _, project := range composeProjects {
		slog.Info("tearing down compose project", "project", project.Name, "volumes", removeVolumes)
		if err := compose.Down(ctx, cli, project, removeVolumes); err != nil {
			return fmt.Errorf("dcx down: %w", err)
		}
	}

	// Remove volumes that were attached to the devcontainer itself.
	// compose.Down won't find them because docker.Down already removed the
	// container, so we clean them up explicitly from the pre-collected list.
	if removeVolumes {
		for _, name := range composeVolumes {
			slog.Info("removing compose volume", "name", name)
			if _, err := cli.VolumeRemove(ctx, name, client.VolumeRemoveOptions{}); err != nil {
				slog.Debug("could not remove compose volume (may still be in use)", "name", name, "error", err)
			}
		}
	}

	return nil
}

// findComposeProjectsAndVolumes extracts unique Docker Compose projects and
// named volume names from the devcontainers for the given workspace. The
// volumes are collected from container Mounts so they can be removed after
// the containers themselves are gone. Returns nil/empty when no devcontainer
// exists or none are part of a compose project.
func findComposeProjectsAndVolumes(ctx context.Context, cli docker.DockerClient, workspaceFolder string) ([]*compose.Project, []string, error) {
	containers, err := docker.FindDevcontainers(ctx, cli, workspaceFolder)
	if err != nil {
		return nil, nil, err
	}

	var projects []*compose.Project
	var volumes []string
	seenProjects := make(map[string]bool)
	seenVolumes := make(map[string]bool)

	for _, ctr := range containers.Items {
		project := compose.ExtractProject(ctr.Labels)
		if project != nil && !seenProjects[project.Name] {
			seenProjects[project.Name] = true
			projects = append(projects, project)
		}

		for _, m := range ctr.Mounts {
			if m.Type == mount.TypeVolume && m.Name != "" && !seenVolumes[m.Name] {
				seenVolumes[m.Name] = true
				volumes = append(volumes, m.Name)
			}
		}
	}

	return projects, volumes, nil
}
