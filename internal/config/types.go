package config

// ComposeIntegration holds settings for Docker Compose integration strategies.
type ComposeIntegration struct {
	ComposeFile string `yaml:"compose_file"`
	Strategy    string `yaml:"strategy"`
	DevService  string `yaml:"dev_service"`
}

// Config represents the fully-resolved dcx configuration. Bool fields use
// pointer types so nil indicates "not set" — this allows merge to distinguish
// between an explicitly-set false and an unset field that should inherit the
// user-level value.
type Config struct {
	SSHForwarding       *bool               `yaml:"ssh_forwarding"`
	GitConfigForwarding *bool               `yaml:"git_config_forwarding"`
	ComposeIntegration  *ComposeIntegration `yaml:"compose_integration"`
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
