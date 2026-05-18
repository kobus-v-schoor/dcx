package config

import "strings"

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

// EnvVar represents an environment variable passthrough declaration from the
// dcx config. The string format follows one of two forms:
//   - "NAME" — shorthand: reads host env var NAME, sets NAME in the container.
//   - "CONTAINER_NAME=${HOST_VAR}" — explicit: reads HOST_VAR from the host,
//     sets CONTAINER_NAME in the container.
// If the referenced host variable is not set, the entry is silently skipped
// during resolution (no error, no warning). This allows declaring a superset
// of variables where only those existing on the current machine are forwarded.
type EnvVar string

// Config represents the fully-resolved dcx configuration. Bool fields use plain
// types with viper defaults; viper's precedence chain (flag → env → config →
// default) ensures unset fields receive their default value rather than zero.
// ComposeIntegration uses a pointer so nil indicates the block was absent.
type Config struct {
	SSH                SSHConfig           `yaml:"ssh" mapstructure:"ssh"`
	Git                GitConfig           `yaml:"git" mapstructure:"git"`
	ComposeIntegration *ComposeIntegration `yaml:"compose_integration" mapstructure:"compose_integration"`
	DefaultFeatures    []Feature           `yaml:"default_features" mapstructure:"default_features"`
	Mounts             []Mount             `yaml:"mounts" mapstructure:"mounts"`
	Environment        []EnvVar            `yaml:"environment" mapstructure:"environment"`
	LogLevel           string              `yaml:"log_level" mapstructure:"log_level"`
}
