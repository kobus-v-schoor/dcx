package spec

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

// goos is a package-level variable shadowing runtime.GOOS so that platform
// checks can be overridden in unit tests.
var goos = runtime.GOOS

// ResolveWorkspaceFolder returns the container-side workspace folder path.
// When cfg.WorkspaceFolder is non-empty it is returned verbatim. Otherwise,
// the absolute path of hostWorkspaceFolder is returned, matching the
// devcontainer CLI default behaviour.
func ResolveWorkspaceFolder(cfg *Config, hostWorkspaceFolder string) string {
	if cfg.WorkspaceFolder != "" {
		return cfg.WorkspaceFolder
	}
	abs, err := filepath.Abs(hostWorkspaceFolder)
	if err == nil {
		return abs
	}
	return hostWorkspaceFolder
}

// ResolveWorkspaceMount returns the Docker --mount format string for the
// workspace. The rules are:
//
//  1. If cfg.WorkspaceMount is non-empty it is validated and returned verbatim.
//  2. If cfg.WorkspaceMount is empty a default bind mount is synthesised from
//     the host workspace folder and resolved workspace folder.
//
// On macOS the default mount includes "consistency=cached"; on Linux it is
// omitted.
func ResolveWorkspaceMount(cfg *Config, hostWorkspaceFolder string) (string, error) {
	if cfg.WorkspaceMount != "" {
		if err := validateMountString(cfg.WorkspaceMount); err != nil {
			return "", fmt.Errorf("invalid workspaceMount: %w", err)
		}
		return cfg.WorkspaceMount, nil
	}

	workspaceFolder := ResolveWorkspaceFolder(cfg, hostWorkspaceFolder)
	absHost, err := filepath.Abs(hostWorkspaceFolder)
	if err != nil {
		absHost = hostWorkspaceFolder
	}

	mount := fmt.Sprintf("type=bind,source=%s,target=%s", absHost, workspaceFolder)
	if goos == "darwin" {
		mount += ",consistency=cached"
	}
	return mount, nil
}

// validateMountString parses a Docker --mount format string and ensures the
// required fields type, source, and target are present.
func validateMountString(mount string) error {
	parts := strings.Split(mount, ",")
	hasType := false
	hasSource := false
	hasTarget := false

	for _, part := range parts {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		switch key {
		case "type":
			hasType = true
		case "source":
			hasSource = true
		case "target":
			hasTarget = true
		}
	}

	if !hasType {
		return fmt.Errorf("mount string missing required field 'type'")
	}
	if !hasSource {
		return fmt.Errorf("mount string missing required field 'source'")
	}
	if !hasTarget {
		return fmt.Errorf("mount string missing required field 'target'")
	}
	return nil
}
