package config

import (
	"strings"
	"time"
)

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

// GitHubProxyConfig controls the GitHub API proxy that injects the host's
// GitHub token into API requests from the devcontainer. When Enabled is true,
// dcx exec starts a local MITM proxy that intercepts HTTPS traffic to GitHub
// domains and injects the host's token as the Authorization header. The proxy
// is fully transparent: container tools use the real GitHub URLs and only
// route through the proxy via HTTP_PROXY/HTTPS_PROXY.
//
// The user's auth token is never exposed inside the container — the proxy
// injects it at the network layer. The token exists only in the host-side dcx
// process memory and is never written to disk or logged.
type GitHubProxyConfig struct {
	// Enabled controls whether the GitHub API proxy is active for dcx exec
	// sessions.
	Enabled bool `yaml:"enabled" mapstructure:"enabled"`

	// Domains is the list of GitHub domains to intercept. When empty, a
	// default set of public GitHub domains is used. Override for GitHub
	// Enterprise Server deployments.
	Domains []string `yaml:"domains" mapstructure:"domains"`
}

// GitLabProxyConfig controls the GitLab API proxy that injects the host's
// GitLab token into API requests from the devcontainer. When Enabled is true,
// dcx exec starts a local MITM proxy that intercepts HTTPS traffic to GitLab
// domains and injects the host's token as the Authorization header. The proxy
// is fully transparent: container tools use the real GitLab URLs and only
// route through the proxy via HTTP_PROXY/HTTPS_PROXY.
//
// The user's auth token is never exposed inside the container — the proxy
// injects it at the network layer. The token exists only in the host-side dcx
// process memory and is never written to disk or logged.
type GitLabProxyConfig struct {
	// Enabled controls whether the GitLab API proxy is active for dcx exec
	// sessions.
	Enabled bool `yaml:"enabled" mapstructure:"enabled"`

	// Domains is the list of GitLab domains to intercept. When empty, a
	// default set of public GitLab domains is used. Override for GitLab
	// self-managed deployments.
	Domains []string `yaml:"domains" mapstructure:"domains"`
}

// ProxyConfig holds the configuration for all proxy services. Each service
// (e.g. GitHub, GitLab) gets its own sub-configuration and listens on a
// separate port. This structure maps to the "proxy:" block in the YAML
// config file.
type ProxyConfig struct {
	// BindAddr is the address the proxy listens on. Defaults to the gateway IP
	// (more secure — only reachable from the container's network). Set to
	// "0.0.0.0" to listen on all interfaces.
	BindAddr string `yaml:"bind_addr" mapstructure:"bind_addr"`

	// CertExpiry is the validity duration for the generated CA certificate.
	// Defaults to 24 hours. The certificate is ephemeral — it only needs to
	// last for the duration of a dcx exec session.
	CertExpiry time.Duration `yaml:"cert_expiry" mapstructure:"cert_expiry"`

	// GitHub controls the GitHub API reverse proxy.
	GitHub GitHubProxyConfig `yaml:"github" mapstructure:"github"`

	// GitLab controls the GitLab API reverse proxy.
	GitLab GitLabProxyConfig `yaml:"gitlab" mapstructure:"gitlab"`
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
type Config struct {
	// WorkspaceFolder is the absolute path to the resolved project root.
	// When the starting directory or one of its ancestors contains a
	// .devcontainer directory, WorkspaceFolder points to that ancestor.
	// Otherwise it falls back to the original cwd passed to Load.
	WorkspaceFolder string

	Proxy           ProxyConfig `yaml:"proxy" mapstructure:"proxy"`
	SSH             SSHConfig   `yaml:"ssh" mapstructure:"ssh"`
	Git             GitConfig   `yaml:"git" mapstructure:"git"`
	DefaultFeatures []Feature   `yaml:"default_features" mapstructure:"default_features"`
	Mounts          []Mount     `yaml:"mounts" mapstructure:"mounts"`
	Environment     []EnvVar    `yaml:"environment" mapstructure:"environment"`
	LogLevel        string      `yaml:"log_level" mapstructure:"log_level"`
	// DefaultImage is the image to use when the workspace has no
	// devcontainer.json. When set, dcx up generates a minimal temporary
	// devcontainer.json containing only the image field. When empty and no
	// devcontainer.json exists, dcx up returns an error.
	DefaultImage string `yaml:"default_image" mapstructure:"default_image"`

	// DefaultShell is the shell to run when dcx exec is invoked without a
	// command. Defaults to the basename of the host's $SHELL environment
	// variable, falling back to "bash" when $SHELL is unset.
	DefaultShell string `yaml:"default_shell" mapstructure:"default_shell"`
}
