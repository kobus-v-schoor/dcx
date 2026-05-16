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

// Config represents the fully-resolved dcx configuration. Bool fields use plain
// types with viper defaults; viper's precedence chain (flag → env → config →
// default) ensures unset fields receive their default value rather than zero.
// ComposeIntegration uses a pointer so nil indicates the block was absent.
type Config struct {
	SSHForwarding       bool                `yaml:"ssh_forwarding" mapstructure:"ssh_forwarding"`
	GitConfigForwarding bool                `yaml:"git_config_forwarding" mapstructure:"git_config_forwarding"`
	ComposeIntegration  *ComposeIntegration `yaml:"compose_integration" mapstructure:"compose_integration"`
	DefaultFeatures     []Feature           `yaml:"default_features" mapstructure:"default_features"`
	LogLevel            string              `yaml:"log_level" mapstructure:"log_level"`
}
