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
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
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

// Up creates or reuses a devcontainer for a non-compose, non-feature project.
// It substitutes variables, resolves mounts, parses runArgs, sets labels and
// metadata, creates the container, and starts it.
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

	// Step 2: parse runArgs.
	parsedRunArgs, err := ParseRunArgs(cfg.RunArgs)
	if err != nil {
		return "", fmt.Errorf("parsing runArgs: %w", err)
	}

	// Step 3: resolve mounts.
	workspaceMount, err := spec.ResolveWorkspaceMount(cfg, absHostFolder)
	if err != nil {
		return "", fmt.Errorf("resolving workspace mount: %w", err)
	}
	allMounts, err := resolveMounts(cfg.Mounts, workspaceMount)
	if err != nil {
		return "", fmt.Errorf("resolving mounts: %w", err)
	}

	// Merge runArgs mounts.
	allMounts = append(allMounts, parsedRunArgs.Mounts...)

	// Step 4: find existing containers by label.
	existing, err := docker.FindDevcontainers(ctx, cli, absHostFolder)
	if err != nil {
		return "", fmt.Errorf("finding existing containers: %w", err)
	}

	// Helper to build the container metadata label.
	metadataJSON, err := buildDevcontainerMetadata(ctx, cli, imageRef, cfg)
	if err != nil {
		return "", fmt.Errorf("building devcontainer metadata: %w", err)
	}

	// Step 5: handle existing container logic.
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

	// Step 6: build container and host configs.
	containerConfig, err := buildContainerConfig(cfg, absHostFolder, imageRef, parsedRunArgs, metadataJSON)
	if err != nil {
		return "", fmt.Errorf("building container config: %w", err)
	}

	hostConfig := buildHostConfig(cfg, workspaceMount, allMounts, parsedRunArgs)

	// Step 7: create and start the container.
	createResult, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:     containerConfig,
		HostConfig: hostConfig,
	})
	if err != nil {
		return "", fmt.Errorf("creating container: %w", err)
	}

	slog.Info("created container", "id", docker.ShortID(createResult.ID))

	if _, err := cli.ContainerStart(ctx, createResult.ID, client.ContainerStartOptions{}); err != nil {
		return "", fmt.Errorf("starting container %s: %w", docker.ShortID(createResult.ID), err)
	}

	slog.Info("started container", "id", docker.ShortID(createResult.ID))
	return createResult.ID, nil
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

// buildContainerConfig assembles container.Config from the merged spec, host
// workspace folder, image reference, runArgs env overrides, and the
// devcontainer metadata label JSON.
func buildContainerConfig(cfg *spec.Config, absHostFolder, imageRef string, parsedRunArgs *ParsedRunArgs, metadataJSON string) (*container.Config, error) {
	// Build the Docker-level Env slice from containerEnv (lower priority) and
	// runArgs --env overrides (higher priority).
	envMap := make(map[string]string, len(cfg.ContainerEnv)+len(parsedRunArgs.Env))
	for k, v := range cfg.ContainerEnv {
		envMap[k] = v
	}
	for k, v := range parsedRunArgs.Env {
		envMap[k] = v
	}

	var envList []string
	for k, v := range envMap {
		envList = append(envList, k+"="+v)
	}
	sort.Strings(envList)

	labels := map[string]string{
		docker.DevcontainerLabel:   absHostFolder,
		"devcontainer.config_file": filepath.Join(absHostFolder, ".devcontainer", "devcontainer.json"),
		devcontainerMetadataLabel:  metadataJSON,
		"dcx.managed":              "true",
	}

	c := &container.Config{
		Image:  imageRef,
		Labels: labels,
		Env:    envList,
		Tty:    false,
	}

	if cfg.ContainerUser != "" {
		c.User = cfg.ContainerUser
	}

	if cfg.WorkspaceFolder != "" {
		c.WorkingDir = cfg.WorkspaceFolder
	}

	if overrideCommandEnabled(cfg) {
		c.Entrypoint = []string{"/bin/sh"}
		c.Cmd = []string{"-c", keepAliveScript, "-"}
	}

	// Apply entrypoint override from runArgs if present.
	// runArgs entrypoint takes precedence over the overrideCommand sleep loop.
	// This matches Docker behaviour where --entrypoint overrides the image default.
	// In the devcontainer context, runArgs --entrypoint is unusual but we honour it.
	if len(parsedRunArgs.Entrypoint) > 0 {
		c.Entrypoint = parsedRunArgs.Entrypoint
	}

	return c, nil
}

// buildHostConfig assembles container.HostConfig from the workspace mount, spec
// mounts, runArgs, and spec fields.
func buildHostConfig(cfg *spec.Config, workspaceMount *spec.WorkspaceMount, mounts []mount.Mount, parsedRunArgs *ParsedRunArgs) *container.HostConfig {
	hc := &container.HostConfig{
		Mounts: mounts,
	}

	hc.Binds = append(hc.Binds, parsedRunArgs.Binds...)

	if parsedRunArgs.NetworkMode != "" {
		hc.NetworkMode = parsedRunArgs.NetworkMode
	}

	if parsedRunArgs.PortBindings != nil {
		hc.PortBindings = parsedRunArgs.PortBindings
	}

	hc.CapAdd = append(hc.CapAdd, parsedRunArgs.CapAdd...)
	hc.CapDrop = append(hc.CapDrop, parsedRunArgs.CapDrop...)

	if parsedRunArgs.Privileged {
		hc.Privileged = true
	}

	hc.SecurityOpt = append(hc.SecurityOpt, parsedRunArgs.SecurityOpt...)

	if parsedRunArgs.Init != nil {
		hc.Init = parsedRunArgs.Init
	}

	if len(parsedRunArgs.Devices) > 0 {
		hc.Devices = append(hc.Devices, parsedRunArgs.Devices...)
	}

	if len(parsedRunArgs.GroupAdd) > 0 {
		hc.GroupAdd = append(hc.GroupAdd, parsedRunArgs.GroupAdd...)
	}

	if parsedRunArgs.ReadonlyRootfs {
		hc.ReadonlyRootfs = true
	}

	if parsedRunArgs.Memory > 0 {
		hc.Memory = parsedRunArgs.Memory
	}

	if parsedRunArgs.NanoCPUs > 0 {
		hc.NanoCPUs = parsedRunArgs.NanoCPUs
	}

	if len(parsedRunArgs.Tmpfs) > 0 {
		hc.Tmpfs = parsedRunArgs.Tmpfs
	}

	return hc
}

// resolveMounts converts all spec.MountEntry strings into []mount.Mount after
// variable substitution. The workspace mount is also converted and included.
func resolveMounts(mountEntries []spec.MountEntry, workspaceMount *spec.WorkspaceMount) ([]mount.Mount, error) {
	var mounts []mount.Mount

	// Convert workspace mount.
	if workspaceMount != nil {
		// Reconstruct a mount string so we can reuse the existing parser.
		wmStr := fmt.Sprintf("type=%s,source=%s,target=%s", workspaceMount.Type, workspaceMount.Source, workspaceMount.Target)
		for _, opt := range workspaceMount.Options {
			wmStr += "," + opt
		}
		m, err := parseMountFlag(wmStr)
		if err != nil {
			return nil, fmt.Errorf("parsing workspace mount: %w", err)
		}
		mounts = append(mounts, m)
	}

	// Convert spec mounts.
	for _, entry := range mountEntries {
		if entry.IsEmpty() {
			continue
		}
		if s, ok := entry.AsString(); ok {
			m, err := parseMountFlag(s)
			if err != nil {
				return nil, fmt.Errorf("parsing mount %q: %w", s, err)
			}
			mounts = append(mounts, m)
		}
	}

	return mounts, nil
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
