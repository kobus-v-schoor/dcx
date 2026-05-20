package config

import (
	"strings"
	"time"
)

// ComposeIntegration holds settings for Docker Compose integration strategies.
// Pointer fields distinguish "not set" (nil) from a zero value during viper
// unmarshalling — a nil ComposeIntegration means the user provided no
// compose_integration block at all.
type ComposeIntegration struct {
	ComposeFile string `yaml:"compose_file" mapstructure:"compose_file"`
	Strategy    string `yaml:"strategy" mapstructure:"strategy"`
	DevService  string `yaml:"dev_service" mapstructure:"dev_service"`
}

// Feature represents a devcontainer feature to inject via --additional-features.
// ID is the feature identifier (e.g. "ghcr.io/devcontainers/features/github-cli").
// Options holds feature-specific key-value pairs; an empty or nil map serializes
// as "{}" in the resulting JSON.
type Feature struct {
	ID      string                 `yaml:"id" mapstructure:"id"`
	Options map[string]interface{} `yaml:"options" mapstructure:"options"`
}

// FeatureID returns the effective feature ID for serialization. If the ID does
// not already contain a version tag (a colon followed by a non-empty segment),
// ":latest" is appended so the devcontainer CLI can resolve it.
func (f Feature) FeatureID() string {
	if idx := strings.LastIndex(f.ID, ":"); idx >= 0 {
		if idx < len(f.ID)-1 && !strings.Contains(f.ID[idx+1:], "/") {
			return f.ID
		}
	}
	return f.ID + ":latest"
}

// Mount represents a user-configured bind mount declaration. Source and Target
// are the host and container paths respectively. ReadOnly controls whether the
// mount is read-only; it defaults to false when not specified. Serialized as a
// Docker --mount flag by the mounts package.
type Mount struct {
	Source   string `yaml:"source" mapstructure:"source"`
	Target   string `yaml:"target" mapstructure:"target"`
	ReadOnly bool   `yaml:"read_only" mapstructure:"read_only"`
}

// SSHConfig controls automatic SSH agent forwarding from the host into the
// dev container. When ForwardAgent is true (the default), dcx reads
// SSH_AUTH_SOCK from the host environment and bind-mounts the socket into the
// container, then sets SSH_AUTH_SOCK inside the container to point at the
// mounted path. AgentSocketTarget is the container path for the bind mount;
// it defaults to /opt/dcx/sockets/ssh-agent.sock. If the socket is missing
// or SSH_AUTH_SOCK is unset, the forwarding is silently skipped with a
// warning.
type SSHConfig struct {
	ForwardAgent      bool   `yaml:"forward_agent" mapstructure:"forward_agent"`
	AgentSocketTarget string `yaml:"agent_socket_target" mapstructure:"agent_socket_target"`
}

// GitConfig controls automatic git configuration forwarding from the host into
// the dev container. When InjectConfigs is true (the default), dcx bind-mounts
// each file listed in Configs into <MountBase>/<index>-<basename> inside the
// container and sets GIT_CONFIG_GLOBAL to the first mounted file's path.
// Configs defaults to ["~/.gitconfig"]; MountBase defaults to "/opt/dcx/git".
// Missing files are silently skipped with a warning.
type GitConfig struct {
	InjectConfigs bool     `yaml:"inject_configs" mapstructure:"inject_configs"`
	Configs       []string `yaml:"configs" mapstructure:"configs"`
	MountBase     string   `yaml:"mount_base" mapstructure:"mount_base"`
}

// GitHubCLIConfig controls the GitHub CLI reverse proxy that injects the
// host's GitHub token into gh CLI requests from the devcontainer. When
// Enabled is true, dcx exec starts a local HTTPS reverse proxy inside the
// dcx process. The gh CLI inside the container is configured (via GH_HOST
// and related env vars) to route all requests through this proxy.
//
// The user's auth token is never exposed inside the container — the proxy
// injects it as an Authorization header on forwarded requests, replacing
// whatever token the container-side gh CLI provides. The token exists only
// in the host-side dcx process memory and is never written to disk or logged.
//
// Note: The proxy does not enforce repository-level scoping. It forwards
// all requests to the GitHub API with the host token. The purpose of the
// proxy is to keep the token on the host side and inject it at the network
// layer, not to restrict access to specific repositories.
type GitHubCLIConfig struct {
	// Enabled controls whether the GitHub CLI proxy is active for dcx exec
	// sessions.
	Enabled bool `yaml:"enabled" mapstructure:"enabled"`

	// BindAddr is the address the proxy listens on. Defaults to the gateway IP
	// (more secure — only reachable from the container's network). Set to
	// "0.0.0.0" to listen on all interfaces.
	BindAddr string `yaml:"bind_addr" mapstructure:"bind_addr"`

	// APIURL is the GitHub API URL to forward requests to. Defaults to
	// "https://api.github.com". Override for GitHub Enterprise Server.
	APIURL string `yaml:"api_url" mapstructure:"api_url"`

	// CACertPath is the container path where the CA certificate is copied.
	// Defaults to "/opt/dcx/gh-proxy/ca.crt".
	CACertPath string `yaml:"ca_cert_path" mapstructure:"ca_cert_path"`

	// CertExpiry is the validity duration for the generated TLS certificates.
	// Defaults to 24 hours. The certificates are ephemeral — they only need to
	// last for the duration of a dcx exec session.
	CertExpiry time.Duration `yaml:"cert_expiry" mapstructure:"cert_expiry"`
}

// EnvVar represents an environment variable passthrough declaration from the
// dcx config. The string format follows one of two forms:
//   - "NAME" — shorthand: reads host env var NAME, sets NAME in the container.
//   - "CONTAINER_NAME=${HOST_VAR}" — explicit: reads HOST_VAR from the host,
//     sets CONTAINER_NAME in the container.
//
// The value part (after '=') supports composite expressions that mix
// substitutions and literal text, e.g. "PATH=${PATH}:/opt/bin". If the value
// contains no ${...} references, it is treated as a plain literal string.
// If a referenced host variable is not set, a warning is logged and the
// reference is substituted with an empty string.
type EnvVar string

// Config represents the fully-resolved dcx configuration. Bool fields use plain
// types with viper defaults; viper's precedence chain (flag → env → config →
// default) ensures unset fields receive their default value rather than zero.
// ComposeIntegration uses a pointer so nil indicates the block was absent.
type Config struct {
	GitHubCLI          GitHubCLIConfig     `yaml:"github_cli" mapstructure:"github_cli"`
	SSH                SSHConfig           `yaml:"ssh" mapstructure:"ssh"`
	Git                GitConfig           `yaml:"git" mapstructure:"git"`
	ComposeIntegration *ComposeIntegration `yaml:"compose_integration" mapstructure:"compose_integration"`
	DefaultFeatures    []Feature           `yaml:"default_features" mapstructure:"default_features"`
	Mounts             []Mount             `yaml:"mounts" mapstructure:"mounts"`
	Environment        []EnvVar            `yaml:"environment" mapstructure:"environment"`
	LogLevel           string              `yaml:"log_level" mapstructure:"log_level"`
}
