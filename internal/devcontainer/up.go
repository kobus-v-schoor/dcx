package devcontainer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"

	"github.com/kobus-v-schoor/dcx/internal/devcontainer/spec"
	"github.com/kobus-v-schoor/dcx/internal/docker"
	"github.com/moby/moby/client"
)

// keepAliveScript is the shell script used by the devcontainer CLI to keep a
// container alive when overrideCommand is enabled. It traps SIGTERM (signal 15)
// so the container exits cleanly on stop, and loops with sleep so the shell
// stays running indefinitely.
const keepAliveScript = `echo Container started
trap "exit 0" 15

exec "$@"
while sleep 1 & wait $!; do :; done`

// createContainer is the function used by Up to create a container via the
// Docker CLI. In production it is docker.ContainerCreateCLI; tests may
// override it to capture constructed arguments without invoking Docker.
var createContainer = docker.ContainerCreateCLI

// startContainer is the function used by Up to start a container via the Docker
// CLI. In production it is docker.ContainerStartCLI; tests may override it.
var startContainer = docker.ContainerStartCLI

// Up creates or reuses a devcontainer for a non-compose, non-feature project.
// It substitutes variables, resolves mounts into string format, sets labels and
// metadata, creates the container via docker create, and starts it via
// docker start.
//
// When rebuild is false and a running container already exists, it returns the
// existing container ID. When rebuild is true, the existing container is removed
// and recreated. When a stopped container exists without stale mounts, it is
// started.
func Up(ctx context.Context, cli docker.DockerClient, cfg *spec.Config, hostWorkspaceFolder string, imageRef string, rebuild bool) (string, error) {
	absHostFolder, err := filepath.Abs(hostWorkspaceFolder)
	if err != nil {
		return "", fmt.Errorf("resolving workspace folder: %w", err)
	}

	// Step 1: variable substitution.
	if err := SubstituteAll(cfg, absHostFolder); err != nil {
		return "", fmt.Errorf("substituting variables: %w", err)
	}

	// Step 2: resolve mounts into string format for Docker CLI --mount flags.
	workspaceMount, err := spec.ResolveWorkspaceMount(cfg, absHostFolder)
	if err != nil {
		return "", fmt.Errorf("resolving workspace mount: %w", err)
	}
	allMounts := resolveMountStrings(cfg.Mounts, workspaceMount)

	// Step 3: find existing containers by label.
	existing, err := docker.FindDevcontainers(ctx, cli, absHostFolder)
	if err != nil {
		return "", fmt.Errorf("finding existing containers: %w", err)
	}

	// Helper to build the container metadata label.
	metadataJSON, err := buildDevcontainerMetadata(ctx, cli, imageRef, cfg)
	if err != nil {
		return "", fmt.Errorf("building devcontainer metadata: %w", err)
	}

	// Step 4: handle existing container logic.
	if len(existing.Items) > 0 {
		ctr := existing.Items[0]
		if docker.IsContainerRunning(ctr) {
			if !rebuild {
				slog.Info("reusing running devcontainer", "id", docker.ShortID(ctr.ID))
				return ctr.ID, nil
			}
			slog.Info("rebuilding running devcontainer", "id", docker.ShortID(ctr.ID))
			if err := removeExistingContainer(ctx, cli, ctr.ID); err != nil {
				return "", err
			}
		} else {
			// Container exists but is stopped.
			if !rebuild {
				slog.Info("starting stopped devcontainer", "id", docker.ShortID(ctr.ID))
				if _, err := cli.ContainerStart(ctx, ctr.ID, client.ContainerStartOptions{}); err != nil {
					return "", fmt.Errorf("starting container %s: %w", docker.ShortID(ctr.ID), err)
				}
				return ctr.ID, nil
			}
			slog.Info("removing stopped devcontainer for rebuild", "id", docker.ShortID(ctr.ID))
			if err := removeExistingContainer(ctx, cli, ctr.ID); err != nil {
				return "", err
			}
		}
	}

	// Step 5: build CLI arguments for docker create.
	labels := map[string]string{
		docker.DevcontainerLabel:   absHostFolder,
		"devcontainer.config_file": filepath.Join(absHostFolder, ".devcontainer", "devcontainer.json"),
		devcontainerMetadataLabel:  metadataJSON,
		"dcx.managed":              "true",
	}

	var envList []string
	for k, v := range cfg.ContainerEnv {
		envList = append(envList, k+"="+v)
	}
	sort.Strings(envList)

	var user, workdir, entrypoint string
	var cmdArgs []string

	if cfg.ContainerUser != "" {
		user = cfg.ContainerUser
	}
	if cfg.WorkspaceFolder != "" {
		workdir = cfg.WorkspaceFolder
	}
	if overrideCommandEnabled(cfg) {
		entrypoint = "/bin/sh"
		cmdArgs = []string{"-c", keepAliveScript, "-"}
	}

	// Step 6: create and start the container via Docker CLI.
	containerID, err := createContainer(ctx, imageRef, cfg.RunArgs, allMounts, envList, labels, user, workdir, entrypoint, cmdArgs)
	if err != nil {
		return "", fmt.Errorf("creating container: %w", err)
	}

	slog.Info("created container", "id", docker.ShortID(containerID))

	if err := startContainer(ctx, containerID); err != nil {
		return "", fmt.Errorf("starting container %s: %w", docker.ShortID(containerID), err)
	}

	slog.Info("started container", "id", docker.ShortID(containerID))
	return containerID, nil
}

// removeExistingContainer stops and removes a container by ID. Called during
// rebuild flows so the old container is fully cleaned up before a new one is
// created.
func removeExistingContainer(ctx context.Context, cli docker.DockerClient, containerID string) error {
	if _, err := cli.ContainerStop(ctx, containerID, client.ContainerStopOptions{}); err != nil {
		return fmt.Errorf("stopping container %s: %w", docker.ShortID(containerID), err)
	}
	if _, err := cli.ContainerRemove(ctx, containerID, client.ContainerRemoveOptions{}); err != nil {
		return fmt.Errorf("removing container %s: %w", docker.ShortID(containerID), err)
	}
	return nil
}

// resolveMountStrings converts all spec.MountEntry strings into a flat slice
// of Docker --mount flag strings. The workspace mount is also formatted and
// included as the first element. Non-string mount entries are skipped.
func resolveMountStrings(mountEntries []spec.MountEntry, workspaceMount *spec.WorkspaceMount) []string {
	var mounts []string

	if workspaceMount != nil {
		mounts = append(mounts, workspaceMount.String())
	}

	for _, entry := range mountEntries {
		if entry.IsEmpty() {
			continue
		}
		if s, ok := entry.AsString(); ok {
			mounts = append(mounts, s)
		}
	}

	return mounts
}

const devcontainerMetadataLabel = "devcontainer.metadata"

// buildDevcontainerMetadata inspects the base image, parses its metadata label,
// and appends the config's metadata object. Returns the JSON string for the
// label value. If the image has no metadata label, only the config object is
// returned.
func buildDevcontainerMetadata(ctx context.Context, cli docker.DockerClient, imageRef string, cfg *spec.Config) (string, error) {
	var meta []map[string]any

	// Attempt to read existing metadata from the image.
	inspect, err := cli.ImageInspect(ctx, imageRef)
	if err == nil && inspect.Config != nil && inspect.Config.Labels != nil {
		if raw := inspect.Config.Labels[devcontainerMetadataLabel]; raw != "" {
			if err := json.Unmarshal([]byte(raw), &meta); err != nil {
				// Non-array: try single object.
				var obj map[string]any
				if err := json.Unmarshal([]byte(raw), &obj); err == nil {
					meta = append(meta, obj)
				}
			}
		}
	}

	// Build the config metadata object with at minimum remoteUser,
	// containerUser, and containerEnv.
	configMeta := make(map[string]any)
	if cfg.RemoteUser != "" {
		configMeta["remoteUser"] = cfg.RemoteUser
	}
	if cfg.ContainerUser != "" {
		configMeta["containerUser"] = cfg.ContainerUser
	}
	if len(cfg.ContainerEnv) > 0 {
		configMeta["containerEnv"] = cfg.ContainerEnv
	}
	if cfg.WorkspaceFolder != "" {
		configMeta["workspaceFolder"] = cfg.WorkspaceFolder
	}
	if len(cfg.Mounts) > 0 {
		configMeta["mounts"] = cfg.Mounts
	}
	if cfg.OverrideCommand != nil {
		configMeta["overrideCommand"] = *cfg.OverrideCommand
	}
	if len(cfg.ForwardPorts) > 0 {
		configMeta["forwardPorts"] = cfg.ForwardPorts
	}
	if cfg.ShutdownAction != "" {
		configMeta["shutdownAction"] = cfg.ShutdownAction
	}
	if cfg.UpdateRemoteUserUID != nil {
		configMeta["updateRemoteUserUID"] = *cfg.UpdateRemoteUserUID
	}

	meta = append(meta, configMeta)

	b, err := json.Marshal(meta)
	if err != nil {
		return "", fmt.Errorf("marshalling metadata: %w", err)
	}
	return string(b), nil
}

// overrideCommandEnabled returns whether the container's default command should
// be overridden with the keep-alive script. When the spec explicitly sets
// overrideCommand, that value is used. Otherwise the devcontainer spec default
// is true for image/Dockerfile projects and false for compose. Since Up is only
// called for image/Dockerfile projects, the default here is true.
func overrideCommandEnabled(cfg *spec.Config) bool {
	if cfg.OverrideCommand != nil {
		return *cfg.OverrideCommand
	}
	return true
}
