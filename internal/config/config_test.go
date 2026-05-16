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

// setupUserConfigEnv sets HOME and clears XDG_CONFIG_HOME so loadUserConfig
// reads from the given home directory. Centralises the env setup repeated
// across user config tests.
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

	if cfg.SSHForwarding == nil || !*cfg.SSHForwarding {
		t.Error("default SSHForwarding should be true")
	}
	if cfg.GitConfigForwarding == nil || !*cfg.GitConfigForwarding {
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

	cfg, err := loadUserConfig()
	if err != nil {
		t.Fatalf("loadUserConfig() error: %v", err)
	}

	if cfg.SSHForwarding == nil || *cfg.SSHForwarding {
		t.Error("SSHForwarding should be false")
	}
	if cfg.GitConfigForwarding == nil || *cfg.GitConfigForwarding {
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

	if cfg.SSHForwarding == nil || *cfg.SSHForwarding {
		t.Error("SSHForwarding should be false from XDG config")
	}
}

func TestLoadUserConfigInvalidYAML(t *testing.T) {
	home := t.TempDir()
	writeUserConfig(t, home, ":\n  :\n  invalid: [\n")

	setupUserConfigEnv(t, home)

	_, err := loadUserConfig()
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
	writeProjectConfig(t, dir, ":\n  invalid: [\n")

	_, err := loadProjectConfig(dir)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestComposeFilePathOutsideProject(t *testing.T) {
	dir := t.TempDir()
	writeProjectConfig(t, dir, `
compose_integration:
  compose_file: ../../outside/docker-compose.yml
  strategy: network_join
`)

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
		SSHForwarding:       boolPtr(false),
		GitConfigForwarding: boolPtr(true),
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

	if merged.SSHForwarding == nil || *merged.SSHForwarding {
		t.Error("merged SSHForwarding should preserve user value (false)")
	}
	if merged.GitConfigForwarding == nil || !*merged.GitConfigForwarding {
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
		SSHForwarding:       boolPtr(true),
		GitConfigForwarding: boolPtr(true),
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

func TestMergeProjectDoesNotOverrideUserWithDefaults(t *testing.T) {
	user := &Config{
		SSHForwarding:       boolPtr(false),
		GitConfigForwarding: boolPtr(false),
		ComposeIntegration: &ComposeIntegration{
			ComposeFile: "../user-compose.yml",
			Strategy:    "network_join",
		},
	}

	project := &Config{
		ComposeIntegration: &ComposeIntegration{
			ComposeFile: "../project-compose.yml",
			Strategy:    "overlay",
		},
	}

	merged := merge(user, project)

	if merged.SSHForwarding == nil || *merged.SSHForwarding {
		t.Error("merged SSHForwarding should preserve user value (false), not be overridden by project default")
	}
	if merged.GitConfigForwarding == nil || *merged.GitConfigForwarding {
		t.Error("merged GitConfigForwarding should preserve user value (false), not be overridden by project default")
	}
}

func TestEnvOverrides(t *testing.T) {
	cfg := &Config{
		SSHForwarding:       boolPtr(true),
		GitConfigForwarding: boolPtr(true),
	}

	t.Setenv("DCX_SSH_FORWARDING", "false")
	t.Setenv("DCX_GIT_CONFIG_FORWARDING", "0")

	applyEnvOverrides(cfg)

	if cfg.SSHForwarding == nil || *cfg.SSHForwarding {
		t.Error("SSHForwarding should be false after env override")
	}
	if cfg.GitConfigForwarding == nil || *cfg.GitConfigForwarding {
		t.Error("GitConfigForwarding should be false after env override")
	}
}

func TestEnvOverridesInvalidAndEmpty(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     *bool
	}{
		{"invalid value", "notabool", boolPtr(true)},
		{"empty value", "", boolPtr(true)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{SSHForwarding: boolPtr(true)}
			t.Setenv("DCX_SSH_FORWARDING", tt.envValue)
			applyEnvOverrides(cfg)
			if *cfg.SSHForwarding != *tt.want {
				t.Errorf("SSHForwarding = %v, want %v", *cfg.SSHForwarding, *tt.want)
			}
		})
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

	if cfg.SSHForwarding == nil || *cfg.SSHForwarding {
		t.Error("SSHForwarding should be false (env override)")
	}
	if cfg.GitConfigForwarding == nil || *cfg.GitConfigForwarding {
		t.Error("GitConfigForwarding should be false (user config)")
	}
	if cfg.ComposeIntegration.Strategy != "overlay" {
		t.Errorf("ComposeIntegration.Strategy = %q, want overlay (project)", cfg.ComposeIntegration.Strategy)
	}
	if cfg.ComposeIntegration.DevService != "app" {
		t.Errorf("ComposeIntegration.DevService = %q, want app (project)", cfg.ComposeIntegration.DevService)
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

	cfg, err := loadUserConfig()
	if err != nil {
		t.Fatalf("loadUserConfig() error: %v", err)
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

	cfg, err := loadProjectConfig(dir)
	if err != nil {
		t.Fatalf("loadProjectConfig() error: %v", err)
	}

	if len(cfg.DefaultFeatures) != 1 {
		t.Fatalf("expected 1 feature, got %d", len(cfg.DefaultFeatures))
	}
	if cfg.DefaultFeatures[0].ID != "ghcr.io/devcontainers/features/docker-in-docker:2" {
		t.Errorf("feature.ID = %q, want docker-in-docker", cfg.DefaultFeatures[0].ID)
	}
}

func TestMergeFeaturesUnionProjectWins(t *testing.T) {
	user := &Config{
		SSHForwarding: boolPtr(true),
		DefaultFeatures: []Feature{
			{ID: "ghcr.io/devcontainers/features/github-cli", Options: map[string]interface{}{"version": "1"}},
			{ID: "ghcr.io/devcontainers/features/docker-in-docker", Options: map[string]interface{}{}},
		},
	}

	project := &Config{
		DefaultFeatures: []Feature{
			{ID: "ghcr.io/devcontainers/features/github-cli", Options: map[string]interface{}{"version": "2"}},
			{ID: "ghcr.io/opencode/devcontainer-feature/opencode", Options: nil},
		},
	}

	merged := merge(user, project)

	if len(merged.DefaultFeatures) != 3 {
		t.Fatalf("expected 3 merged features, got %d", len(merged.DefaultFeatures))
	}

	byID := make(map[string]Feature, len(merged.DefaultFeatures))
	for _, f := range merged.DefaultFeatures {
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
	user := &Config{
		SSHForwarding: boolPtr(true),
		DefaultFeatures: []Feature{
			{ID: "ghcr.io/devcontainers/features/github-cli", Options: map[string]interface{}{}},
		},
	}

	project := &Config{}

	merged := merge(user, project)

	if len(merged.DefaultFeatures) != 1 {
		t.Fatalf("expected 1 feature, got %d", len(merged.DefaultFeatures))
	}
	if merged.DefaultFeatures[0].ID != "ghcr.io/devcontainers/features/github-cli" {
		t.Errorf("feature.ID = %q, want github-cli", merged.DefaultFeatures[0].ID)
	}
}

func TestMergeFeaturesProjectOnly(t *testing.T) {
	user := &Config{
		SSHForwarding: boolPtr(true),
	}

	project := &Config{
		DefaultFeatures: []Feature{
			{ID: "ghcr.io/opencode/devcontainer-feature/opencode", Options: nil},
		},
	}

	merged := merge(user, project)

	if len(merged.DefaultFeatures) != 1 {
		t.Fatalf("expected 1 feature, got %d", len(merged.DefaultFeatures))
	}
	if merged.DefaultFeatures[0].ID != "ghcr.io/opencode/devcontainer-feature/opencode" {
		t.Errorf("feature.ID = %q, want opencode", merged.DefaultFeatures[0].ID)
	}
}

func TestMergeFeaturesNeither(t *testing.T) {
	user := &Config{SSHForwarding: boolPtr(true)}
	project := &Config{}

	merged := merge(user, project)

	if len(merged.DefaultFeatures) != 0 {
		t.Errorf("expected 0 features, got %d", len(merged.DefaultFeatures))
	}
}
