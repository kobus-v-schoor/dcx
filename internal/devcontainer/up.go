package devcontainer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kobus-v-schoor/dcx/internal/devcontainer/spec"
	"github.com/kobus-v-schoor/dcx/internal/docker"
	"github.com/moby/moby/client"
)

// keepAliveScript is the shell script used to keep a container alive when
// overrideCommand is enabled. It traps SIGTERM (signal 15)
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

// postCreateRunner is called by Up after a new container is created and
// started to run any configured postCreateCommand. In production it is
// RunPostCreate; tests may override it to avoid invoking Docker.
var postCreateRunner = RunPostCreate

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
				if err := startContainer(ctx, ctr.ID); err != nil {
					return "", fmt.Errorf("starting container %s: %w", docker.ShortID(ctr.ID), err)
				}
				// Run post-start commands for a container that was just started.
				meta, _ := extractImageMetadata(ctx, cli, imageRef)
				meta.substitute(absHostFolder, cfg.WorkspaceFolder)
				meta.runPostStarts(ctx, ctr.ID, cfg)
				return ctr.ID, nil
			}
			slog.Info("removing stopped devcontainer for rebuild", "id", docker.ShortID(ctr.ID))
			if err := removeExistingContainer(ctx, cli, ctr.ID); err != nil {
				return "", err
			}
		}
	}

	// Step 5: extract feature metadata from the built image and merge it.
	imgMeta, err := extractImageMetadata(ctx, cli, imageRef)
	if err != nil {
		return "", fmt.Errorf("extracting image metadata: %w", err)
	}
	imgMeta.substitute(absHostFolder, cfg.WorkspaceFolder)

	// Merge feature mounts into the list passed to docker create.
	for _, m := range imgMeta.Mounts {
		if s, ok := m.AsString(); ok {
			allMounts = append(allMounts, s)
		} else {
			if s, err := mountEntryObjectToString(m); err == nil {
				allMounts = append(allMounts, s)
			}
		}
	}

	// Build environment variable list (config + feature metadata).
	var envList []string
	for k, v := range cfg.ContainerEnv {
		envList = append(envList, k+"="+v)
	}
	for k, v := range imgMeta.ContainerEnv {
		found := false
		for _, existing := range envList {
			if strings.HasPrefix(existing, k+"=") {
				found = true
				break
			}
		}
		if !found {
			envList = append(envList, k+"="+v)
		}
	}
	sort.Strings(envList)

	// Merge runArgs with feature metadata flags.
	runArgs := buildMergedRunArgs(cfg.RunArgs, imgMeta)

	// Step 6: build CLI arguments for docker create.
	labels := map[string]string{
		docker.DevcontainerLabel:   absHostFolder,
		"devcontainer.config_file": filepath.Join(absHostFolder, ".devcontainer", "devcontainer.json"),
		devcontainerMetadataLabel:  metadataJSON,
		"dcx.managed":              "true",
	}

	var user, workdir, entrypoint string
	var cmdArgs []string

	if cfg.ContainerUser != "" {
		user = cfg.ContainerUser
	}
	if cfg.WorkspaceFolder != "" {
		workdir = cfg.WorkspaceFolder
	}
	if overrideCommandEnabled(cfg) {
		if imgMeta.Entrypoint != "" {
			// When a feature provides an entrypoint (e.g. docker-in-docker's
			// init script), the entrypoint script will 'exec "$@"' to run the
			// original container command. We must pass the full shell invocation
			// as arguments so the feature entrypoint can exec it.
			entrypoint = imgMeta.Entrypoint
			cmdArgs = []string{"/bin/sh", "-c", keepAliveScript, "-"}
		} else {
			entrypoint = "/bin/sh"
			cmdArgs = []string{"-c", keepAliveScript, "-"}
		}
	} else if imgMeta.Entrypoint != "" {
		entrypoint = imgMeta.Entrypoint
	}

	// Step 7: create and start the container via Docker CLI.
	containerID, err := createContainer(ctx, imageRef, runArgs, allMounts, envList, labels, user, workdir, entrypoint, cmdArgs)
	if err != nil {
		return "", fmt.Errorf("creating container: %w", err)
	}

	slog.Info("created container", "id", docker.ShortID(containerID))

	if err := startContainer(ctx, containerID); err != nil {
		return "", fmt.Errorf("starting container %s: %w", docker.ShortID(containerID), err)
	}

	slog.Info("started container", "id", docker.ShortID(containerID))

	// Step 8: run lifecycle commands for newly created containers.
	// Execution order per the devcontainer spec:
	//   onCreate (features) → onCreate (config) → postCreate (features) → postCreate (config) → postStart (features) → postStart (config)
	// If any command fails, subsequent commands in the chain are skipped.
	var chainErr error

	for _, lc := range imgMeta.OnCreateCommands {
		if chainErr = runLifecycleCommand(ctx, containerID, lc, cfg.WorkspaceFolder, "onCreateCommand"); chainErr != nil {
			break
		}
	}
	if chainErr == nil && !cfg.OnCreateCommand.IsEmpty() {
		chainErr = runLifecycleCommand(ctx, containerID, cfg.OnCreateCommand, cfg.WorkspaceFolder, "onCreateCommand")
	}

	if chainErr == nil {
		for _, lc := range imgMeta.UpdateContentCommands {
			if chainErr = runLifecycleCommand(ctx, containerID, lc, cfg.WorkspaceFolder, "updateContentCommand"); chainErr != nil {
				break
			}
		}
	}
	if chainErr == nil && !cfg.UpdateContentCommand.IsEmpty() {
		chainErr = runLifecycleCommand(ctx, containerID, cfg.UpdateContentCommand, cfg.WorkspaceFolder, "updateContentCommand")
	}

	if chainErr == nil {
		for _, lc := range imgMeta.PostCreateCommands {
			if chainErr = runLifecycleCommand(ctx, containerID, lc, cfg.WorkspaceFolder, "postCreateCommand"); chainErr != nil {
				break
			}
		}
	}
	if chainErr == nil {
		chainErr = runLifecycleCommand(ctx, containerID, cfg.PostCreateCommand, cfg.WorkspaceFolder, "postCreateCommand")
	}

	if chainErr == nil {
		for _, lc := range imgMeta.PostStartCommands {
			if chainErr = runLifecycleCommand(ctx, containerID, lc, cfg.WorkspaceFolder, "postStartCommand"); chainErr != nil {
				break
			}
		}
	}
	if chainErr == nil {
		chainErr = runLifecycleCommand(ctx, containerID, cfg.PostStartCommand, cfg.WorkspaceFolder, "postStartCommand")
	}

	if chainErr != nil {
		slog.Warn("lifecycle command failed, skipping remaining commands", "error", chainErr)
	}

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
			continue
		}
		if s, err := mountEntryObjectToString(entry); err == nil {
			mounts = append(mounts, s)
		}
	}

	return mounts
}

// mountEntryObjectToString converts a mount object (JSON map) into a Docker
// --mount format string. Keys are sorted alphabetically for determinism.
func mountEntryObjectToString(entry spec.MountEntry) (string, error) {
	var obj map[string]interface{}
	if err := json.Unmarshal(entry, &obj); err != nil {
		return "", err
	}
	if len(obj) == 0 {
		return "", fmt.Errorf("empty mount object")
	}
	var keys []string
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		switch v := obj[k].(type) {
		case string:
			parts = append(parts, fmt.Sprintf("%s=%s", k, v))
		case bool:
			parts = append(parts, fmt.Sprintf("%s=%t", k, v))
		case float64:
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
	}
	return strings.Join(parts, ","), nil
}

const devcontainerMetadataLabel = "devcontainer.metadata"

// metadataEntry is a single item inside the devcontainer.metadata label.
// It can represent either a feature's metadata or the devcontainer.json config
// metadata. The ID field is empty for config entries.
type metadataEntry struct {
	ID                   string                `json:"id,omitempty"`
	Init                 bool                  `json:"init,omitempty"`
	Privileged           bool                  `json:"privileged,omitempty"`
	CapAdd               []string              `json:"capAdd,omitempty"`
	SecurityOpt          []string              `json:"securityOpt,omitempty"`
	Entrypoint           string                `json:"entrypoint,omitempty"`
	Mounts               json.RawMessage       `json:"mounts,omitempty"`
	OnCreateCommand      spec.LifecycleCommand `json:"onCreateCommand,omitempty"`
	UpdateContentCommand spec.LifecycleCommand `json:"updateContentCommand,omitempty"`
	PostCreateCommand    spec.LifecycleCommand `json:"postCreateCommand,omitempty"`
	PostStartCommand     spec.LifecycleCommand `json:"postStartCommand,omitempty"`
	PostAttachCommand    spec.LifecycleCommand `json:"postAttachCommand,omitempty"`
	ContainerEnv         map[string]string     `json:"containerEnv,omitempty"`
	RemoteUser           string                `json:"remoteUser,omitempty"`
	ContainerUser        string                `json:"containerUser,omitempty"`
	WorkspaceFolder      string                `json:"workspaceFolder,omitempty"`
}

// imageMetadata holds properties collected from the devcontainer.metadata
// label that need to be applied to container creation and lifecycle
// execution. Values are aggregated across all feature entries in the label.
type imageMetadata struct {
	Init                  bool
	Privileged            bool
	CapAdd                []string
	SecurityOpt           []string
	Entrypoint            string
	Mounts                []spec.MountEntry
	OnCreateCommands      []spec.LifecycleCommand
	UpdateContentCommands []spec.LifecycleCommand
	PostCreateCommands    []spec.LifecycleCommand
	PostStartCommands     []spec.LifecycleCommand
	PostAttachCommands    []spec.LifecycleCommand
	ContainerEnv          map[string]string
}

// substitute replaces ${...} variable references in the metadata's mounts,
// entrypoint, lifecycle commands, and containerEnv values.
func (m *imageMetadata) substitute(absHostFolder, containerFolder string) {
	devcontainerID := computeDevcontainerID(absHostFolder)

	for i := range m.Mounts {
		m.Mounts[i] = substituteMountEntry(m.Mounts[i], absHostFolder, containerFolder, devcontainerID)
	}

	if m.Entrypoint != "" {
		m.Entrypoint = substituteString(m.Entrypoint, absHostFolder, containerFolder, devcontainerID)
	}

	for i := range m.OnCreateCommands {
		m.OnCreateCommands[i] = substituteLifecycleCommand(m.OnCreateCommands[i], absHostFolder, containerFolder, devcontainerID)
	}
	for i := range m.UpdateContentCommands {
		m.UpdateContentCommands[i] = substituteLifecycleCommand(m.UpdateContentCommands[i], absHostFolder, containerFolder, devcontainerID)
	}
	for i := range m.PostCreateCommands {
		m.PostCreateCommands[i] = substituteLifecycleCommand(m.PostCreateCommands[i], absHostFolder, containerFolder, devcontainerID)
	}
	for i := range m.PostStartCommands {
		m.PostStartCommands[i] = substituteLifecycleCommand(m.PostStartCommands[i], absHostFolder, containerFolder, devcontainerID)
	}
	for i := range m.PostAttachCommands {
		m.PostAttachCommands[i] = substituteLifecycleCommand(m.PostAttachCommands[i], absHostFolder, containerFolder, devcontainerID)
	}

	for k, v := range m.ContainerEnv {
		m.ContainerEnv[k] = substituteString(v, absHostFolder, containerFolder, devcontainerID)
	}
}

// substituteLifecycleCommand replaces variables in a lifecycle command. String
// commands are substituted directly; array commands have each element
// substituted. Object commands are left unchanged.
func substituteLifecycleCommand(lc spec.LifecycleCommand, absHostFolder, containerFolder, devcontainerID string) spec.LifecycleCommand {
	if lc.IsEmpty() {
		return lc
	}
	if s, ok := lc.AsString(); ok {
		return spec.NewLifecycleCommandString(substituteString(s, absHostFolder, containerFolder, devcontainerID))
	}
	if arr, ok := lc.AsArray(); ok {
		for i, arg := range arr {
			arr[i] = substituteString(arg, absHostFolder, containerFolder, devcontainerID)
		}
		return spec.NewLifecycleCommandArray(arr...)
	}
	return lc
}

// extractImageMetadata reads the devcontainer.metadata label from the given
// image and extracts feature properties that need to be applied at container
// creation time. Config metadata entries (those without an "id" field) are
// skipped.
func extractImageMetadata(ctx context.Context, cli docker.DockerClient, imageRef string) (imageMetadata, error) {
	var meta imageMetadata
	inspect, err := cli.ImageInspect(ctx, imageRef)
	if err != nil || inspect.Config == nil || inspect.Config.Labels == nil {
		return meta, nil
	}

	raw := inspect.Config.Labels[devcontainerMetadataLabel]
	if raw == "" {
		return meta, nil
	}

	var entries []metadataEntry
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		var single metadataEntry
		if err := json.Unmarshal([]byte(raw), &single); err != nil {
			return meta, nil
		}
		entries = []metadataEntry{single}
	}

	for _, e := range entries {
		if e.ID == "" {
			continue // skip config metadata entries
		}
		if e.Init {
			meta.Init = true
		}
		if e.Privileged {
			meta.Privileged = true
		}
		meta.CapAdd = append(meta.CapAdd, e.CapAdd...)
		meta.SecurityOpt = append(meta.SecurityOpt, e.SecurityOpt...)
		if e.Entrypoint != "" {
			meta.Entrypoint = e.Entrypoint
		}
		if len(e.Mounts) > 0 {
			var mounts []spec.MountEntry
			if err := json.Unmarshal(e.Mounts, &mounts); err == nil {
				meta.Mounts = append(meta.Mounts, mounts...)
			} else {
				var strMounts []string
				if err := json.Unmarshal(e.Mounts, &strMounts); err == nil {
					for _, s := range strMounts {
						meta.Mounts = append(meta.Mounts, spec.NewMountEntryString(s))
					}
				}
			}
		}
		if !e.OnCreateCommand.IsEmpty() {
			meta.OnCreateCommands = append(meta.OnCreateCommands, e.OnCreateCommand)
		}
		if !e.UpdateContentCommand.IsEmpty() {
			meta.UpdateContentCommands = append(meta.UpdateContentCommands, e.UpdateContentCommand)
		}
		if !e.PostCreateCommand.IsEmpty() {
			meta.PostCreateCommands = append(meta.PostCreateCommands, e.PostCreateCommand)
		}
		if !e.PostStartCommand.IsEmpty() {
			meta.PostStartCommands = append(meta.PostStartCommands, e.PostStartCommand)
		}
		if !e.PostAttachCommand.IsEmpty() {
			meta.PostAttachCommands = append(meta.PostAttachCommands, e.PostAttachCommand)
		}
		if len(e.ContainerEnv) > 0 {
			if meta.ContainerEnv == nil {
				meta.ContainerEnv = make(map[string]string)
			}
			for k, v := range e.ContainerEnv {
				meta.ContainerEnv[k] = v
			}
		}
	}

	// Deduplicate arrays.
	meta.CapAdd = uniqueStrings(meta.CapAdd)
	meta.SecurityOpt = uniqueStrings(meta.SecurityOpt)

	return meta, nil
}

// runPostStarts executes all postStartCommand lifecycle scripts contributed
// by features in installation order, followed by the config's postStartCommand.
// This is called after the container is started (either freshly created or
// an existing stopped container that was just started).
func (m *imageMetadata) runPostStarts(ctx context.Context, containerID string, cfg *spec.Config) {
	var chainErr error
	for _, lc := range m.PostStartCommands {
		if chainErr = runLifecycleCommand(ctx, containerID, lc, cfg.WorkspaceFolder, "postStartCommand"); chainErr != nil {
			break
		}
	}
	if chainErr == nil {
		chainErr = runLifecycleCommand(ctx, containerID, cfg.PostStartCommand, cfg.WorkspaceFolder, "postStartCommand")
	}
	if chainErr != nil {
		slog.Warn("postStartCommand failed", "error", chainErr)
	}
}

// buildMergedRunArgs combines user-provided runArgs with flags derived from
// feature metadata. Feature metadata flags are prepended to the user's runArgs
// so that explicit user values can override if the Docker CLI supports later
// options overriding earlier ones (for flags like --init this is irrelevant
// because Docker deduplicates internally).
func buildMergedRunArgs(baseRunArgs []string, meta imageMetadata) []string {
	var extra []string
	if meta.Init {
		extra = append(extra, "--init")
	}
	if meta.Privileged {
		extra = append(extra, "--privileged")
	}
	for _, c := range meta.CapAdd {
		extra = append(extra, "--cap-add", c)
	}
	for _, o := range meta.SecurityOpt {
		extra = append(extra, "--security-opt", o)
	}
	return append(extra, baseRunArgs...)
}

// uniqueStrings returns a new slice with duplicate strings removed while
// preserving the original order of first appearance.
func uniqueStrings(ss []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range ss {
		if seen[s] {
			continue
		}
		seen[s] = true
		result = append(result, s)
	}
	return result
}

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
