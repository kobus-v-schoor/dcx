package override

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateWritesOverride(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}

	original := `{"image": "test:latest"}`
	srcPath := filepath.Join(devcontainerDir, "devcontainer.json")
	if err := os.WriteFile(srcPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	dir, cleanup, err := Create(workspace)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer cleanup()

	overrideData, err := os.ReadFile(filepath.Join(dir, "devcontainer.json"))
	if err != nil {
		t.Fatalf("reading override file: %v", err)
	}

	if string(overrideData) != original {
		t.Errorf("override content = %q, want %q", string(overrideData), original)
	}
}

func TestCreateMissingDevcontainerJSON(t *testing.T) {
	workspace := t.TempDir()

	_, _, err := Create(workspace)
	if err == nil {
		t.Fatal("expected error when devcontainer.json is missing")
	}
}

func TestCreateCleanupRemovesDir(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	dir, cleanup, err := Create(workspace)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatal("override dir should exist before cleanup")
	}

	cleanup()

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("override dir should be removed after cleanup")
	}
}

func TestCreateRandomDirName(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	dir1, cleanup1, err := Create(workspace)
	if err != nil {
		t.Fatalf("Create() first call error: %v", err)
	}
	cleanup1()

	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	dir2, cleanup2, err := Create(workspace)
	if err != nil {
		t.Fatalf("Create() second call error: %v", err)
	}
	cleanup2()

	if dir1 == dir2 {
		t.Errorf("dir names should be random across invocations: both %q", dir1)
	}
}

func TestCreateDirNamePrefix(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	dir, cleanup, err := Create(workspace)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer cleanup()

	base := filepath.Base(dir)
	if len(base) < 5 || base[:4] != "dcx-" {
		t.Errorf("dir base name should start with dcx-, got %q", base)
	}
}
