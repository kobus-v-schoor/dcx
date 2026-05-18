package override

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Create reads the project's devcontainer.json, writes it into a temporary
// directory, and returns the directory path along with a cleanup function.
// A fresh random directory is created on each invocation so that stale files
// from previous runs cannot affect the current one. Called by dcx up to
// prepare the override config before delegating to the devcontainer CLI.
func Create(workspaceFolder string) (dir string, cleanup func(), err error) {
	srcPath := filepath.Join(workspaceFolder, ".devcontainer", "devcontainer.json")
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return "", nil, fmt.Errorf("reading devcontainer.json: %w", err)
	}

	dir, err = os.MkdirTemp("", "dcx-")
	if err != nil {
		return "", nil, fmt.Errorf("creating override directory: %w", err)
	}

	dstPath := filepath.Join(dir, "devcontainer.json")
	if err := os.WriteFile(dstPath, data, 0o644); err != nil {
		return "", nil, fmt.Errorf("writing override devcontainer.json: %w", err)
	}

	cleanup = func() {
		_ = os.RemoveAll(dir)
	}

	return dir, cleanup, nil
}

// InjectContainerEnv reads the override devcontainer.json, merges the provided
// environment variables into its containerEnv property, and writes it back. If
// the file already has a containerEnv object, the new values are merged on top
// (new values win on key conflict). If containerEnv is absent, it is created.
// containerEnv sets Docker-level environment variables that are persistent in
// the running container (visible via env, docker exec, etc.), unlike remoteEnv
// which only applies to VS Code server processes. Called by dcx up after
// creating the override directory and resolving all env vars (user-configured,
// SSH agent, git config).
func InjectContainerEnv(dir string, envVars map[string]string) error {
	if len(envVars) == 0 {
		return nil
	}

	configPath := filepath.Join(dir, "devcontainer.json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("reading override config for containerEnv injection: %w", err)
	}

	// Parse into a generic map so we preserve any existing keys and formatting
	// is controlled by our marshal call.
	var config map[string]json.RawMessage
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("parsing override config for containerEnv injection: %w", err)
	}

	// Decode existing containerEnv if present, otherwise start fresh.
	existingContainerEnv := make(map[string]string)
	if raw, ok := config["containerEnv"]; ok {
		if err := json.Unmarshal(raw, &existingContainerEnv); err != nil {
			return fmt.Errorf("parsing existing containerEnv: %w", err)
		}
	}

	// Merge new values on top; new values win on key conflict.
	for k, v := range envVars {
		existingContainerEnv[k] = v
	}

	containerEnvJSON, err := json.Marshal(existingContainerEnv)
	if err != nil {
		return fmt.Errorf("marshalling containerEnv: %w", err)
	}

	config["containerEnv"] = json.RawMessage(containerEnvJSON)

	updated, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling updated override config: %w", err)
	}

	if err := os.WriteFile(configPath, updated, 0o644); err != nil {
		return fmt.Errorf("writing updated override config: %w", err)
	}

	return nil
}
