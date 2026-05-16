package config

import "strings"

// ComposeIntegration holds settings for Docker Compose integration strategies.
type ComposeIntegration struct {
	ComposeFile string `yaml:"compose_file"`
	Strategy    string `yaml:"strategy"`
	DevService  string `yaml:"dev_service"`
}

// Feature represents a devcontainer feature to inject via --additional-features.
// ID is the feature identifier (e.g. "ghcr.io/devcontainers/features/github-cli").
// Options holds feature-specific key-value pairs; an empty or nil map serializes
// as "{}" in the resulting JSON.
type Feature struct {
	ID      string                 `yaml:"id"`
	Options map[string]interface{} `yaml:"options"`
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

// Config represents the fully-resolved dcx configuration. Bool fields use
// pointer types so nil indicates "not set" — this allows merge to distinguish
// between an explicitly-set false and an unset field that should inherit the
// user-level value.
type Config struct {
	SSHForwarding       *bool               `yaml:"ssh_forwarding"`
	GitConfigForwarding *bool               `yaml:"git_config_forwarding"`
	ComposeIntegration  *ComposeIntegration `yaml:"compose_integration"`
	DefaultFeatures     []Feature           `yaml:"default_features"`
}

// boolPtr returns a pointer to the given bool value. Used to construct *bool
// fields where nil means "not set" and the pointer value is the explicit setting.
func boolPtr(v bool) *bool { return &v }

func defaultConfig() *Config {
	return &Config{
		SSHForwarding:       boolPtr(true),
		GitConfigForwarding: boolPtr(true),
	}
}
