package flags

import (
	"path/filepath"
	"testing"

	"github.com/kobus-v-schoor/dcx/internal/config"
)

func TestBuildBasicFlags(t *testing.T) {
	cfg := &config.Config{
		SSHForwarding:       true,
		GitConfigForwarding: true,
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

func TestBuildReturnsUpSubcommand(t *testing.T) {
	cfg := &config.Config{}
	args := Build("/ws", cfg, "/tmp/override")

	if len(args) == 0 {
		t.Fatal("expected at least one argument")
	}
	if args[0] != "up" {
		t.Errorf("subcommand = %q, want %q", args[0], "up")
	}
}

func TestBuildOverrideConfigPath(t *testing.T) {
	cfg := &config.Config{}
	args := Build("/my/project", cfg, "/tmp/dcx-deadbeef")

	overridePath := filepath.Join("/tmp/dcx-deadbeef", "devcontainer.json")
	found := false
	for i, a := range args {
		if a == "--override-config" && i+1 < len(args) && args[i+1] == overridePath {
			found = true
		}
	}
	if !found {
		t.Errorf("--override-config %s not found in args: %v", overridePath, args)
	}
}

func TestFormatRemoteEnv(t *testing.T) {
	got := FormatRemoteEnv("MY_VAR", "hello")
	want := "MY_VAR=hello"
	if got != want {
		t.Errorf("FormatRemoteEnv() = %q, want %q", got, want)
	}
}

func TestBuildPlaceholderFunctionsReturnNil(t *testing.T) {
	cfg := &config.Config{}
	if buildMounts(cfg) != nil {
		t.Error("buildMounts should return nil (placeholder)")
	}
	if buildRemoteEnv(cfg) != nil {
		t.Error("buildRemoteEnv should return nil (placeholder)")
	}
}

func TestBuildAdditionalFeaturesEmpty(t *testing.T) {
	cfg := &config.Config{}
	if buildAdditionalFeatures(cfg) != nil {
		t.Error("buildAdditionalFeatures should return nil when no features configured")
	}
}

func TestBuildAdditionalFeaturesWithFeatures(t *testing.T) {
	cfg := &config.Config{
		DefaultFeatures: []config.Feature{
			{ID: "ghcr.io/devcontainers/features/github-cli:1", Options: map[string]interface{}{"version": "latest"}},
		},
	}

	args := buildAdditionalFeatures(cfg)

	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(args), args)
	}
	if args[0] != "--additional-features" {
		t.Errorf("flag = %q, want --additional-features", args[0])
	}
	if args[1] == "" {
		t.Error("additional-features value should not be empty")
	}
}

func TestBuildWithFeatures(t *testing.T) {
	cfg := &config.Config{
		SSHForwarding:       true,
		GitConfigForwarding: true,
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
