// Package compose provides Docker Compose lifecycle management using the Docker
// Engine API. It discovers compose projects by reading labels that the Docker
// Compose runtime sets on containers, then operates on all containers in a
// project collectively.
//
// This package is used by dcx stop, down, and (in future) ps commands to extend
// container management beyond the devcontainer itself to the full compose stack.
package compose

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/kobus-v-schoor/dcx/internal/docker"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

const (
	// labelProject is the Docker label key set by Docker Compose on every
	// container it manages. The value is the compose project name.
	labelProject = "com.docker.compose.project"
)

// Project holds the minimal metadata needed to identify a Docker Compose
// project from container labels.
type Project struct {
	// Name is the compose project name (from com.docker.compose.project).
	Name string
}

// ExtractProject parses compose-related labels from a container's label map.
// Returns nil if the container is not managed by Docker Compose (i.e. the
// com.docker.compose.project label is absent or empty).
func ExtractProject(labels map[string]string) *Project {
	name := labels[labelProject]
	if name == "" {
		return nil
	}
	return &Project{Name: name}
}

// FindContainers lists all containers (running and stopped) that belong to the
// given compose project. Returns an empty slice if no containers are found.
func FindContainers(ctx context.Context, cli docker.DockerClient, project *Project) (client.ContainerListResult, error) {
	slog.Debug("searching for compose containers", "project", project.Name)

	result, err := cli.ContainerList(ctx, client.ContainerListOptions{
		All: true,
		Filters: client.Filters{
			"label": {labelProject + "=" + project.Name: true},
		},
	})
	if err != nil {
		return client.ContainerListResult{}, fmt.Errorf("listing compose containers: %w", err)
	}

	slog.Debug("found compose containers", "project", project.Name, "count", len(result.Items))
	return result, nil
}

// Stop stops all running containers in the compose project. Already-stopped
// containers are silently skipped. Returns an error if any running container
// fails to stop.
func Stop(ctx context.Context, cli docker.DockerClient, project *Project) error {
	containers, err := FindContainers(ctx, cli, project)
	if err != nil {
		return err
	}

	if len(containers.Items) == 0 {
		return nil
	}

	for _, ctr := range containers.Items {
		// Skip containers that are already stopped.
		if !isContainerRunning(ctr) {
			slog.Debug("compose container already stopped, skipping", "id", docker.ShortID(ctr.ID), "project", project.Name)
			continue
		}

		slog.Info("stopping compose container", "id", docker.ShortID(ctr.ID), "service", ctr.Labels["com.docker.compose.service"], "project", project.Name)

		if _, err := cli.ContainerStop(ctx, ctr.ID, client.ContainerStopOptions{}); err != nil {
			return fmt.Errorf("stopping compose container %s: %w", docker.ShortID(ctr.ID), err)
		}

		slog.Info("compose container stopped", "id", docker.ShortID(ctr.ID), "project", project.Name)
	}

	return nil
}

// Down stops and removes all containers in the compose project. If
// removeVolumes is true, it also removes named volumes mounted by those
// containers (best-effort — volume removal failures are logged but not fatal,
// since other projects may still reference the volume).
func Down(ctx context.Context, cli docker.DockerClient, project *Project, removeVolumes bool) error {
	containers, err := FindContainers(ctx, cli, project)
	if err != nil {
		return err
	}

	if len(containers.Items) == 0 {
		return nil
	}

	// Collect unique volume names across all compose containers for optional
	// cleanup after containers are removed.
	volumeNames := make(map[string]struct{})

	for _, ctr := range containers.Items {
		// Gather volume mounts before the container disappears.
		if removeVolumes {
			for _, m := range ctr.Mounts {
				if m.Type == mount.TypeVolume && m.Name != "" {
					volumeNames[m.Name] = struct{}{}
				}
			}
		}

		if isContainerRunning(ctr) {
			slog.Info("stopping compose container", "id", docker.ShortID(ctr.ID), "service", ctr.Labels["com.docker.compose.service"], "project", project.Name)

			if _, err := cli.ContainerStop(ctx, ctr.ID, client.ContainerStopOptions{}); err != nil {
				return fmt.Errorf("stopping compose container %s: %w", docker.ShortID(ctr.ID), err)
			}
		}

		slog.Info("removing compose container", "id", docker.ShortID(ctr.ID), "project", project.Name)

		if _, err := cli.ContainerRemove(ctx, ctr.ID, client.ContainerRemoveOptions{}); err != nil {
			// A container may already have been removed by an earlier step
			// (e.g. docker.Down removes the devcontainer container before
			// compose.Down runs). Log and continue in that case.
			if strings.Contains(err.Error(), "No such container") || strings.Contains(err.Error(), "not found") {
				slog.Debug("compose container already removed, skipping", "id", docker.ShortID(ctr.ID), "project", project.Name)
				continue
			}
			return fmt.Errorf("removing compose container %s: %w", docker.ShortID(ctr.ID), err)
		}

		slog.Info("compose container removed", "id", docker.ShortID(ctr.ID), "project", project.Name)
	}

	// Remove collected volumes. This is best-effort: if another container
	// still references the volume, Docker will refuse and we log rather than
	// fail the entire down operation.
	if removeVolumes {
		for name := range volumeNames {
			slog.Info("removing compose volume", "name", name, "project", project.Name)

			if _, err := cli.VolumeRemove(ctx, name, client.VolumeRemoveOptions{}); err != nil {
				slog.Debug("could not remove compose volume (may still be in use)", "name", name, "error", err)
			}
		}
	}

	return nil
}

// isContainerRunning reports whether a container summary indicates a running
// state. It checks the State.Status field for "running".
func isContainerRunning(ctr container.Summary) bool {
	return ctr.State == container.StateRunning
}
