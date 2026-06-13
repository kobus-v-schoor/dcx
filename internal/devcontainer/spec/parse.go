package spec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Load reads the project's devcontainer.json, applies an optional override
// config, and resolves default values. If devcontainer.json is missing and
// defaultImage is non-empty, a minimal Config containing only the image is
// generated instead of returning an error.
//
// The workspaceFolder is resolved from the parsed workspaceFolder property,
// falling back to the host workspaceFolder path when absent (matching the
// devcontainer CLI default behaviour).
func Load(workspaceFolder, defaultImage string) (*Config, error) {
	srcPath := filepath.Join(workspaceFolder, ".devcontainer", "devcontainer.json")

	data, err := os.ReadFile(srcPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading devcontainer.json: %w", err)
		}
		if defaultImage == "" {
			return nil, fmt.Errorf("no devcontainer.json found in %s and default_image is not configured", filepath.Join(workspaceFolder, ".devcontainer"))
		}
		// Generate a minimal spec so dcx up can run without a project
		// devcontainer.json.
		cfg := &Config{Image: defaultImage}
		resolveDefaults(cfg, workspaceFolder)
		return cfg, nil
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing devcontainer.json: %w", err)
	}

	resolveDefaults(&cfg, workspaceFolder)
	return &cfg, nil
}

// LoadWithOverride reads the project's devcontainer.json, merges an optional
// override config from overrideDir/devcontainer.json on top, and resolves
// defaults. It is a convenience wrapper around Load plus Merge.
func LoadWithOverride(workspaceFolder, overrideDir, defaultImage string) (*Config, error) {
	base, err := Load(workspaceFolder, defaultImage)
	if err != nil {
		return nil, err
	}

	if overrideDir == "" {
		return base, nil
	}

	overridePath := filepath.Join(overrideDir, "devcontainer.json")
	overrideData, err := os.ReadFile(overridePath)
	if err != nil {
		if os.IsNotExist(err) {
			return base, nil
		}
		return nil, fmt.Errorf("reading override devcontainer.json: %w", err)
	}

	var override Config
	if err := json.Unmarshal(overrideData, &override); err != nil {
		return nil, fmt.Errorf("parsing override devcontainer.json: %w", err)
	}

	return Merge(base, &override), nil
}

// resolveDefaults fills in default values for Config fields that the
// devcontainer spec defines defaults for. Currently this only handles
// workspaceFolder, which defaults to the host workspaceFolder path when
// absent.
func resolveDefaults(c *Config, workspaceFolder string) {
	if c.WorkspaceFolder == "" {
		c.WorkspaceFolder = workspaceFolder
	}
}
