package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	gosdkclient "github.com/docker/go-sdk/client"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

// DockerClient is a narrow interface over the Docker Engine API, exposing only
// the operations needed by dcx (container lifecycle, image cleanup, file
// copy, and exec). The production implementation is *client.Client (obtained
// via the docker go-sdk), which satisfies this interface. A mock
// implementation is used in tests.
type DockerClient interface {
	Ping(ctx context.Context, options client.PingOptions) (client.PingResult, error)
	ContainerList(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error)
	ContainerInspect(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error)
	ContainerStop(ctx context.Context, containerID string, options client.ContainerStopOptions) (client.ContainerStopResult, error)
	ContainerRemove(ctx context.Context, containerID string, options client.ContainerRemoveOptions) (client.ContainerRemoveResult, error)
	ImageRemove(ctx context.Context, imageID string, options client.ImageRemoveOptions) (client.ImageRemoveResult, error)
	CopyToContainer(ctx context.Context, containerID string, options client.CopyToContainerOptions) (client.CopyToContainerResult, error)
	ExecCreate(ctx context.Context, containerID string, options client.ExecCreateOptions) (client.ExecCreateResult, error)
	ExecStart(ctx context.Context, execID string, options client.ExecStartOptions) (client.ExecStartResult, error)
	ExecInspect(ctx context.Context, execID string, options client.ExecInspectOptions) (client.ExecInspectResult, error)
	Close() error
}

// defaultDockerHost is the default Docker daemon socket path used when the
// docker go-sdk cannot resolve the current Docker context (e.g. when
// ~/.docker/ does not exist). It matches the go-sdk's own default on each
// platform.
var defaultDockerHost string

func init() {
	switch runtime.GOOS {
	case "windows":
		defaultDockerHost = "npipe://./pipe/docker_engine"
	default:
		defaultDockerHost = "unix:///var/run/docker.sock"
	}
}

// NewClient creates a Docker Engine API client configured from the current
// Docker context. Unlike the raw moby client's FromEnv (which only reads
// DOCKER_HOST), the docker go-sdk resolves the Docker host by inspecting
// the Docker CLI config (~/.docker/config.json) and context metadata
// (~/.docker/contexts/meta/...), so tools like Colima that set a custom
// context work out of the box.
//
// The docker go-sdk has a known issue where it fails if ~/.docker/ does not
// exist: config.Dir() returns a fmt.Errorf-wrapped "file does not exist"
// error, which os.IsNotExist does not match, preventing the SDK's own
// fallback to the default context. This function works around that by
// detecting the missing config dir and retrying with an explicit Docker
// host, which skips context resolution entirely.
//
// The client also performs a health check (ping with retries) during
// construction, so the caller can assume the daemon is reachable if no
// error is returned. The caller must call Close() when done. Used by all
// dcx commands that interact with Docker directly.
func NewClient(ctx context.Context) (DockerClient, error) {
	sdkClient, err := gosdkclient.New(ctx, gosdkclient.WithLogger(slog.Default()))
	if err != nil {
		// If the error is caused by a missing Docker config directory,
		// retry with an explicit Docker host. The go-sdk's config.Dir()
		// returns a fmt.Errorf-wrapped "file does not exist" error which
		// os.IsNotExist does not match, so the SDK's own fallback to the
		// default context never triggers. Passing WithDockerHost bypasses
		// the context resolution entirely.
		if isMissingDockerConfigDir(err) {
			slog.Debug("docker config dir not found, falling back to default docker host", "host", defaultDockerHost)
			sdkClient, err = gosdkclient.New(ctx, gosdkclient.WithLogger(slog.Default()), gosdkclient.WithDockerHost(defaultDockerHost))
			if err != nil {
				return nil, fmt.Errorf("creating Docker client with default host: %w", err)
			}
			return sdkClient, nil
		}
		return nil, fmt.Errorf("creating Docker client: %w", err)
	}
	return sdkClient, nil
}

// isMissingDockerConfigDir checks whether the given error (or any error in
// its chain) was caused by the Docker config directory not existing. This
// handles the docker go-sdk's config.Dir() error format
// ("file does not exist (<path>)"), which does not wrap os.ErrNotExist and
// therefore is not matched by os.IsNotExist.
func isMissingDockerConfigDir(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "file does not exist") &&
		strings.Contains(err.Error(), ".docker")
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

// FindDevcontainers lists all containers (running and stopped) that were
// created by the devcontainer CLI for the given workspace folder. Returns an
// empty slice if none are found. Exported so the exec command can check
// whether a devcontainer exists before attempting to exec into it.
func FindDevcontainers(ctx context.Context, cli DockerClient, workspaceFolder string) (client.ContainerListResult, error) {
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

// GatewayIP inspects the given container and returns the gateway IP address
// of its primary network. This is the IP address the container can use to
// reach the host. Used by dcx exec to determine the host IP so the
// container can connect to the GitHub API proxy running on the host.
func GatewayIP(ctx context.Context, cli DockerClient, containerID string) (string, error) {
	inspect, err := cli.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{})
	if err != nil {
		return "", fmt.Errorf("inspecting container %s for gateway IP: %w", shortID(containerID), err)
	}

	for _, net := range inspect.Container.NetworkSettings.Networks {
		if net.Gateway.IsValid() {
			return net.Gateway.String(), nil
		}
	}

	return "", fmt.Errorf("no gateway IP found for container %s", shortID(containerID))
}

// Stop stops the devcontainer for the given workspace folder without removing
// it. Returns an error if no devcontainer is found. If the container is
// already stopped, this is a no-op.
func Stop(ctx context.Context, cli DockerClient, workspaceFolder string) error {
	containers, err := FindDevcontainers(ctx, cli, workspaceFolder)
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
// removes the associated image. If no devcontainer is found, it logs an info
// message and returns nil (the desired end-state is already achieved).
// Image removal errors are logged but not treated as fatal, since other
// containers may still reference the image.
func Down(ctx context.Context, cli DockerClient, workspaceFolder string) error {
	containers, err := FindDevcontainers(ctx, cli, workspaceFolder)
	if err != nil {
		return err
	}

	if len(containers.Items) == 0 {
		absPath, _ := filepath.Abs(workspaceFolder)
		slog.Info("no devcontainer found for workspace, nothing to stop", "path", absPath)
		return nil
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

// CheckStaleMounts inspects the given container and returns an error if any
// bind mount source paths no longer exist on the host. The details are logged
// so the user can see which paths are missing; the returned error is a short,
// generic message. If no stale mounts are found, it returns nil.
func CheckStaleMounts(ctx context.Context, cli DockerClient, containerID string) error {
	inspect, err := cli.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{})
	if err != nil {
		return fmt.Errorf("inspecting container %s: %w", shortID(containerID), err)
	}

	var stale []string
	for _, m := range inspect.Container.Mounts {
		if m.Type == mount.TypeBind && m.Source != "" {
			if _, err := os.Stat(m.Source); os.IsNotExist(err) {
				stale = append(stale, m.Source)
			}
		}
	}

	if len(stale) > 0 {
		slog.Error(
			"stale bind mount(s) detected",
			"container", shortID(containerID),
			"missing_paths", stale,
			"resolution", "restore the missing path(s), or remove the mount and run 'dcx up --rebuild'",
			"note", "SSH agent sockets can change path when rebooting or restarting your SSH agent",
		)
		return fmt.Errorf("stale bind mounts detected on container %s", shortID(containerID))
	}

	return nil
}

// CopyFileToContainer copies a file from the host into a running container.
// It reads the file at hostPath, creates a tar archive in memory, and uses
// the Docker API's CopyToContainer to place it at containerDir inside the
// container. The file retains its basename. Used by dcx exec to copy the
// proxy's CA certificate into the container so the gh CLI trusts the proxy's
// self-signed TLS certificate.
func CopyFileToContainer(ctx context.Context, cli DockerClient, containerID, hostPath, containerDir string) error {
	data, err := os.ReadFile(hostPath)
	if err != nil {
		return fmt.Errorf("reading host file %s: %w", hostPath, err)
	}

	return CopyBytesToContainer(ctx, cli, containerID, filepath.Base(hostPath), data, containerDir)
}

// CopyBytesToContainer copies the given content as a file into a running
// container. It creates a tar archive in memory containing a single file with
// the given name and content, and uses the Docker API's CopyToContainer to
// place it at containerDir inside the container. The target directory must
// already exist inside the container — use MkdirInContainer to create it
// first. Used by dcx exec to write the proxy's CA certificate into the
// container.
func CopyBytesToContainer(ctx context.Context, cli DockerClient, containerID, fileName string, content []byte, containerDir string) error {
	// Create a tar archive containing the file. The Docker API's
	// CopyToContainer expects a tar archive, not a raw file.
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	hdr := &tar.Header{
		Name: fileName,
		Mode: 0o644,
		Size: int64(len(content)),
	}

	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("writing tar header: %w", err)
	}

	if _, err := tw.Write(content); err != nil {
		return fmt.Errorf("writing tar content: %w", err)
	}

	if err := tw.Close(); err != nil {
		return fmt.Errorf("closing tar archive: %w", err)
	}

	_, err := cli.CopyToContainer(ctx, containerID, client.CopyToContainerOptions{
		DestinationPath: containerDir,
		Content:         &buf,
	})
	if err != nil {
		return fmt.Errorf("copying to container %s: %w", shortID(containerID), err)
	}

	return nil
}

// MkdirInContainer creates a directory inside a running container by
// executing mkdir -p via the Docker exec API. Used by dcx exec to ensure
// the target directory exists before copying the CA certificate into the
// container.
func MkdirInContainer(ctx context.Context, cli DockerClient, containerID, dir string) error {
	return ExecInContainer(ctx, cli, containerID, "mkdir", "-p", dir)
}

// ExecInContainer executes the given command inside a running container via
// the Docker exec API. Returns an error if the exec cannot be created, fails
// to start, or exits with a non-zero code. Used by dcx exec to run commands
// inside the container (e.g. creating directories, building CA bundles).
func ExecInContainer(ctx context.Context, cli DockerClient, containerID string, cmd ...string) error {
	execCreate, err := cli.ExecCreate(ctx, containerID, client.ExecCreateOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return fmt.Errorf("creating exec in container %s: %w", shortID(containerID), err)
	}

	_, err = cli.ExecStart(ctx, execCreate.ID, client.ExecStartOptions{})
	if err != nil {
		return fmt.Errorf("running exec in container %s: %w", shortID(containerID), err)
	}

	// Check the exit code of the command.
	inspect, err := cli.ExecInspect(ctx, execCreate.ID, client.ExecInspectOptions{})
	if err != nil {
		return fmt.Errorf("inspecting exec in container %s: %w", shortID(containerID), err)
	}

	if inspect.ExitCode != 0 {
		return fmt.Errorf("command %v in container %s exited with code %d", cmd, shortID(containerID), inspect.ExitCode)
	}

	return nil
}
