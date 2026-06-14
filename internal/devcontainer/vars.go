package devcontainer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kobus-v-schoor/dcx/internal/devcontainer/spec"
)

// varPattern matches ${...} substitution references in devcontainer.json strings.
// It captures the inner content between the braces.
var varPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// SubstituteAll resolves ${...} references in the merged spec.Config in place.
// It processes workspaceMount, mounts entries, containerEnv values, runArgs strings,
// and the workspaceFolder field. The hostWorkspaceFolder must be an absolute path.
//
// Supported patterns:
//   - ${localWorkspaceFolder} → absHostFolder
//   - ${localWorkspaceFolderBasename} → basename(absHostFolder)
//   - ${containerWorkspaceFolder} → cfg.WorkspaceFolder
//   - ${containerWorkspaceFolderBasename} → basename(cfg.WorkspaceFolder)
//   - ${localEnv:NAME} → os.Getenv("NAME") (empty if unset)
//   - ${localEnv:NAME:DEFAULT} → os.Getenv("NAME") or "DEFAULT" if unset
//   - ${devcontainerId} → stable SHA256 hex hash of absHostFolder, truncated to 32 chars
func SubstituteAll(cfg *spec.Config, hostWorkspaceFolder string) error {
	absHostFolder, err := filepath.Abs(hostWorkspaceFolder)
	if err != nil {
		return fmt.Errorf("resolving host workspace folder: %w", err)
	}

	devcontainerID := computeDevcontainerID(absHostFolder)

	// Substitute in workspaceFolder.
	if cfg.WorkspaceFolder != "" {
		cfg.WorkspaceFolder = substituteString(cfg.WorkspaceFolder, absHostFolder, cfg.WorkspaceFolder, devcontainerID)
	}

	// Substitute in workspaceMount.
	if cfg.WorkspaceMount != "" {
		cfg.WorkspaceMount = substituteString(cfg.WorkspaceMount, absHostFolder, cfg.WorkspaceFolder, devcontainerID)
	}

	// Substitute in mounts.
	for i, m := range cfg.Mounts {
		if s, ok := m.AsString(); ok {
			cfg.Mounts[i] = spec.NewMountEntryString(substituteString(s, absHostFolder, cfg.WorkspaceFolder, devcontainerID))
		}
	}

	// Substitute in containerEnv values.
	for k, v := range cfg.ContainerEnv {
		cfg.ContainerEnv[k] = substituteString(v, absHostFolder, cfg.WorkspaceFolder, devcontainerID)
	}

	// Substitute in runArgs.
	for i, arg := range cfg.RunArgs {
		cfg.RunArgs[i] = substituteString(arg, absHostFolder, cfg.WorkspaceFolder, devcontainerID)
	}

	return nil
}

// substituteString replaces all ${...} references in s with their resolved
// values. Unknown patterns are left untouched for forward compatibility.
func substituteString(s, absHostFolder, containerFolder, devcontainerID string) string {
	return varPattern.ReplaceAllStringFunc(s, func(match string) string {
		inner := match[2 : len(match)-1] // strip ${ and }
		return resolveVar(inner, absHostFolder, containerFolder, devcontainerID)
	})
}

// resolveVar maps a single variable name to its value.
func resolveVar(inner, absHostFolder, containerFolder, devcontainerID string) string {
	switch inner {
	case "localWorkspaceFolder":
		return absHostFolder
	case "localWorkspaceFolderBasename":
		return filepath.Base(absHostFolder)
	case "containerWorkspaceFolder":
		return containerFolder
	case "containerWorkspaceFolderBasename":
		return filepath.Base(containerFolder)
	case "devcontainerId":
		return devcontainerID
	default:
		// Check for localEnv:NAME or localEnv:NAME:DEFAULT
		if strings.HasPrefix(inner, "localEnv:") {
			rest := inner[len("localEnv:"):]
			name, defVal, hasDefault := strings.Cut(rest, ":")
			val := os.Getenv(name)
			if val == "" && hasDefault {
				return defVal
			}
			return val
		}
	}
	// Unknown pattern: return unchanged for forward compatibility.
	return "${" + inner + "}"
}

// computeDevcontainerID returns a stable SHA256 hex hash of the host workspace
// folder path, truncated to 32 characters. This matches the devcontainer CLI's
// stable identifier generation.
func computeDevcontainerID(absHostFolder string) string {
	h := sha256.Sum256([]byte(absHostFolder))
	return hex.EncodeToString(h[:])[:32]
}
