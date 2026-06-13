package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadExistingFile(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := `{"image": "test:latest", "workspaceFolder": "/workspace"}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(workspace, "")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Image != "test:latest" {
		t.Errorf("Image = %q, want test:latest", cfg.Image)
	}
	if cfg.WorkspaceFolder != "/workspace" {
		t.Errorf("WorkspaceFolder = %q, want /workspace", cfg.WorkspaceFolder)
	}
}

func TestLoadMissingFileWithDefaultImage(t *testing.T) {
	workspace := t.TempDir()

	cfg, err := Load(workspace, "mcr.microsoft.com/devcontainers/base:debian")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Image != "mcr.microsoft.com/devcontainers/base:debian" {
		t.Errorf("Image = %q, want mcr.microsoft.com/devcontainers/base:debian", cfg.Image)
	}
	if cfg.WorkspaceFolder != workspace {
		t.Errorf("WorkspaceFolder = %q, want %q", cfg.WorkspaceFolder, workspace)
	}
}

func TestLoadMissingFileWithoutDefaultImage(t *testing.T) {
	workspace := t.TempDir()

	_, err := Load(workspace, "")
	if err == nil {
		t.Fatal("expected error when devcontainer.json is missing and no default_image is set")
	}
	if !strings.Contains(err.Error(), "default_image is not configured") {
		t.Errorf("error message should mention default_image, got: %v", err)
	}
}

func TestLoadResolvesWorkspaceFolderDefault(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(`{"image": "test:latest"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(workspace, "")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.WorkspaceFolder != workspace {
		t.Errorf("WorkspaceFolder = %q, want %q", cfg.WorkspaceFolder, workspace)
	}
}

func TestLoadHandlesPolymorphicFields(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{
		"image": "test:latest",
		"build": "Dockerfile.dev",
		"dockerComposeFile": ["docker-compose.yml"],
		"postCreateCommand": "echo hello"
	}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(workspace, "")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Build == nil || cfg.Build.Dockerfile != "Dockerfile.dev" {
		t.Errorf("Build = %v, want &Build{Dockerfile: Dockerfile.dev}", cfg.Build)
	}
	if len(cfg.DockerComposeFile) != 1 || cfg.DockerComposeFile[0] != "docker-compose.yml" {
		t.Errorf("DockerComposeFile = %v, want [docker-compose.yml]", cfg.DockerComposeFile)
	}
	if cfg.PostCreateCommand != "echo hello" {
		t.Errorf("PostCreateCommand = %q, want echo hello", cfg.PostCreateCommand)
	}
}

func TestLoadWithOverrideDir(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	baseContent := `{"image": "base:latest", "name": "base"}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(baseContent), 0o644); err != nil {
		t.Fatal(err)
	}

	overrideDir := t.TempDir()
	overrideContent := `{"image": "override:latest"}`
	if err := os.WriteFile(filepath.Join(overrideDir, "devcontainer.json"), []byte(overrideContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadWithOverride(workspace, overrideDir, "")
	if err != nil {
		t.Fatalf("LoadWithOverride() error: %v", err)
	}
	if cfg.Image != "override:latest" {
		t.Errorf("Image = %q, want override:latest", cfg.Image)
	}
	if cfg.Name != "base" {
		t.Errorf("Name = %q, want base", cfg.Name)
	}
}

func TestLoadWithOverrideDirMissingOverride(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(`{"image": "base:latest"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadWithOverride(workspace, t.TempDir(), "")
	if err != nil {
		t.Fatalf("LoadWithOverride() error: %v", err)
	}
	if cfg.Image != "base:latest" {
		t.Errorf("Image = %q, want base:latest", cfg.Image)
	}
}

func TestLoadWithOverrideEmptyDir(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(`{"image": "base:latest"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadWithOverride(workspace, "", "")
	if err != nil {
		t.Fatalf("LoadWithOverride() error: %v", err)
	}
	if cfg.Image != "base:latest" {
		t.Errorf("Image = %q, want base:latest", cfg.Image)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(`{invalid}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(workspace, "")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "parsing devcontainer.json") {
		t.Errorf("expected parsing error, got: %v", err)
	}
}

func TestLoadInvalidOverrideJSON(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(`{"image": "base:latest"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	overrideDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(overrideDir, "devcontainer.json"), []byte(`{invalid}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadWithOverride(workspace, overrideDir, "")
	if err == nil {
		t.Fatal("expected error for invalid override JSON")
	}
	if !strings.Contains(err.Error(), "parsing override devcontainer.json") {
		t.Errorf("expected override parsing error, got: %v", err)
	}
}

func TestLoadReadsFileError(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create a non-readable file (best effort; may not work on all platforms).
	path := filepath.Join(devcontainerDir, "devcontainer.json")
	if err := os.WriteFile(path, []byte(`{"image": "test"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(path, 0o000)
	defer os.Chmod(path, 0o644)

	_, err := Load(workspace, "")
	if err == nil {
		// Permission-based tests are flaky in some environments; skip failure.
		t.Skip("permission test not supported in this environment")
	}
}
