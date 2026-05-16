package docker

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/moby/moby/client"
)

// DockerClient is a narrow interface over the Docker Engine API, exposing only
// the operations needed by dcx (container lifecycle and image cleanup). The
// production implementation is *client.Client, which satisfies this interface.
// A mock implementation is used in tests.
type DockerClient interface {
	Ping(ctx context.Context, options client.PingOptions) (client.PingResult, error)
	ContainerList(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error)
	ContainerInspect(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error)
	ContainerStop(ctx context.Context, containerID string, options client.ContainerStopOptions) (client.ContainerStopResult, error)
	ContainerRemove(ctx context.Context, containerID string, options client.ContainerRemoveOptions) (client.ContainerRemoveResult, error)
	ImageRemove(ctx context.Context, imageID string, options client.ImageRemoveOptions) (client.ImageRemoveResult, error)
	Close() error
}

// NewClient creates a Docker Engine API client configured from the environment
// (DOCKER_HOST, DOCKER_API_VERSION, etc.). The caller must call Close() when
// done. Used by all dcx commands that interact with Docker directly.
func NewClient() (DockerClient, error) {
	cli, err := client.New(client.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("creating Docker client: %w", err)
	}
	return cli, nil
}

// CheckDaemon verifies the Docker daemon is reachable by calling Ping. Returns
// a user-friendly error if the daemon cannot be contacted. Called at the start
// of every dcx command that needs Docker, before any other API calls.
func CheckDaemon(ctx context.Context, cli DockerClient) error {
	_, err := cli.Ping(ctx, client.PingOptions{})
	if err != nil {
		return fmt.Errorf("cannot connect to Docker daemon: is Docker running?\nDetails: %w", err)
	}
	return nil
}

const (
	// DevcontainerLabel is the Docker label key that the devcontainer CLI sets on
	// containers it manages. The value is the absolute path of the workspace folder.
	DevcontainerLabel = "devcontainer.local_folder"

	// shortIDLen is the number of characters to show from a container or image ID
	// in log output, matching the Docker CLI convention.
	shortIDLen = 12
)

// shortID returns the first shortIDLen characters of a Docker ID string for
// human-readable log output.
func shortID(id string) string {
	if len(id) > shortIDLen {
		return id[:shortIDLen]
	}
	return id
}

// findDevcontainers lists all containers (running and stopped) that were
// created by the devcontainer CLI for the given workspace folder. Returns an
// empty slice if none are found.
func findDevcontainers(ctx context.Context, cli DockerClient, workspaceFolder string) (client.ContainerListResult, error) {
	absPath, err := filepath.Abs(workspaceFolder)
	if err != nil {
		return client.ContainerListResult{}, fmt.Errorf("resolving workspace path: %w", err)
	}

	slog.Debug("searching for devcontainers", "label", DevcontainerLabel, "value", absPath)

	result, err := cli.ContainerList(ctx, client.ContainerListOptions{
		All: true,
		Filters: client.Filters{
			"label": {DevcontainerLabel + "=" + absPath: true},
		},
	})
	if err != nil {
		return client.ContainerListResult{}, fmt.Errorf("listing containers: %w", err)
	}

	return result, nil
}

// Stop stops the devcontainer for the given workspace folder without removing
// it. Returns an error if no devcontainer is found. If the container is
// already stopped, this is a no-op.
func Stop(ctx context.Context, cli DockerClient, workspaceFolder string) error {
	containers, err := findDevcontainers(ctx, cli, workspaceFolder)
	if err != nil {
		return err
	}

	if len(containers.Items) == 0 {
		absPath, _ := filepath.Abs(workspaceFolder)
		return fmt.Errorf("no devcontainer found for %s", absPath)
	}

	for _, ctr := range containers.Items {
		slog.Info("stopping container", "id", shortID(ctr.ID), "image", ctr.Image)

		if _, err := cli.ContainerStop(ctx, ctr.ID, client.ContainerStopOptions{}); err != nil {
			return fmt.Errorf("stopping container %s: %w", shortID(ctr.ID), err)
		}

		slog.Info("container stopped", "id", shortID(ctr.ID))
	}

	return nil
}

// Down stops and removes the devcontainer for the given workspace folder, then
// removes the associated image. Returns an error if no devcontainer is found.
// Image removal errors are logged but not treated as fatal, since other
// containers may still reference the image.
func Down(ctx context.Context, cli DockerClient, workspaceFolder string) error {
	containers, err := findDevcontainers(ctx, cli, workspaceFolder)
	if err != nil {
		return err
	}

	if len(containers.Items) == 0 {
		absPath, _ := filepath.Abs(workspaceFolder)
		return fmt.Errorf("no devcontainer found for %s", absPath)
	}

	for _, ctr := range containers.Items {
		// Inspect before stopping/removing to capture the image ID for cleanup.
		inspect, err := cli.ContainerInspect(ctx, ctr.ID, client.ContainerInspectOptions{})
		if err != nil {
			return fmt.Errorf("inspecting container %s: %w", shortID(ctr.ID), err)
		}
		imageID := inspect.Container.Image

		slog.Info("stopping container", "id", shortID(ctr.ID), "image", ctr.Image)

		if _, err := cli.ContainerStop(ctx, ctr.ID, client.ContainerStopOptions{}); err != nil {
			return fmt.Errorf("stopping container %s: %w", shortID(ctr.ID), err)
		}

		slog.Info("removing container", "id", shortID(ctr.ID))

		if _, err := cli.ContainerRemove(ctx, ctr.ID, client.ContainerRemoveOptions{}); err != nil {
			return fmt.Errorf("removing container %s: %w", shortID(ctr.ID), err)
		}

		// Attempt image cleanup. This is best-effort: if another container
		// still references the image, Docker will refuse and we log a debug
		// message rather than failing the entire down operation.
		if imageID != "" {
			slog.Info("removing image", "id", shortID(imageID))

			if _, err := cli.ImageRemove(ctx, imageID, client.ImageRemoveOptions{}); err != nil {
				slog.Debug("could not remove image (may still be in use)", "id", shortID(imageID), "error", err)
			}
		}
	}

	return nil
}
