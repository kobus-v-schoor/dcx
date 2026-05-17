package ssh

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kobus-v-schoor/dcx/internal/config"
)

func TestDetectGitConfigsDefault(t *testing.T) {
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
	if result.Mounts[0].Target != "/opt/dcx/git/.gitconfig" {
		t.Errorf("Mounts[0].Target = %q, want /opt/dcx/git/.gitconfig", result.Mounts[0].Target)
	}
	if !result.Mounts[0].ReadOnly {
		t.Error("Mounts[0].ReadOnly should be true for git configs")
	}
	if result.EnvName != "GIT_CONFIG_GLOBAL" {
		t.Errorf("EnvName = %q, want GIT_CONFIG_GLOBAL", result.EnvName)
	}
	if result.EnvValue != "/opt/dcx/git/.gitconfig" {
		t.Errorf("EnvValue = %q, want /opt/dcx/git/.gitconfig", result.EnvValue)
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
	if result.Mounts[1].Source != gitignorePath {
		t.Errorf("Mounts[1].Source = %q, want %q", result.Mounts[1].Source, gitignorePath)
	}

	// GIT_CONFIG_GLOBAL should only be set for the first gitconfig file.
	if result.EnvName != "GIT_CONFIG_GLOBAL" {
		t.Errorf("EnvName = %q, want GIT_CONFIG_GLOBAL", result.EnvName)
	}
	if result.EnvValue != "/opt/dcx/git/.gitconfig" {
		t.Errorf("EnvValue = %q, want /opt/dcx/git/.gitconfig", result.EnvValue)
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

	// Only .gitconfig exists; .gitignore_global is silently skipped.
	if len(result.Mounts) != 1 {
		t.Fatalf("expected 1 mount (missing file skipped), got %d", len(result.Mounts))
	}
	if result.EnvName != "GIT_CONFIG_GLOBAL" {
		t.Errorf("EnvName = %q, want GIT_CONFIG_GLOBAL", result.EnvName)
	}
}

func TestDetectGitConfigsEmptyListDefaultsToGitconfig(t *testing.T) {
	home := t.TempDir()
	gitconfigPath := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(gitconfigPath, []byte("[user]\n  name = test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", home)

	cfg := config.GitConfig{
		InjectConfigs: true,
		Configs:       nil,
	}

	result := DetectGitConfigs(cfg)

	if len(result.Mounts) != 1 {
		t.Fatalf("expected 1 mount with default config, got %d", len(result.Mounts))
	}
	if result.Mounts[0].Source != gitconfigPath {
		t.Errorf("Mounts[0].Source = %q, want %q", result.Mounts[0].Source, gitconfigPath)
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
	if result.Mounts[0].Target != "/opt/dcx/git/my-gitconfig" {
		t.Errorf("Mounts[0].Target = %q, want /opt/dcx/git/my-gitconfig", result.Mounts[0].Target)
	}
}

func TestDetectGitConfigsNoGitconfigBasenameNoEnv(t *testing.T) {
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
	// File without "gitconfig" in basename should not set GIT_CONFIG_GLOBAL.
	if result.EnvName != "" {
		t.Errorf("EnvName = %q, want empty when basename does not contain 'gitconfig'", result.EnvName)
	}
}

func TestFormatGitEnv(t *testing.T) {
	result := GitResult{
		EnvName:  "GIT_CONFIG_GLOBAL",
		EnvValue: "/opt/dcx/git/.gitconfig",
	}

	got := FormatGitEnv(result)
	want := "GIT_CONFIG_GLOBAL=/opt/dcx/git/.gitconfig"
	if got != want {
		t.Errorf("FormatGitEnv() = %q, want %q", got, want)
	}
}

func TestFormatGitEnvEmpty(t *testing.T) {
	result := GitResult{}

	got := FormatGitEnv(result)
	if got != "" {
		t.Errorf("FormatGitEnv() with empty result = %q, want empty string", got)
	}
}
