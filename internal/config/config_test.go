package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeUserConfig(t *testing.T, home, content string) {
	t.Helper()
	configDir := filepath.Join(home, ".config", "dcx")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeProjectConfig(t *testing.T, dir, content string) {
	t.Helper()
	devcontainerDir := filepath.Join(dir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devcontainerDir, "dcx.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// setupUserConfigEnv sets HOME and clears XDG_CONFIG_HOME so Load reads
// user config from the given home directory. Centralises the env setup
// repeated across tests.
func setupUserConfigEnv(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
}

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if !cfg.SSHForwarding {
		t.Error("default SSHForwarding should be true")
	}
	if !cfg.GitConfigForwarding {
		t.Error("default GitConfigForwarding should be true")
	}
	if cfg.ComposeIntegration != nil {
		t.Error("default ComposeIntegration should be nil")
	}
}

func TestLoadUserConfig(t *testing.T) {
	home := t.TempDir()
	writeUserConfig(t, home, `
ssh_forwarding: false
git_config_forwarding: false
compose_integration:
  compose_file: ../docker-compose.yml
  strategy: network_join
`)

	setupUserConfigEnv(t, home)

	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.SSHForwarding {
		t.Error("SSHForwarding should be false")
	}
	if cfg.GitConfigForwarding {
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

	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.SSHForwarding {
		t.Error("SSHForwarding should be false from XDG config")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	home := t.TempDir()
	writeUserConfig(t, home, ":\n  :\n  invalid: [\n")

	setupUserConfigEnv(t, home)

	_, err := Load(t.TempDir())
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadProjectConfig(t *testing.T) {
	dir := t.TempDir()
	writeProjectConfig(t, dir, `
compose_integration:
  compose_file: ../docker-compose.yml
  strategy: overlay
  dev_service: app
`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
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

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.ComposeIntegration != nil {
		t.Error("ComposeIntegration should be nil for missing project config")
	}
}

func TestComposeFilePathOutsideProject(t *testing.T) {
	dir := t.TempDir()
	writeProjectConfig(t, dir, `
compose_integration:
  compose_file: ../../outside/docker-compose.yml
  strategy: network_join
`)

	_, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
}

func TestLoadProjectComposeOverridesUser(t *testing.T) {
	home := t.TempDir()
	writeUserConfig(t, home, `
ssh_forwarding: false
git_config_forwarding: true
compose_integration:
  compose_file: ../user-compose.yml
  strategy: network_join
`)

	projectDir := t.TempDir()
	writeProjectConfig(t, projectDir, `
compose_integration:
  compose_file: ../project-compose.yml
  strategy: overlay
  dev_service: app
`)

	setupUserConfigEnv(t, home)

	cfg, err := Load(projectDir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.SSHForwarding {
		t.Error("SSHForwarding should preserve user value (false)")
	}
	if !cfg.GitConfigForwarding {
		t.Error("GitConfigForwarding should preserve user value (true)")
	}
	if cfg.ComposeIntegration.ComposeFile != "../project-compose.yml" {
		t.Errorf("ComposeFile = %q, want project value", cfg.ComposeIntegration.ComposeFile)
	}
	if cfg.ComposeIntegration.Strategy != "overlay" {
		t.Errorf("Strategy = %q, want project value", cfg.ComposeIntegration.Strategy)
	}
}

func TestLoadNoProjectCompose(t *testing.T) {
	home := t.TempDir()
	writeUserConfig(t, home, `
ssh_forwarding: true
git_config_forwarding: true
compose_integration:
  compose_file: ../user-compose.yml
  strategy: network_join
`)

	projectDir := t.TempDir()

	setupUserConfigEnv(t, home)

	cfg, err := Load(projectDir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.ComposeIntegration.ComposeFile != "../user-compose.yml" {
		t.Errorf("ComposeFile = %q, want user value", cfg.ComposeIntegration.ComposeFile)
	}
}

func TestLoadProjectDoesNotOverrideUserWithDefaults(t *testing.T) {
	home := t.TempDir()
	writeUserConfig(t, home, `
ssh_forwarding: false
git_config_forwarding: false
compose_integration:
  compose_file: ../user-compose.yml
  strategy: network_join
`)

	projectDir := t.TempDir()
	writeProjectConfig(t, projectDir, `
compose_integration:
  compose_file: ../project-compose.yml
  strategy: overlay
`)

	setupUserConfigEnv(t, home)

	cfg, err := Load(projectDir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.SSHForwarding {
		t.Error("SSHForwarding should preserve user value (false)")
	}
	if cfg.GitConfigForwarding {
		t.Error("GitConfigForwarding should preserve user value (false)")
	}
}

func TestEnvOverrides(t *testing.T) {
	home := t.TempDir()
	writeUserConfig(t, home, `
ssh_forwarding: true
git_config_forwarding: true
`)

	setupUserConfigEnv(t, home)
	t.Setenv("DCX_SSH_FORWARDING", "false")
	t.Setenv("DCX_GIT_CONFIG_FORWARDING", "0")

	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.SSHForwarding {
		t.Error("SSHForwarding should be false after env override")
	}
	if cfg.GitConfigForwarding {
		t.Error("GitConfigForwarding should be false after env override")
	}
}

func TestLoadUserConfigLogLevel(t *testing.T) {
	home := t.TempDir()
	writeUserConfig(t, home, `
log_level: debug
ssh_forwarding: false
`)

	setupUserConfigEnv(t, home)

	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
}

func TestLoadProjectConfigLogLevel(t *testing.T) {
	dir := t.TempDir()
	writeProjectConfig(t, dir, `
log_level: info
`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
}

func TestEnvOverrideLogLevel(t *testing.T) {
	home := t.TempDir()
	writeUserConfig(t, home, `
log_level: info
ssh_forwarding: true
`)

	setupUserConfigEnv(t, home)
	t.Setenv("DCX_LOG_LEVEL", "debug")

	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q after env override", cfg.LogLevel, "debug")
	}
}

func TestLoadFullPipeline(t *testing.T) {
	home := t.TempDir()
	writeUserConfig(t, home, `
ssh_forwarding: true
git_config_forwarding: false
compose_integration:
  compose_file: ../user-compose.yml
  strategy: network_join
`)

	projectDir := t.TempDir()
	writeProjectConfig(t, projectDir, `
compose_integration:
  compose_file: ../docker-compose.yml
  strategy: overlay
  dev_service: app
`)

	setupUserConfigEnv(t, home)
	t.Setenv("DCX_SSH_FORWARDING", "false")

	cfg, err := Load(projectDir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.SSHForwarding {
		t.Error("SSHForwarding should be false (env override)")
	}
	if cfg.GitConfigForwarding {
		t.Error("GitConfigForwarding should be false (user config)")
	}
	if cfg.ComposeIntegration.Strategy != "overlay" {
		t.Errorf("Strategy = %q, want overlay (project)", cfg.ComposeIntegration.Strategy)
	}
	if cfg.ComposeIntegration.DevService != "app" {
		t.Errorf("DevService = %q, want app (project)", cfg.ComposeIntegration.DevService)
	}
}

func TestLoadFullPipelineLogLevel(t *testing.T) {
	home := t.TempDir()
	writeUserConfig(t, home, `
log_level: info
`)

	projectDir := t.TempDir()
	writeProjectConfig(t, projectDir, ``)

	setupUserConfigEnv(t, home)

	cfg, err := Load(projectDir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q (user config)", cfg.LogLevel, "info")
	}
}

func TestLoadFullPipelineLogLevelEnvOverride(t *testing.T) {
	home := t.TempDir()
	writeUserConfig(t, home, `
log_level: info
`)

	projectDir := t.TempDir()
	writeProjectConfig(t, projectDir, ``)

	setupUserConfigEnv(t, home)
	t.Setenv("DCX_LOG_LEVEL", "debug")

	cfg, err := Load(projectDir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q (env override)", cfg.LogLevel, "debug")
	}
}

func TestLoadUserConfigWithFeatures(t *testing.T) {
	home := t.TempDir()
	writeUserConfig(t, home, `
default_features:
  - id: ghcr.io/devcontainers/features/github-cli
    options:
      version: latest
  - id: ghcr.io/opencode/devcontainer-feature/opencode
    options: {}
`)

	setupUserConfigEnv(t, home)

	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if len(cfg.DefaultFeatures) != 2 {
		t.Fatalf("expected 2 features, got %d", len(cfg.DefaultFeatures))
	}
	if cfg.DefaultFeatures[0].ID != "ghcr.io/devcontainers/features/github-cli" {
		t.Errorf("feature[0].ID = %q, want github-cli", cfg.DefaultFeatures[0].ID)
	}
	if cfg.DefaultFeatures[0].Options["version"] != "latest" {
		t.Errorf("feature[0].Options[version] = %v, want latest", cfg.DefaultFeatures[0].Options["version"])
	}
	if cfg.DefaultFeatures[1].ID != "ghcr.io/opencode/devcontainer-feature/opencode" {
		t.Errorf("feature[1].ID = %q, want opencode", cfg.DefaultFeatures[1].ID)
	}
	if len(cfg.DefaultFeatures[1].Options) != 0 {
		t.Errorf("feature[1].Options = %v, want empty map", cfg.DefaultFeatures[1].Options)
	}
}

func TestLoadProjectConfigWithFeatures(t *testing.T) {
	dir := t.TempDir()
	writeProjectConfig(t, dir, `
default_features:
  - id: ghcr.io/devcontainers/features/docker-in-docker:2
    options: {}
`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if len(cfg.DefaultFeatures) != 1 {
		t.Fatalf("expected 1 feature, got %d", len(cfg.DefaultFeatures))
	}
	if cfg.DefaultFeatures[0].ID != "ghcr.io/devcontainers/features/docker-in-docker:2" {
		t.Errorf("feature.ID = %q, want docker-in-docker", cfg.DefaultFeatures[0].ID)
	}
}

func TestMergeFeaturesUnionProjectWins(t *testing.T) {
	user := []Feature{
		{ID: "ghcr.io/devcontainers/features/github-cli", Options: map[string]interface{}{"version": "1"}},
		{ID: "ghcr.io/devcontainers/features/docker-in-docker", Options: map[string]interface{}{}},
	}

	project := []Feature{
		{ID: "ghcr.io/devcontainers/features/github-cli", Options: map[string]interface{}{"version": "2"}},
		{ID: "ghcr.io/opencode/devcontainer-feature/opencode", Options: nil},
	}

	merged := mergeFeatures(user, project)

	if len(merged) != 3 {
		t.Fatalf("expected 3 merged features, got %d", len(merged))
	}

	byID := make(map[string]Feature, len(merged))
	for _, f := range merged {
		byID[f.ID] = f
	}

	if f, ok := byID["ghcr.io/devcontainers/features/github-cli"]; !ok {
		t.Error("github-cli feature missing from merged result")
	} else if f.Options["version"] != "2" {
		t.Errorf("github-cli version = %v, want 2 (project wins)", f.Options["version"])
	}

	if _, ok := byID["ghcr.io/devcontainers/features/docker-in-docker"]; !ok {
		t.Error("docker-in-docker feature missing from merged result (user-only)")
	}

	if _, ok := byID["ghcr.io/opencode/devcontainer-feature/opencode"]; !ok {
		t.Error("opencode feature missing from merged result (project-only)")
	}
}

func TestMergeFeaturesUserOnly(t *testing.T) {
	user := []Feature{
		{ID: "ghcr.io/devcontainers/features/github-cli", Options: map[string]interface{}{}},
	}

	merged := mergeFeatures(user, nil)

	if len(merged) != 1 {
		t.Fatalf("expected 1 feature, got %d", len(merged))
	}
	if merged[0].ID != "ghcr.io/devcontainers/features/github-cli" {
		t.Errorf("feature.ID = %q, want github-cli", merged[0].ID)
	}
}

func TestMergeFeaturesProjectOnly(t *testing.T) {
	project := []Feature{
		{ID: "ghcr.io/opencode/devcontainer-feature/opencode", Options: nil},
	}

	merged := mergeFeatures(nil, project)

	if len(merged) != 1 {
		t.Fatalf("expected 1 feature, got %d", len(merged))
	}
	if merged[0].ID != "ghcr.io/opencode/devcontainer-feature/opencode" {
		t.Errorf("feature.ID = %q, want opencode", merged[0].ID)
	}
}

func TestMergeFeaturesNeither(t *testing.T) {
	merged := mergeFeatures(nil, nil)

	if len(merged) != 0 {
		t.Errorf("expected 0 features, got %d", len(merged))
	}
}
