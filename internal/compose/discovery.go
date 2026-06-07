package compose

import (
	"context"

	"github.com/kobus-v-schoor/dcx/internal/docker"
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
