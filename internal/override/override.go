package override

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

	return &OverrideDir{
		Dir:    dir,
		config: config,
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
