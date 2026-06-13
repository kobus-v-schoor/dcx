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

// WorkspaceMount holds the resolved workspace mount configuration.
// It is independent of any specific orchestrator format so that it can be
// used both when delegating to the devcontainer CLI and when creating
// containers directly via the Docker API.
type WorkspaceMount struct {
	Type   string // e.g. "bind", "volume", "tmpfs"
	Source string
	Target string
	// Options stores any additional comma-separated parts of the mount string
	// that are not type, source, or target (e.g. "readonly", "consistency=cached").
	Options []string
}

// ResolveWorkspaceMount returns the resolved workspace mount.
// The rules are:
//
//  1. If cfg.WorkspaceMount is non-empty it is parsed into a WorkspaceMount.
//  2. If cfg.WorkspaceMount is empty a default bind mount is synthesised from
//     the host workspace folder and resolved workspace folder.
//
// On macOS the default mount includes "consistency=cached"; on Linux it is
// omitted.
func ResolveWorkspaceMount(cfg *Config, hostWorkspaceFolder string) (*WorkspaceMount, error) {
	if cfg.WorkspaceMount != "" {
		return parseMountString(cfg.WorkspaceMount)
	}

	workspaceFolder := ResolveWorkspaceFolder(cfg, hostWorkspaceFolder)
	absHost, err := filepath.Abs(hostWorkspaceFolder)
	if err != nil {
		absHost = hostWorkspaceFolder
	}

	wm := &WorkspaceMount{
		Type:   "bind",
		Source: absHost,
		Target: workspaceFolder,
	}
	if goos == "darwin" {
		wm.Options = []string{"consistency=cached"}
	}
	return wm, nil
}

// parseMountString parses a Docker --mount format string into a
// WorkspaceMount. It ensures the required fields type, source, and target
// are present.
func parseMountString(mount string) (*WorkspaceMount, error) {
	parts := strings.Split(mount, ",")
	wm := &WorkspaceMount{}

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		key := kv[0]
		switch key {
		case "type":
			if len(kv) == 2 {
				wm.Type = strings.TrimSpace(kv[1])
			}
		case "source":
			if len(kv) == 2 {
				wm.Source = strings.TrimSpace(kv[1])
			}
		case "target":
			if len(kv) == 2 {
				wm.Target = strings.TrimSpace(kv[1])
			}
		default:
			wm.Options = append(wm.Options, part)
		}
	}

	if wm.Type == "" {
		return nil, fmt.Errorf("mount string missing required field 'type'")
	}
	if wm.Source == "" {
		return nil, fmt.Errorf("mount string missing required field 'source'")
	}
	if wm.Target == "" {
		return nil, fmt.Errorf("mount string missing required field 'target'")
	}
	return wm, nil
}
