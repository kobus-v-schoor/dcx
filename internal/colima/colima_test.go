package colima

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsActiveViaDockerHost(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", tmpDir)
	t.Setenv("DOCKER_HOST", "unix:///Users/kobus/.colima/default/docker.sock")

	if !IsActive() {
		t.Error("expected IsActive() = true when DOCKER_HOST contains .colima")
	}
}

func TestIsActiveNotColima(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", tmpDir)
	t.Setenv("DOCKER_HOST", "unix:///var/run/docker.sock")

	configPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"currentContext": "desktop-linux"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if IsActive() {
		t.Error("expected IsActive() = false for non-colima context")
	}
}

func TestIsActiveViaDockerConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", tmpDir)
	t.Setenv("DOCKER_HOST", "")

	configPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"currentContext": "colima"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if !IsActive() {
		t.Error("expected IsActive() = true when currentContext is colima")
	}
}

func TestIsActiveViaNamedProfile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", tmpDir)
	t.Setenv("DOCKER_HOST", "")

	configPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"currentContext": "colima-work"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if !IsActive() {
		t.Error("expected IsActive() = true when currentContext is colima-work")
	}
}

func TestProfileDefault(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", tmpDir)

	configPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"currentContext": "colima"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := Profile(); got != "default" {
		t.Errorf("Profile() = %q, want default", got)
	}
}

func TestProfileNamed(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", tmpDir)

	configPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"currentContext": "colima-work"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := Profile(); got != "work" {
		t.Errorf("Profile() = %q, want work", got)
	}
}

func TestProfileDockerHostFallback(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", tmpDir)
	t.Setenv("DOCKER_HOST", "unix:///Users/kobus/.colima/default/docker.sock")

	if got := Profile(); got != "default" {
		t.Errorf("Profile() = %q, want default", got)
	}
}

func TestProfileInactive(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", tmpDir)
	t.Setenv("DOCKER_HOST", "")

	configPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"currentContext": "desktop-linux"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := Profile(); got != "" {
		t.Errorf("Profile() = %q, want empty string", got)
	}
}

func TestCurrentContextMissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", filepath.Join(tmpDir, "nonexistent"))

	if got := currentContext(); got != "" {
		t.Errorf("currentContext() = %q, want empty string", got)
	}
}
