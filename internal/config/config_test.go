package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if !cfg.SSHForwardingValue() {
		t.Error("default SSHForwarding should be true")
	}
	if !cfg.GitConfigForwardingValue() {
		t.Error("default GitConfigForwarding should be true")
	}
	if cfg.ComposeIntegration != nil {
		t.Error("default ComposeIntegration should be nil")
	}
}

func TestLoadUserConfig(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "dcx")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}

	configYAML := `
ssh_forwarding: false
git_config_forwarding: false
compose_integration:
  compose_file: ../docker-compose.yml
  strategy: network_join
`
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	cfg, err := loadUserConfig()
	if err != nil {
		t.Fatalf("loadUserConfig() error: %v", err)
	}

	if cfg.SSHForwardingValue() {
		t.Error("SSHForwarding should be false")
	}
	if cfg.GitConfigForwardingValue() {
		t.Error("GitConfigForwarding should be false")
	}
	if cfg.ComposeIntegration == nil {
		t.Fatal("ComposeIntegration should not be nil")
	}
	if cfg.ComposeIntegration.Strategy != "network_join" {
		t.Errorf("Strategy = %q, want %q", cfg.ComposeIntegration.Strategy, "network_join")
	}
}

func TestLoadUserConfigXDG(t *testing.T) {
	xdg := t.TempDir()
	configDir := filepath.Join(xdg, "dcx")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("ssh_forwarding: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("XDG_CONFIG_HOME", xdg)

	cfg, err := loadUserConfig()
	if err != nil {
		t.Fatalf("loadUserConfig() error: %v", err)
	}

	if cfg.SSHForwardingValue() {
		t.Error("SSHForwarding should be false from XDG config")
	}
}

func TestLoadUserConfigInvalidYAML(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "dcx")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(":\n  :\n  invalid: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	_, err := loadUserConfig()
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadProjectConfig(t *testing.T) {
	dir := t.TempDir()
	devcontainerDir := filepath.Join(dir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}

	configYAML := `
compose_integration:
  compose_file: ../docker-compose.yml
  strategy: overlay
  dev_service: app
`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "dcx.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadProjectConfig(dir)
	if err != nil {
		t.Fatalf("loadProjectConfig() error: %v", err)
	}

	if cfg.ComposeIntegration == nil {
		t.Fatal("ComposeIntegration should not be nil")
	}
	if cfg.ComposeIntegration.Strategy != "overlay" {
		t.Errorf("Strategy = %q, want %q", cfg.ComposeIntegration.Strategy, "overlay")
	}
	if cfg.ComposeIntegration.DevService != "app" {
		t.Errorf("DevService = %q, want %q", cfg.ComposeIntegration.DevService, "app")
	}
}

func TestLoadProjectConfigMissing(t *testing.T) {
	dir := t.TempDir()

	cfg, err := loadProjectConfig(dir)
	if err != nil {
		t.Fatalf("loadProjectConfig() error: %v", err)
	}

	if cfg.ComposeIntegration != nil {
		t.Error("ComposeIntegration should be nil for missing project config")
	}
}

func TestLoadProjectConfigInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	devcontainerDir := filepath.Join(dir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(devcontainerDir, "dcx.yaml"), []byte(":\n  invalid: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := loadProjectConfig(dir)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestComposeFilePathOutsideProject(t *testing.T) {
	dir := t.TempDir()
	devcontainerDir := filepath.Join(dir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}

	configYAML := `
compose_integration:
  compose_file: ../../outside/docker-compose.yml
  strategy: network_join
`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "dcx.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadProjectConfig(dir)
	if err != nil {
		t.Fatalf("loadProjectConfig() error: %v", err)
	}

	if cfg.ComposeIntegration == nil {
		t.Fatal("ComposeIntegration should not be nil")
	}
}

func TestMergeProjectComposeOverridesUser(t *testing.T) {
	user := &Config{
		SSHForwarding:       ptrBool(false),
		GitConfigForwarding: ptrBool(true),
		ComposeIntegration: &ComposeIntegration{
			ComposeFile: "../user-compose.yml",
			Strategy:    "network_join",
		},
	}

	project := &Config{
		ComposeIntegration: &ComposeIntegration{
			ComposeFile: "../project-compose.yml",
			Strategy:    "overlay",
			DevService:  "app",
		},
	}

	merged := merge(user, project)

	if merged.SSHForwardingValue() {
		t.Error("merged SSHForwarding should preserve user value (false)")
	}
	if !merged.GitConfigForwardingValue() {
		t.Error("merged GitConfigForwarding should preserve user value (true)")
	}
	if merged.ComposeIntegration.ComposeFile != "../project-compose.yml" {
		t.Errorf("merged ComposeIntegration.ComposeFile = %q, want project value", merged.ComposeIntegration.ComposeFile)
	}
	if merged.ComposeIntegration.Strategy != "overlay" {
		t.Errorf("merged ComposeIntegration.Strategy = %q, want project value", merged.ComposeIntegration.Strategy)
	}
}

func TestMergeNoProjectCompose(t *testing.T) {
	user := &Config{
		SSHForwarding:       ptrBool(true),
		GitConfigForwarding: ptrBool(true),
		ComposeIntegration: &ComposeIntegration{
			ComposeFile: "../user-compose.yml",
			Strategy:    "network_join",
		},
	}

	project := &Config{}

	merged := merge(user, project)

	if merged.ComposeIntegration.ComposeFile != "../user-compose.yml" {
		t.Errorf("merged ComposeIntegration.ComposeFile = %q, want user value", merged.ComposeIntegration.ComposeFile)
	}
}

func TestEnvOverrides(t *testing.T) {
	cfg := &Config{
		SSHForwarding:       ptrBool(true),
		GitConfigForwarding: ptrBool(true),
	}

	t.Setenv("DCX_SSH_FORWARDING", "false")
	t.Setenv("DCX_GIT_CONFIG_FORWARDING", "0")

	applyEnvOverrides(cfg)

	if cfg.SSHForwardingValue() {
		t.Error("SSHForwarding should be false after env override")
	}
	if cfg.GitConfigForwardingValue() {
		t.Error("GitConfigForwarding should be false after env override")
	}
}

func TestEnvOverridesInvalid(t *testing.T) {
	cfg := &Config{
		SSHForwarding: ptrBool(true),
	}

	t.Setenv("DCX_SSH_FORWARDING", "notabool")

	applyEnvOverrides(cfg)

	if !cfg.SSHForwardingValue() {
		t.Error("SSHForwarding should remain true when env value is invalid")
	}
}

func TestEnvOverridesEmpty(t *testing.T) {
	cfg := &Config{
		SSHForwarding: ptrBool(true),
	}

	t.Setenv("DCX_SSH_FORWARDING", "")

	applyEnvOverrides(cfg)

	if !cfg.SSHForwardingValue() {
		t.Error("SSHForwarding should remain true when env value is empty")
	}
}

func TestLoadFullPipeline(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "dcx")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}

	userYAML := `
ssh_forwarding: true
git_config_forwarding: false
compose_integration:
  compose_file: ../user-compose.yml
  strategy: network_join
`
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(userYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	projectDir := t.TempDir()
	devcontainerDir := filepath.Join(projectDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}

	projectYAML := `
compose_integration:
  compose_file: ../docker-compose.yml
  strategy: overlay
  dev_service: app
`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "dcx.yaml"), []byte(projectYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("DCX_SSH_FORWARDING", "false")

	cfg, err := Load(projectDir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.SSHForwardingValue() {
		t.Error("SSHForwarding should be false (env override)")
	}
	if cfg.GitConfigForwardingValue() {
		t.Error("GitConfigForwarding should be false (user config)")
	}
	if cfg.ComposeIntegration.Strategy != "overlay" {
		t.Errorf("ComposeIntegration.Strategy = %q, want overlay (project)", cfg.ComposeIntegration.Strategy)
	}
	if cfg.ComposeIntegration.DevService != "app" {
		t.Errorf("ComposeIntegration.DevService = %q, want app (project)", cfg.ComposeIntegration.DevService)
	}
}

func ptrBool(v bool) *bool {
	return &v
}
