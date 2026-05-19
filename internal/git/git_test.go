package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kobus-v-schoor/dcx/internal/config"
)

func TestDetectConfigsSingleFile(t *testing.T) {
	home := t.TempDir()
	gitconfigPath := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(gitconfigPath, []byte("[user]\n  name = test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", home)

	cfg := config.GitConfig{
		InjectConfigs: true,
		Configs:       []string{"~/.gitconfig"},
		MountBase:     "/opt/dcx/git",
	}

	result := DetectConfigs(cfg)

	if len(result.Mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(result.Mounts))
	}
	if result.Mounts[0].Source != gitconfigPath {
		t.Errorf("Mounts[0].Source = %q, want %q", result.Mounts[0].Source, gitconfigPath)
	}
	if result.Mounts[0].Target != "/opt/dcx/git/0-.gitconfig" {
		t.Errorf("Mounts[0].Target = %q, want /opt/dcx/git/0-.gitconfig", result.Mounts[0].Target)
	}
	if !result.Mounts[0].ReadOnly {
		t.Error("Mounts[0].ReadOnly should be true for git configs")
	}
	if result.EnvName != "GIT_CONFIG_GLOBAL" {
		t.Errorf("EnvName = %q, want GIT_CONFIG_GLOBAL", result.EnvName)
	}
	if result.EnvValue != "/opt/dcx/git/0-.gitconfig" {
		t.Errorf("EnvValue = %q, want /opt/dcx/git/0-.gitconfig", result.EnvValue)
	}
}

func TestDetectConfigsDisabled(t *testing.T) {
	cfg := config.GitConfig{
		InjectConfigs: false,
		Configs:       []string{"~/.gitconfig"},
		MountBase:     "/opt/dcx/git",
	}

	result := DetectConfigs(cfg)

	if len(result.Mounts) != 0 {
		t.Errorf("expected 0 mounts when disabled, got %d", len(result.Mounts))
	}
	if result.EnvName != "" {
		t.Errorf("EnvName = %q, want empty when disabled", result.EnvName)
	}
}

func TestDetectConfigsEmptyConfigs(t *testing.T) {
	cfg := config.GitConfig{
		InjectConfigs: true,
		Configs:       nil,
		MountBase:     "/opt/dcx/git",
	}

	result := DetectConfigs(cfg)

	if len(result.Mounts) != 0 {
		t.Errorf("expected 0 mounts with empty configs, got %d", len(result.Mounts))
	}
	if result.EnvName != "" {
		t.Errorf("EnvName = %q, want empty when configs is empty", result.EnvName)
	}
}

func TestDetectConfigsMissingFile(t *testing.T) {
	t.Setenv("HOME", "/nonexistent/home")

	cfg := config.GitConfig{
		InjectConfigs: true,
		Configs:       []string{"~/.gitconfig"},
		MountBase:     "/opt/dcx/git",
	}

	result := DetectConfigs(cfg)

	if len(result.Mounts) != 0 {
		t.Errorf("expected 0 mounts for missing file, got %d", len(result.Mounts))
	}
	if result.EnvName != "" {
		t.Errorf("EnvName = %q, want empty when no files found", result.EnvName)
	}
}

func TestDetectConfigsMultipleFiles(t *testing.T) {
	home := t.TempDir()

	gitconfigPath := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(gitconfigPath, []byte("[user]\n  name = test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	gitignorePath := filepath.Join(home, ".gitignore_global")
	if err := os.WriteFile(gitignorePath, []byte("*.swp\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", home)

	cfg := config.GitConfig{
		InjectConfigs: true,
		Configs:       []string{"~/.gitconfig", "~/.gitignore_global"},
		MountBase:     "/opt/dcx/git",
	}

	result := DetectConfigs(cfg)

	if len(result.Mounts) != 2 {
		t.Fatalf("expected 2 mounts, got %d", len(result.Mounts))
	}
	if result.Mounts[0].Target != "/opt/dcx/git/0-.gitconfig" {
		t.Errorf("Mounts[0].Target = %q, want /opt/dcx/git/0-.gitconfig", result.Mounts[0].Target)
	}
	if result.Mounts[1].Target != "/opt/dcx/git/1-.gitignore_global" {
		t.Errorf("Mounts[1].Target = %q, want /opt/dcx/git/1-.gitignore_global", result.Mounts[1].Target)
	}
	if result.EnvName != "GIT_CONFIG_GLOBAL" {
		t.Errorf("EnvName = %q, want GIT_CONFIG_GLOBAL", result.EnvName)
	}
}

func TestDetectConfigsSomeFilesMissing(t *testing.T) {
	home := t.TempDir()

	gitconfigPath := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(gitconfigPath, []byte("[user]\n  name = test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", home)

	cfg := config.GitConfig{
		InjectConfigs: true,
		Configs:       []string{"~/.gitconfig", "~/.gitignore_global"},
		MountBase:     "/opt/dcx/git",
	}

	result := DetectConfigs(cfg)

	if len(result.Mounts) != 1 {
		t.Fatalf("expected 1 mount (missing file skipped), got %d", len(result.Mounts))
	}
	if result.EnvName != "GIT_CONFIG_GLOBAL" {
		t.Errorf("EnvName = %q, want GIT_CONFIG_GLOBAL", result.EnvName)
	}
}
