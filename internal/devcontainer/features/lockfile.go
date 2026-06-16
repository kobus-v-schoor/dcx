package features

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Lockfile is the parsed devcontainer-lock.json. It pins features to
// exact digests so that builds are reproducible.
type Lockfile struct {
	Features map[string]LockfileFeature `json:"features"`
}

// LockfileFeature is a single entry in the lockfile.
type LockfileFeature struct {
	Version   string   `json:"version"`
	Resolved  string   `json:"resolved"`
	Integrity string   `json:"integrity"`
	DependsOn []string `json:"dependsOn,omitempty"`
}

// SaveLockfile writes the lockfile to the workspace's .devcontainer directory.
func SaveLockfile(workspaceFolder string, lf *Lockfile) error {
	path := filepath.Join(workspaceFolder, ".devcontainer", "devcontainer-lock.json")
	data, err := json.MarshalIndent(lf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling lockfile: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing lockfile: %w", err)
	}
	return nil
}

// LoadLockfile reads the devcontainer-lock.json from the workspace's
// .devcontainer directory. If the file does not exist, it returns (nil, nil)
// so callers can proceed without a lockfile.
func LoadLockfile(workspaceFolder string) (*Lockfile, error) {
	path := filepath.Join(workspaceFolder, ".devcontainer", "devcontainer-lock.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading lockfile: %w", err)
	}
	var lf Lockfile
	if err := json.Unmarshal(data, &lf); err != nil {
		return nil, fmt.Errorf("parsing lockfile: %w", err)
	}
	return &lf, nil
}

// ResolveDigest looks up a feature (by its raw lowercase identifier) in
// the lockfile and returns the pinned manifest digest. The digest is
// extracted from the "resolved" field (e.g. "ghcr.io/.../feature@sha256:abc").
// If the feature is not pinned, an empty string is returned.
func (lf *Lockfile) ResolveDigest(rawID string) string {
	if lf == nil || len(lf.Features) == 0 {
		return ""
	}
	entry, ok := lf.Features[strings.ToLower(rawID)]
	if !ok {
		return ""
	}
	// Extract the digest from the resolved field. For OCI features it has
	// the form "registry/ns/id@sha256:<hex>".
	const prefix = "@sha256:"
	idx := strings.LastIndex(entry.Resolved, prefix)
	if idx < 0 {
		return ""
	}
	return "sha256:" + entry.Resolved[idx+len(prefix):]
}
