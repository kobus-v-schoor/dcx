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

	if !cfg.SSH.ForwardAgent {
		t.Error("default SSH.ForwardAgent should be true")
	}
	if !cfg.Git.InjectConfigs {
		t.Error("default Git.InjectConfigs should be true")
	}
	if len(cfg.Git.Configs) != 1 || cfg.Git.Configs[0] != "~/.gitconfig" {
		t.Errorf("default Git.Configs = %v, want [~/.gitconfig]", cfg.Git.Configs)
	}
	if cfg.ComposeIntegration != nil {
		t.Error("default ComposeIntegration should be nil")
	}
}

func TestLoadUserConfig(t *testing.T) {
	home := t.TempDir()
	writeUserConfig(t, home, `
ssh:
  forward_agent: false
git:
  inject_configs: false
compose_integration:
  compose_file: ../docker-compose.yml
  strategy: network_join
`)

	setupUserConfigEnv(t, home)

	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.SSH.ForwardAgent {
		t.Error("SSH.ForwardAgent should be false")
	}
	if cfg.Git.InjectConfigs {
		t.Error("Git.InjectConfigs should be false")
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
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("ssh:\n  forward_agent: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("XDG_CONFIG_HOME", xdg)

	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.SSH.ForwardAgent {
		t.Error("SSH.ForwardAgent should be false from XDG config")
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
ssh:
  forward_agent: false
git:
  inject_configs: true
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

	if cfg.SSH.ForwardAgent {
		t.Error("SSH.ForwardAgent should preserve user value (false)")
	}
	if !cfg.Git.InjectConfigs {
		t.Error("Git.InjectConfigs should preserve user value (true)")
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
ssh:
  forward_agent: true
git:
  inject_configs: true
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
ssh:
  forward_agent: false
git:
  inject_configs: false
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

	if cfg.SSH.ForwardAgent {
		t.Error("SSH.ForwardAgent should preserve user value (false)")
	}
	if cfg.Git.InjectConfigs {
		t.Error("Git.InjectConfigs should preserve user value (false)")
	}
}

func TestEnvOverrides(t *testing.T) {
	home := t.TempDir()
	writeUserConfig(t, home, `
ssh:
  forward_agent: true
git:
  inject_configs: true
`)

	setupUserConfigEnv(t, home)
	t.Setenv("DCX_SSH_FORWARD_AGENT", "false")
	t.Setenv("DCX_GIT_INJECT_CONFIGS", "0")

	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.SSH.ForwardAgent {
		t.Error("SSH.ForwardAgent should be false after env override")
	}
	if cfg.Git.InjectConfigs {
		t.Error("Git.InjectConfigs should be false after env override")
	}
}

func TestLoadUserConfigLogLevel(t *testing.T) {
	home := t.TempDir()
	writeUserConfig(t, home, `
log_level: debug
ssh:
  forward_agent: false
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
ssh:
  forward_agent: true
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
ssh:
  forward_agent: true
git:
  inject_configs: false
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
	t.Setenv("DCX_SSH_FORWARD_AGENT", "false")

	cfg, err := Load(projectDir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.SSH.ForwardAgent {
		t.Error("SSH.ForwardAgent should be false (env override)")
	}
	if cfg.Git.InjectConfigs {
		t.Error("Git.InjectConfigs should be false (user config)")
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

func TestLoadUserConfigWithMounts(t *testing.T) {
	home := t.TempDir()
	writeUserConfig(t, home, `
mounts:
  - source: ~/config
    target: /home/vscode/.config
    read_only: true
  - source: /data/project
    target: /workspace
`)

	setupUserConfigEnv(t, home)

	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if len(cfg.Mounts) != 2 {
		t.Fatalf("expected 2 mounts, got %d", len(cfg.Mounts))
	}
	if cfg.Mounts[0].Source != "~/config" {
		t.Errorf("mounts[0].Source = %q, want ~/config", cfg.Mounts[0].Source)
	}
	if cfg.Mounts[0].Target != "/home/vscode/.config" {
		t.Errorf("mounts[0].Target = %q, want /home/vscode/.config", cfg.Mounts[0].Target)
	}
	if !cfg.Mounts[0].ReadOnly {
		t.Error("mounts[0].ReadOnly should be true")
	}
	if cfg.Mounts[1].ReadOnly {
		t.Error("mounts[1].ReadOnly should be false (default)")
	}
}

func TestLoadProjectConfigWithMounts(t *testing.T) {
	dir := t.TempDir()
	writeProjectConfig(t, dir, `
mounts:
  - source: /opt/tools
    target: /tools
    read_only: true
`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if len(cfg.Mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(cfg.Mounts))
	}
	if cfg.Mounts[0].Source != "/opt/tools" {
		t.Errorf("mount.Source = %q, want /opt/tools", cfg.Mounts[0].Source)
	}
}

func TestMergeMountsConcatenates(t *testing.T) {
	user := []Mount{
		{Source: "/user/a", Target: "/a", ReadOnly: true},
	}
	project := []Mount{
		{Source: "/project/b", Target: "/b", ReadOnly: false},
	}

	merged := mergeMounts(user, project)

	if len(merged) != 2 {
		t.Fatalf("expected 2 mounts, got %d", len(merged))
	}
	if merged[0].Source != "/user/a" {
		t.Errorf("merged[0].Source = %q, want /user/a", merged[0].Source)
	}
	if merged[1].Source != "/project/b" {
		t.Errorf("merged[1].Source = %q, want /project/b", merged[1].Source)
	}
}

func TestMergeMountsUserOnly(t *testing.T) {
	user := []Mount{
		{Source: "/user/a", Target: "/a", ReadOnly: true},
	}

	merged := mergeMounts(user, nil)

	if len(merged) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(merged))
	}
	if merged[0].Source != "/user/a" {
		t.Errorf("merged[0].Source = %q, want /user/a", merged[0].Source)
	}
}

func TestMergeMountsProjectOnly(t *testing.T) {
	project := []Mount{
		{Source: "/project/b", Target: "/b", ReadOnly: false},
	}

	merged := mergeMounts(nil, project)

	if len(merged) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(merged))
	}
	if merged[0].Source != "/project/b" {
		t.Errorf("merged[0].Source = %q, want /project/b", merged[0].Source)
	}
}

func TestMergeMountsNeither(t *testing.T) {
	merged := mergeMounts(nil, nil)

	if len(merged) != 0 {
		t.Errorf("expected 0 mounts, got %d", len(merged))
	}
}

func TestLoadMountsUserAndProject(t *testing.T) {
	home := t.TempDir()
	writeUserConfig(t, home, `
mounts:
  - source: ~/config
    target: /home/vscode/.config
    read_only: true
`)

	projectDir := t.TempDir()
	writeProjectConfig(t, projectDir, `
mounts:
  - source: /project/data
    target: /data
`)

	setupUserConfigEnv(t, home)

	cfg, err := Load(projectDir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if len(cfg.Mounts) != 2 {
		t.Fatalf("expected 2 mounts (user + project), got %d", len(cfg.Mounts))
	}
	if cfg.Mounts[0].Source != "~/config" {
		t.Errorf("mounts[0].Source = %q, want ~/config (user)", cfg.Mounts[0].Source)
	}
	if cfg.Mounts[1].Source != "/project/data" {
		t.Errorf("mounts[1].Source = %q, want /project/data (project)", cfg.Mounts[1].Source)
	}
}

func TestLoadGitConfigsCustom(t *testing.T) {
	home := t.TempDir()
	writeUserConfig(t, home, `
git:
  inject_configs: true
  configs:
    - ~/.gitconfig
    - ~/.gitignore_global
`)

	setupUserConfigEnv(t, home)

	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if len(cfg.Git.Configs) != 2 {
		t.Fatalf("expected 2 git configs, got %d", len(cfg.Git.Configs))
	}
	if cfg.Git.Configs[0] != "~/.gitconfig" {
		t.Errorf("Git.Configs[0] = %q, want ~/.gitconfig", cfg.Git.Configs[0])
	}
	if cfg.Git.Configs[1] != "~/.gitignore_global" {
		t.Errorf("Git.Configs[1] = %q, want ~/.gitignore_global", cfg.Git.Configs[1])
	}
}
