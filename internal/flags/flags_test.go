package flags

import (
	"path/filepath"
	"testing"

	"github.com/kobus-v-schoor/dcx/internal/config"
)

func TestBuildBasicFlags(t *testing.T) {
	cfg := &config.Config{
		SSH: config.SSHConfig{ForwardAgent: true},
		Git: config.GitConfig{InjectConfigs: true, Configs: []string{"~/.gitconfig"}},
	}

	args := Build("/workspace", cfg, "/tmp/dcx-abc123")

	wantUp := "up"
	if args[0] != wantUp {
		t.Errorf("first arg = %q, want %q", args[0], wantUp)
	}

	foundWorkspaceFolder := false
	foundOverrideConfig := false
	for i, a := range args {
		if a == "--workspace-folder" && i+1 < len(args) {
			if args[i+1] != "/workspace" {
				t.Errorf("--workspace-folder value = %q, want %q", args[i+1], "/workspace")
			}
			foundWorkspaceFolder = true
		}
		if a == "--override-config" && i+1 < len(args) {
			expected := filepath.Join("/tmp/dcx-abc123", "devcontainer.json")
			if args[i+1] != expected {
				t.Errorf("--override-config value = %q, want %q", args[i+1], expected)
			}
			foundOverrideConfig = true
		}
	}

	if !foundWorkspaceFolder {
		t.Error("--workspace-folder flag not found")
	}
	if !foundOverrideConfig {
		t.Error("--override-config flag not found")
	}
}

func TestBuildWithFeatures(t *testing.T) {
	cfg := &config.Config{
		SSH: config.SSHConfig{ForwardAgent: true},
		Git: config.GitConfig{InjectConfigs: true},
		DefaultFeatures: []config.Feature{
			{ID: "ghcr.io/devcontainers/features/github-cli:1", Options: map[string]interface{}{"version": "latest"}},
		},
	}

	args := Build("/workspace", cfg, "/tmp/dcx-abc123")

	found := false
	for i, a := range args {
		if a == "--additional-features" && i+1 < len(args) {
			found = true
		}
	}
	if !found {
		t.Error("--additional-features flag not found in Build output")
	}
}

func TestBuildMountFlagsWithExistingSource(t *testing.T) {
	dir := t.TempDir()

	cfg := &config.Config{
		Mounts: []config.Mount{
			{Source: dir, Target: "/container/data", ReadOnly: true},
		},
	}

	args := Build("/workspace", cfg, "/tmp/dcx-abc123")

	found := false
	for i, a := range args {
		if a == "--mount" && i+1 < len(args) {
			found = true
		}
	}
	if !found {
		t.Error("--mount flag not found in Build output")
	}
}

func TestBuildMountFlagsNoMounts(t *testing.T) {
	cfg := &config.Config{}

	args := Build("/workspace", cfg, "/tmp/dcx-abc123")

	for _, a := range args {
		if a == "--mount" {
			t.Error("--mount flag should not be present when no mounts configured")
		}
	}
}
