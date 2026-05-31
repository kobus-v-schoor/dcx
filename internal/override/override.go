package override

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// OverrideDir represents a temporary directory containing an override
// devcontainer.json. It holds the parsed JSON config as a map so that
// multiple Inject functions can modify it without repeated parse/marshal
// cycles. Callers must call Save to persist changes to disk, and Close
// to clean up the temporary directory.
type OverrideDir struct {
	// Dir is the path to the temporary directory containing the override
	// devcontainer.json.
	Dir string
	// config holds the parsed devcontainer.json as a generic map. Inject
	// functions modify this map directly; Save marshals it back to disk.
	config map[string]json.RawMessage
	// ContainerWorkspaceFolder is the path to the workspace folder inside the
	// container, extracted from the devcontainer.json workspaceFolder property.
	// When the property is absent, it defaults to the host workspace folder
	// path because the devcontainer CLI mounts the workspace at the same host
	// path inside the container by default.
	ContainerWorkspaceFolder string
	// ContainerHomeDir is the home directory of the container's primary user,
	// derived from the remoteUser property in devcontainer.json. If remoteUser
	// is "root" the value is "/root"; for any other user it is "/home/<user>".
	// When remoteUser is absent from devcontainer.json, the field is empty unless
	// the image is from the Microsoft devcontainers registry
	// (mcr.microsoft.com/devcontainers/*), in which case it defaults to
	// "/home/vscode" because those images use the vscode user by default.
	ContainerHomeDir string
}

// Create reads the project's devcontainer.json, writes it into a temporary
// directory, and returns an OverrideDir with the config pre-parsed. If the
// workspace has no devcontainer.json and defaultImage is non-empty, a minimal
// spec containing only the image is generated instead. A fresh random
// directory is created on each invocation so that stale files from previous
// runs cannot affect the current one. Callers should defer Close() to clean
// up the temporary directory, and call Save() after all Inject calls to
// persist modifications to disk. Called by dcx up to prepare the override
// config before delegating to the devcontainer CLI.
func Create(workspaceFolder string, defaultImage string) (*OverrideDir, error) {
	srcPath := filepath.Join(workspaceFolder, ".devcontainer", "devcontainer.json")
	data, err := os.ReadFile(srcPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading devcontainer.json: %w", err)
		}
		if defaultImage == "" {
			return nil, fmt.Errorf("no devcontainer.json found in %s and default_image is not configured", filepath.Join(workspaceFolder, ".devcontainer"))
		}
		// Generate a minimal spec so dcx up can run without a project devcontainer.json.
		data = []byte(fmt.Sprintf(`{"image": %q}`, defaultImage))
	}

	dir, err := os.MkdirTemp("", "dcx-")
	if err != nil {
		return nil, fmt.Errorf("creating override directory: %w", err)
	}

	// Parse the JSON into a map once so Inject functions can modify it
	// without re-reading and re-parsing the file each time.
	var config map[string]json.RawMessage
	if err := json.Unmarshal(data, &config); err != nil {
		// Clean up the temp dir if parsing fails.
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("parsing devcontainer.json: %w", err)
	}

	// Extract the container-side workspace folder path. The devcontainer CLI
	// mounts the workspace at the host path inside the container by default,
	// so we fall back to the host workspaceFolder when the property is absent.
	containerWorkspaceFolder := workspaceFolder
	if raw, ok := config["workspaceFolder"]; ok {
		var wf string
		if err := json.Unmarshal(raw, &wf); err == nil && wf != "" {
			containerWorkspaceFolder = wf
		}
	}

	// Extract remoteUser and derive the container home directory.
	// remoteUser overrides the default user in the container image, and
	// therefore its associated home directory. When absent, the effective
	// user depends on image defaults which dcx cannot determine without
	// pulling the image, so ContainerHomeDir is left empty — except for
	// Microsoft devcontainer images (mcr.microsoft.com/devcontainers/*)
	// which default to the 'vscode' user.
	remoteUser := ""
	if raw, ok := config["remoteUser"]; ok {
		_ = json.Unmarshal(raw, &remoteUser)
	}
	if remoteUser == "" {
		image := ""
		if raw, ok := config["image"]; ok {
			_ = json.Unmarshal(raw, &image)
		}
		if strings.HasPrefix(image, "mcr.microsoft.com/devcontainers/") {
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
		if _, ok := config["remoteUser"]; !ok {
			config["remoteUser"] = json.RawMessage(fmt.Sprintf("%q", remoteUser))
		}
	}

	return &OverrideDir{
		Dir:                      dir,
		config:                   config,
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

// Save marshals the in-memory config back to the override devcontainer.json
// on disk. This must be called after all Inject functions have modified the
// config map to persist the changes. Called by dcx up after all injection
// steps are complete, before delegating to the devcontainer CLI.
func (o *OverrideDir) Save() error {
	configPath := filepath.Join(o.Dir, "devcontainer.json")

	updated, err := json.MarshalIndent(o.config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling override config: %w", err)
	}

	if err := os.WriteFile(configPath, updated, 0o644); err != nil {
		return fmt.Errorf("writing override devcontainer.json: %w", err)
	}

	return nil
}

// InjectMounts appends formatted mount strings to the in-memory config's mounts
// property. The devcontainer.json mounts property accepts Docker --mount format
// strings (e.g. "type=bind,source=/host/path,target=/container/path,readonly"),
// which support the full range of Docker mount options including readonly — unlike
// the devcontainer CLI's --mount flag which only accepts type, source, target,
// and external. If the config already has a mounts array, the new entries are
// appended. If mounts is absent, it is created. The caller must call Save to
// persist the change to disk. Called by dcx up after resolving all mount sources
// (user-configured, SSH agent, git config).
func (o *OverrideDir) InjectMounts(mountStrings []string) {
	if len(mountStrings) == 0 {
		return
	}

	// Decode existing mounts if present, otherwise start fresh.
	var existingMounts []string
	if raw, ok := o.config["mounts"]; ok {
		_ = json.Unmarshal(raw, &existingMounts)
	}

	existingMounts = append(existingMounts, mountStrings...)

	mountsJSON, err := json.Marshal(existingMounts)
	if err != nil {
		return
	}

	o.config["mounts"] = json.RawMessage(mountsJSON)
}

// InjectPostCreateCommand appends the provided shell command strings to the
// in-memory config's postCreateCommand property. If the base config already
// defines a postCreateCommand string, it is combined with the new commands
// using " && " so all commands run. This avoids relying on subtle devcontainer
// CLI merge semantics. The caller must call Save to persist the change to disk.
// Called by dcx up when injecting post-create commands from multiple sources.
func (o *OverrideDir) InjectPostCreateCommand(cmds []string) {
	if len(cmds) == 0 {
		return
	}

	// If the base config already has a postCreateCommand string, prepend it
	// to the new commands so all commands execute. We intentionally do not
	// try to merge with array or object forms (rare in practice).
	if raw, ok := o.config["postCreateCommand"]; ok {
		var existing string
		if err := json.Unmarshal(raw, &existing); err == nil {
			if existing != "" {
				cmds = append([]string{existing}, cmds...)
			}
		}
	}

	cmd := strings.Join(cmds, " && ")
	o.config["postCreateCommand"] = json.RawMessage(fmt.Sprintf("%q", cmd))
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

	// Decode existing containerEnv if present, otherwise start fresh.
	existingContainerEnv := make(map[string]string)
	if raw, ok := o.config["containerEnv"]; ok {
		if err := json.Unmarshal(raw, &existingContainerEnv); err != nil {
			// If the existing containerEnv is malformed, overwrite it entirely
			// rather than failing — the injected values take precedence.
			existingContainerEnv = make(map[string]string)
		}
	}

	// Merge new values on top; new values win on key conflict.
	for k, v := range envVars {
		existingContainerEnv[k] = v
	}

	containerEnvJSON, err := json.Marshal(existingContainerEnv)
	if err != nil {
		// Marshal of a map[string]string should never fail; log and skip.
		return
	}

	o.config["containerEnv"] = json.RawMessage(containerEnvJSON)
}
