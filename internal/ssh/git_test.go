package ssh

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kobus-v-schoor/dcx/internal/config"
)

func TestDetectGitConfigsSingleFile(t *testing.T) {
	home := t.TempDir()
	gitconfigPath := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(gitconfigPath, []byte("[user]\n  name = test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", home)

	cfg := config.GitConfig{
		InjectConfigs: true,
		Configs:       []string{"~/.gitconfig"},
	}

	result := DetectGitConfigs(cfg)

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

func TestDetectGitConfigsDisabled(t *testing.T) {
	cfg := config.GitConfig{
		InjectConfigs: false,
		Configs:       []string{"~/.gitconfig"},
	}

	result := DetectGitConfigs(cfg)

	if len(result.Mounts) != 0 {
		t.Errorf("expected 0 mounts when disabled, got %d", len(result.Mounts))
	}
	if result.EnvName != "" {
		t.Errorf("EnvName = %q, want empty when disabled", result.EnvName)
	}
}

func TestDetectGitConfigsEmptyConfigs(t *testing.T) {
	cfg := config.GitConfig{
		InjectConfigs: true,
		Configs:       nil,
	}

	result := DetectGitConfigs(cfg)

	if len(result.Mounts) != 0 {
		t.Errorf("expected 0 mounts with empty configs, got %d", len(result.Mounts))
	}
	if result.EnvName != "" {
		t.Errorf("EnvName = %q, want empty when configs is empty", result.EnvName)
	}
}

func TestDetectGitConfigsMissingFile(t *testing.T) {
	t.Setenv("HOME", "/nonexistent/home")

	cfg := config.GitConfig{
		InjectConfigs: true,
		Configs:       []string{"~/.gitconfig"},
	}

	result := DetectGitConfigs(cfg)

	if len(result.Mounts) != 0 {
		t.Errorf("expected 0 mounts for missing file, got %d", len(result.Mounts))
	}
	if result.EnvName != "" {
		t.Errorf("EnvName = %q, want empty when no files found", result.EnvName)
	}
}

func TestDetectGitConfigsMultipleFiles(t *testing.T) {
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
	}

	result := DetectGitConfigs(cfg)

	if len(result.Mounts) != 2 {
		t.Fatalf("expected 2 mounts, got %d", len(result.Mounts))
	}
	if result.Mounts[0].Source != gitconfigPath {
		t.Errorf("Mounts[0].Source = %q, want %q", result.Mounts[0].Source, gitconfigPath)
	}
	if result.Mounts[0].Target != "/opt/dcx/git/0-.gitconfig" {
		t.Errorf("Mounts[0].Target = %q, want /opt/dcx/git/0-.gitconfig", result.Mounts[0].Target)
	}
	if result.Mounts[1].Source != gitignorePath {
		t.Errorf("Mounts[1].Source = %q, want %q", result.Mounts[1].Source, gitignorePath)
	}
	if result.Mounts[1].Target != "/opt/dcx/git/1-.gitignore_global" {
		t.Errorf("Mounts[1].Target = %q, want /opt/dcx/git/1-.gitignore_global", result.Mounts[1].Target)
	}

	if result.EnvName != "GIT_CONFIG_GLOBAL" {
		t.Errorf("EnvName = %q, want GIT_CONFIG_GLOBAL", result.EnvName)
	}
	if result.EnvValue != "/opt/dcx/git/0-.gitconfig" {
		t.Errorf("EnvValue = %q, want /opt/dcx/git/0-.gitconfig", result.EnvValue)
	}
}

func TestDetectGitConfigsSomeFilesMissing(t *testing.T) {
	home := t.TempDir()

	gitconfigPath := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(gitconfigPath, []byte("[user]\n  name = test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", home)

	cfg := config.GitConfig{
		InjectConfigs: true,
		Configs:       []string{"~/.gitconfig", "~/.gitignore_global"},
	}

	result := DetectGitConfigs(cfg)

	if len(result.Mounts) != 1 {
		t.Fatalf("expected 1 mount (missing file skipped), got %d", len(result.Mounts))
	}
	if result.EnvName != "GIT_CONFIG_GLOBAL" {
		t.Errorf("EnvName = %q, want GIT_CONFIG_GLOBAL", result.EnvName)
	}
}

func TestDetectGitConfigsAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	gitconfigPath := filepath.Join(dir, "my-gitconfig")
	if err := os.WriteFile(gitconfigPath, []byte("[user]\n  name = test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.GitConfig{
		InjectConfigs: true,
		Configs:       []string{gitconfigPath},
	}

	result := DetectGitConfigs(cfg)

	if len(result.Mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(result.Mounts))
	}
	if result.Mounts[0].Source != gitconfigPath {
		t.Errorf("Mounts[0].Source = %q, want %q", result.Mounts[0].Source, gitconfigPath)
	}
	if result.Mounts[0].Target != "/opt/dcx/git/0-my-gitconfig" {
		t.Errorf("Mounts[0].Target = %q, want /opt/dcx/git/0-my-gitconfig", result.Mounts[0].Target)
	}
}

func TestDetectGitConfigsFirstFileGetsEnv(t *testing.T) {
	dir := t.TempDir()
	customPath := filepath.Join(dir, "custom-config")
	if err := os.WriteFile(customPath, []byte("[core]\n  editor = vim\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.GitConfig{
		InjectConfigs: true,
		Configs:       []string{customPath},
	}

	result := DetectGitConfigs(cfg)

	if len(result.Mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(result.Mounts))
	}
	if result.EnvName != "GIT_CONFIG_GLOBAL" {
		t.Errorf("EnvName = %q, want GIT_CONFIG_GLOBAL (first file always gets env)", result.EnvName)
	}
	if result.EnvValue != "/opt/dcx/git/0-custom-config" {
		t.Errorf("EnvValue = %q, want /opt/dcx/git/0-custom-config", result.EnvValue)
	}
}

func TestDetectGitConfigsCustomMountBase(t *testing.T) {
	home := t.TempDir()
	gitconfigPath := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(gitconfigPath, []byte("[user]\n  name = test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", home)

	cfg := config.GitConfig{
		InjectConfigs: true,
		Configs:       []string{"~/.gitconfig"},
		MountBase:     "/custom/git",
	}

	result := DetectGitConfigs(cfg)

	if len(result.Mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(result.Mounts))
	}
	if result.Mounts[0].Target != "/custom/git/0-.gitconfig" {
		t.Errorf("Mounts[0].Target = %q, want /custom/git/0-.gitconfig", result.Mounts[0].Target)
	}
	if result.EnvValue != "/custom/git/0-.gitconfig" {
		t.Errorf("EnvValue = %q, want /custom/git/0-.gitconfig", result.EnvValue)
	}
}

func TestDetectGitConfigsDuplicateBasenames(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	path1 := filepath.Join(dir1, "config")
	if err := os.WriteFile(path1, []byte("[user]\n  name = test1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path2 := filepath.Join(dir2, "config")
	if err := os.WriteFile(path2, []byte("[user]\n  name = test2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.GitConfig{
		InjectConfigs: true,
		Configs:       []string{path1, path2},
	}

	result := DetectGitConfigs(cfg)

	if len(result.Mounts) != 2 {
		t.Fatalf("expected 2 mounts, got %d", len(result.Mounts))
	}
	if result.Mounts[0].Target != "/opt/dcx/git/0-config" {
		t.Errorf("Mounts[0].Target = %q, want /opt/dcx/git/0-config", result.Mounts[0].Target)
	}
	if result.Mounts[1].Target != "/opt/dcx/git/1-config" {
		t.Errorf("Mounts[1].Target = %q, want /opt/dcx/git/1-config", result.Mounts[1].Target)
	}
}
