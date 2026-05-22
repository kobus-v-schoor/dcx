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
	if cfg.DefaultImage != "mcr.microsoft.com/devcontainers/base:debian" {
		t.Errorf("default DefaultImage = %q, want mcr.microsoft.com/devcontainers/base:debian", cfg.DefaultImage)
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

func TestLoadUserConfigWithXDGConfigHome(t *testing.T) {
	xdgDir := t.TempDir()
	configDir := filepath.Join(xdgDir, "dcx")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(`
ssh:
  forward_agent: false
`), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("XDG_CONFIG_HOME", xdgDir)
	t.Setenv("HOME", t.TempDir()) // ensure HOME fallback is not used

	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.SSH.ForwardAgent {
		t.Error("SSH.ForwardAgent should be false")
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

func TestLoadProjectConfigOverridesUser(t *testing.T) {
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

func TestLoadUserConfigWithEnvironment(t *testing.T) {
	home := t.TempDir()
	writeUserConfig(t, home, `
environment:
  - AWS_ACCESS_KEY_ID
  - SOMETHING_ELSE=${AWS_SECRET_ACCESS_KEY}
`)

	setupUserConfigEnv(t, home)

	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if len(cfg.Environment) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(cfg.Environment))
	}
	if string(cfg.Environment[0]) != "AWS_ACCESS_KEY_ID" {
		t.Errorf("Environment[0] = %q, want AWS_ACCESS_KEY_ID", cfg.Environment[0])
	}
	if string(cfg.Environment[1]) != "SOMETHING_ELSE=${AWS_SECRET_ACCESS_KEY}" {
		t.Errorf("Environment[1] = %q, want SOMETHING_ELSE=${AWS_SECRET_ACCESS_KEY}", cfg.Environment[1])
	}
}

func TestLoadProjectConfigWithEnvironment(t *testing.T) {
	dir := t.TempDir()
	writeProjectConfig(t, dir, `
environment:
  - API_KEY
  - DB_URL=${DATABASE_URL}
`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if len(cfg.Environment) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(cfg.Environment))
	}
	if string(cfg.Environment[0]) != "API_KEY" {
		t.Errorf("Environment[0] = %q, want API_KEY", cfg.Environment[0])
	}
	if string(cfg.Environment[1]) != "DB_URL=${DATABASE_URL}" {
		t.Errorf("Environment[1] = %q, want DB_URL=${DATABASE_URL}", cfg.Environment[1])
	}
}

func TestLoadEnvironmentUserAndProject(t *testing.T) {
	home := t.TempDir()
	writeUserConfig(t, home, `
environment:
  - USER_VAR
`)

	projectDir := t.TempDir()
	writeProjectConfig(t, projectDir, `
environment:
  - PROJECT_VAR
`)

	setupUserConfigEnv(t, home)

	cfg, err := Load(projectDir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if len(cfg.Environment) != 2 {
		t.Fatalf("expected 2 env vars (user + project), got %d", len(cfg.Environment))
	}
	if string(cfg.Environment[0]) != "USER_VAR" {
		t.Errorf("Environment[0] = %q, want USER_VAR (user)", cfg.Environment[0])
	}
	if string(cfg.Environment[1]) != "PROJECT_VAR" {
		t.Errorf("Environment[1] = %q, want PROJECT_VAR (project)", cfg.Environment[1])
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

func TestLoadProxyGitHubDefaults(t *testing.T) {
	dir := t.TempDir()

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// Proxy should default to disabled.
	if cfg.Proxy.GitHub.Enabled {
		t.Error("default Proxy.GitHub.Enabled should be false")
	}
	if cfg.Proxy.GitHub.APIURL != "https://api.github.com" {
		t.Errorf("default Proxy.GitHub.APIURL = %q, want https://api.github.com", cfg.Proxy.GitHub.APIURL)
	}
	if cfg.Proxy.GitHub.CACertPath != "/opt/dcx/gh-proxy/ca.crt" {
		t.Errorf("default Proxy.GitHub.CACertPath = %q, want /opt/dcx/gh-proxy/ca.crt", cfg.Proxy.GitHub.CACertPath)
	}
}

func TestLoadProxyGitHubUserConfig(t *testing.T) {
	home := t.TempDir()
	writeUserConfig(t, home, `
proxy:
  github:
    enabled: true
    bind_addr: "0.0.0.0"
    api_url: "https://github.example.com/api/v3"
`)

	setupUserConfigEnv(t, home)

	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if !cfg.Proxy.GitHub.Enabled {
		t.Error("Proxy.GitHub.Enabled should be true")
	}
	if cfg.Proxy.GitHub.BindAddr != "0.0.0.0" {
		t.Errorf("Proxy.GitHub.BindAddr = %q, want 0.0.0.0", cfg.Proxy.GitHub.BindAddr)
	}
	if cfg.Proxy.GitHub.APIURL != "https://github.example.com/api/v3" {
		t.Errorf("Proxy.GitHub.APIURL = %q, want https://github.example.com/api/v3", cfg.Proxy.GitHub.APIURL)
	}
}

func TestLoadProxyGitHubEnvOverride(t *testing.T) {
	home := t.TempDir()
	writeUserConfig(t, home, `
proxy:
  github:
    enabled: false
`)

	setupUserConfigEnv(t, home)
	t.Setenv("DCX_PROXY_GITHUB_ENABLED", "true")

	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if !cfg.Proxy.GitHub.Enabled {
		t.Error("Proxy.GitHub.Enabled should be true after env override")
	}
}

func TestLoadDefaultImage(t *testing.T) {
	home := t.TempDir()
	writeUserConfig(t, home, `
default_image: mcr.microsoft.com/devcontainers/base:debian
`)

	setupUserConfigEnv(t, home)

	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.DefaultImage != "mcr.microsoft.com/devcontainers/base:debian" {
		t.Errorf("DefaultImage = %q, want mcr.microsoft.com/devcontainers/base:debian", cfg.DefaultImage)
	}
}

func TestLoadDefaultImageEnvOverride(t *testing.T) {
	t.Setenv("DCX_DEFAULT_IMAGE", "ubuntu:22.04")

	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.DefaultImage != "ubuntu:22.04" {
		t.Errorf("DefaultImage = %q, want ubuntu:22.04", cfg.DefaultImage)
	}
}
