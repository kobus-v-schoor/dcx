package compose

import (
	"context"

	"github.com/kobus-v-schoor/dcx/internal/docker"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
)

// FindProjectsAndVolumes discovers Docker Compose projects and named volumes
// from the devcontainers for the given workspace folder. It returns unique
// projects and volume names so they can be stopped or removed together with
// the devcontainer. Returns nil/empty slices when no devcontainer exists or
// none are part of a compose project.
func FindProjectsAndVolumes(ctx context.Context, cli docker.DockerClient, workspaceFolder string) ([]*Project, []string, error) {
	containers, err := docker.FindDevcontainers(ctx, cli, workspaceFolder)
	if err != nil {
		return nil, nil, err
	}

	var projects []*Project
	var volumes []string
	seenProjects := make(map[string]bool)
	seenVolumes := make(map[string]bool)

	for _, ctr := range containers.Items {
		project := ExtractProject(ctr.Labels)
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

// FindProjectContainers discovers all containers associated with the given
// workspace folder: the devcontainer itself (if any) plus all Docker Compose
// containers that belong to the same compose project as the devcontainer.
// Containers are deduplicated by ID so a devcontainer that is itself part
// of a compose project is not listed twice. The returned slice is unordered;
// callers should sort if stable output is required.
func FindProjectContainers(ctx context.Context, cli docker.DockerClient, workspaceFolder string) ([]container.Summary, error) {
	devcontainers, err := docker.FindDevcontainers(ctx, cli, workspaceFolder)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	var all []container.Summary

	for _, ctr := range devcontainers.Items {
		if _, ok := seen[ctr.ID]; ok {
			continue
		}
		seen[ctr.ID] = struct{}{}
		all = append(all, ctr)
	}

	projects, _, err := FindProjectsAndVolumes(ctx, cli, workspaceFolder)
	if err != nil {
		return nil, err
	}

	for _, project := range projects {
		composeContainers, err := FindContainers(ctx, cli, project)
		if err != nil {
			return nil, err
		}
		for _, ctr := range composeContainers.Items {
			if _, ok := seen[ctr.ID]; ok {
				continue
			}
			seen[ctr.ID] = struct{}{}
			all = append(all, ctr)
		}
	}

	return all, nil
}
