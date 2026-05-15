package override

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

const tempDirPattern = "dcx-"

// Create reads the project's devcontainer.json, writes it into a temporary
// directory, and returns the directory path along with a cleanup function.
// The temporary directory is located under os.TempDir() with a name derived
// from the workspace folder hash (dcx-<hash>). This ensures the same project
// always maps to the same temp directory, avoiding accumulation of stale
// directories across runs. Scope: filesystem. Called by dcx up to prepare the
// override config before delegating to the devcontainer CLI.
func Create(workspaceFolder string) (dir string, cleanup func(), err error) {
	srcPath := filepath.Join(workspaceFolder, ".devcontainer", "devcontainer.json")
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return "", nil, fmt.Errorf("reading devcontainer.json: %w", err)
	}

	hash := sha256.Sum256([]byte(filepath.Clean(workspaceFolder)))
	dirName := tempDirPattern + hex.EncodeToString(hash[:])[:12]
	dir = filepath.Join(os.TempDir(), dirName)

	if err := os.MkdirAll(dir, 0o755); err != nil {
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
