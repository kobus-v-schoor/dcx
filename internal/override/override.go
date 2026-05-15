package override

import (
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
