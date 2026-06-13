package override

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kobus-v-schoor/dcx/internal/devcontainer/spec"
)

// OverrideDir represents a temporary directory containing an override
// devcontainer.json. It holds the parsed devcontainer configuration as a
// typed spec.Config so that multiple Inject functions can modify it without
// repeated parse/marshal cycles. Callers must call Save to persist changes to
// disk, and Close to clean up the temporary directory.
type OverrideDir struct {
	// Dir is the path to the temporary directory containing the override
	// devcontainer.json.
	Dir string

	// Config is the in-memory typed representation of the override
	// devcontainer.json. Inject functions modify this struct directly;
	// Save marshals it back to disk.
	Config *spec.Config

	// ContainerWorkspaceFolder is the path to the workspace folder inside the
	// container, extracted from the devcontainer.json workspaceFolder property.
	// When the property is absent, it defaults to the host workspace folder
	// path because the devcontainer CLI mounts the workspace at the same host
	// path inside the container by default.
	ContainerWorkspaceFolder string

	// ContainerHomeDir is the home directory of the container's primary user,
	// derived from the remoteUser property in devcontainer.json. If remoteUser
	// is "root" the value is "/root"; for any other user it is "/home/<user>".
	// When remoteUser is absent from devcontainer.json, the field is empty
	// unless the image is from the Microsoft devcontainers registry
	// (mcr.microsoft.com/devcontainers/*), in which case it defaults to
	// "/home/vscode" because those images use the vscode user by default.
	ContainerHomeDir string
}

// Create reads the project's devcontainer.json, writes it into a temporary
// directory, and returns an OverrideDir with the config pre-parsed as a typed
// spec.Config. If the workspace has no devcontainer.json and defaultImage is
// non-empty, a minimal spec containing only the image is generated instead. A
// fresh random directory is created on each invocation so that stale files from
// previous runs cannot affect the current one. Callers should defer Close() to
// clean up the temporary directory, and call Save() after all Inject calls to
// persist modifications to disk. Called by dcx up to prepare the override
// config before delegating to the devcontainer CLI.
func Create(workspaceFolder string, defaultImage string) (*OverrideDir, error) {
	dir, err := os.MkdirTemp("", "dcx-")
	if err != nil {
		return nil, fmt.Errorf("creating override directory: %w", err)
	}

	cfg, err := spec.Load(workspaceFolder, defaultImage)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}

	// Extract the container-side workspace folder path. The devcontainer CLI
	// mounts the workspace at the host path inside the container by default,
	// so we fall back to the host workspaceFolder when the property is absent.
	containerWorkspaceFolder := cfg.WorkspaceFolder
	if containerWorkspaceFolder == "" {
		containerWorkspaceFolder = workspaceFolder
	}

	// Extract remoteUser and derive the container home directory.
	// remoteUser overrides the default user in the container image, and
	// therefore its associated home directory. When absent, the effective
	// user depends on image defaults which dcx cannot determine without
	// pulling the image, so ContainerHomeDir is left empty — except for
	// Microsoft devcontainer images which default to the 'vscode' user.
	remoteUser := cfg.RemoteUser
	if remoteUser == "" {
		if strings.HasPrefix(cfg.Image, "mcr.microsoft.com/devcontainers/") {
			remoteUser = "vscode"
		}
	}
	containerHomeDir := ""
	if remoteUser != "" {
		if remoteUser == "root" {
			containerHomeDir = "/root"
		} else {
			containerHomeDir = "/home/" + remoteUser
		}
		// If remoteUser was absent from the original devcontainer.json and
		// defaulted based on the image, inject it into the override config so
		// the devcontainer CLI applies the correct user.
		if cfg.RemoteUser == "" {
			cfg.RemoteUser = remoteUser
		}
	}

	return &OverrideDir{
		Dir:                      dir,
		Config:                   cfg,
		ContainerWorkspaceFolder: containerWorkspaceFolder,
		ContainerHomeDir:         containerHomeDir,
	}, nil
}

// Close removes the temporary directory and all its contents. Should be
// called (typically via defer) when the override directory is no longer
// needed.
func (o *OverrideDir) Close() {
	_ = os.RemoveAll(o.Dir)
}

// Save marshals the in-memory typed config back to the override
// devcontainer.json on disk. This must be called after all Inject functions
// have modified the config to persist the changes. Called by dcx up after
// all injection steps are complete, before delegating to the devcontainer CLI.
func (o *OverrideDir) Save() error {
	if o.Config == nil {
		return fmt.Errorf("no config to save")
	}

	configPath := filepath.Join(o.Dir, "devcontainer.json")

	updated, err := json.MarshalIndent(o.Config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling override config: %w", err)
	}

	if err := os.WriteFile(configPath, updated, 0o644); err != nil {
		return fmt.Errorf("writing override devcontainer.json: %w", err)
	}

	return nil
}

// InjectMounts appends formatted mount strings to the in-memory config's
// mounts property. The devcontainer.json mounts property accepts Docker
// --mount format strings (e.g.
// "type=bind,source=/host/path,target=/container/path,readonly"), which
// support the full range of Docker mount options including readonly — unlike
// the devcontainer CLI's --mount flag which only accepts type, source,
// target, and external. If the config already has a mounts array, the new
// entries are appended. If mounts is absent, it is created. The caller must
// call Save to persist the change to disk. Called by dcx up after resolving
// all mount sources (user-configured, SSH agent, git config).
func (o *OverrideDir) InjectMounts(mountStrings []string) {
	if len(mountStrings) == 0 {
		return
	}

	o.Config.Mounts = append(o.Config.Mounts, mountStrings...)
}

// InjectPostCreateCommand appends the provided shell command strings to the
// in-memory config's postCreateCommand property. If the base config already
// defines a postCreateCommand string, it is combined with the new commands
// using " && " so all commands run. If the base config defined an array or
// object form, the new commands replace it entirely. The caller must call
// Save to persist the change to disk. Called by dcx up when injecting
// post-create commands from multiple sources.
func (o *OverrideDir) InjectPostCreateCommand(cmds []string) {
	if len(cmds) == 0 {
		return
	}

	if o.Config.PostCreateCommand != "" {
		cmds = append([]string{o.Config.PostCreateCommand}, cmds...)
	}

	o.Config.PostCreateCommand = strings.Join(cmds, " && ")
}

// InjectContainerEnv merges the provided environment variables into the
// in-memory config's containerEnv property. If the config already has a
// containerEnv object, the new values are merged on top (new values win on
// key conflict). If containerEnv is absent, it is created. containerEnv sets
// Docker-level environment variables that are persistent in the running
// container (visible via env, docker exec, etc.), unlike remoteEnv which only
// applies to VS Code server processes. The caller must call Save to persist
// the change to disk. Called by dcx up after resolving all env vars
// (user-configured, SSH agent, git config).
func (o *OverrideDir) InjectContainerEnv(envVars map[string]string) {
	if len(envVars) == 0 {
		return
	}

	if o.Config.ContainerEnv == nil {
		o.Config.ContainerEnv = make(map[string]string)
	}

	for k, v := range envVars {
		o.Config.ContainerEnv[k] = v
	}
}
