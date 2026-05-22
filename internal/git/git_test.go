package git

import (
	"os"
	"os/exec"
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

// TestRepoFromURL_HTTP tests parsing of HTTPS remote URLs.
func TestRepoFromURL_HTTP(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
		ok   bool
	}{
		{"https with .git", "https://github.com/owner/repo.git", "owner/repo", true},
		{"https without .git", "https://github.com/owner/repo", "owner/repo", true},
		{"https with path prefix", "https://github.com/org/team/owner/repo.git", "owner/repo", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := RepoFromURL(tt.url)
			if ok != tt.ok {
				t.Fatalf("RepoFromURL(%q) ok = %v, want %v", tt.url, ok, tt.ok)
			}
			if got != tt.want {
				t.Errorf("RepoFromURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

// TestRepoFromURL_SSH tests parsing of SSH remote URLs.
func TestRepoFromURL_SSH(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
		ok   bool
	}{
		{"scp-like with .git", "git@github.com:owner/repo.git", "owner/repo", true},
		{"scp-like without .git", "git@github.com:owner/repo", "owner/repo", true},
		{"scp-like with path prefix", "git@github.com:org/owner/repo.git", "owner/repo", true},
		{"ssh scheme with .git", "ssh://git@github.com/owner/repo.git", "owner/repo", true},
		{"ssh scheme without .git", "ssh://git@github.com/owner/repo", "owner/repo", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := RepoFromURL(tt.url)
			if ok != tt.ok {
				t.Fatalf("RepoFromURL(%q) ok = %v, want %v", tt.url, ok, tt.ok)
			}
			if got != tt.want {
				t.Errorf("RepoFromURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

// TestRepoFromURL_Invalid tests that invalid or unparseable URLs return false.
func TestRepoFromURL_Invalid(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"empty", ""},
		{"just a host", "github.com"},
		{"only owner", "https://github.com/owner"},
		{"no path segments", "https://github.com/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := RepoFromURL(tt.url)
			if ok {
				t.Errorf("RepoFromURL(%q) ok = true, want false (got %q)", tt.url, got)
			}
		})
	}
}

// TestDetectRepo tests detecting the repository from a real git repository.
func TestDetectRepo(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialise a git repository and add an origin remote.
	if err := exec.Command("git", "init", tmpDir).Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}
	if err := exec.Command("git", "-C", tmpDir, "remote", "add", "origin", "https://github.com/test-owner/test-repo.git").Run(); err != nil {
		t.Fatalf("git remote add failed: %v", err)
	}

	// Change into the temp directory so DetectRepo finds the remote.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Fatalf("chdir back failed: %v", err)
		}
	}()

	repo, ok := DetectRepo()
	if !ok {
		t.Fatal("DetectRepo() = false, want true")
	}
	if repo != "test-owner/test-repo" {
		t.Errorf("DetectRepo() = %q, want %q", repo, "test-owner/test-repo")
	}
}

// TestDetectRepo_NotARepo tests that DetectRepo returns false when not in a
// git repository.
func TestDetectRepo_NotARepo(t *testing.T) {
	tmpDir := t.TempDir()

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Fatalf("chdir back failed: %v", err)
		}
	}()

	_, ok := DetectRepo()
	if ok {
		t.Error("DetectRepo() = true, want false when not in a git repo")
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

func TestSafeDirEnvVars(t *testing.T) {
	result := SafeDirEnvVars("/workspace")

	if len(result) != 3 {
		t.Fatalf("expected 3 env vars, got %d", len(result))
	}

	found := make(map[string]string, len(result))
	for _, r := range result {
		found[r.Name] = r.Value
	}

	if found["GIT_CONFIG_COUNT"] != "1" {
		t.Errorf("GIT_CONFIG_COUNT = %q, want %q", found["GIT_CONFIG_COUNT"], "1")
	}
	if found["GIT_CONFIG_KEY_0"] != "safe.directory" {
		t.Errorf("GIT_CONFIG_KEY_0 = %q, want %q", found["GIT_CONFIG_KEY_0"], "safe.directory")
	}
	if found["GIT_CONFIG_VALUE_0"] != "/workspace" {
		t.Errorf("GIT_CONFIG_VALUE_0 = %q, want %q", found["GIT_CONFIG_VALUE_0"], "/workspace")
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
