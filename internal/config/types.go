package config

type ComposeIntegration struct {
	ComposeFile string `yaml:"compose_file"`
	Strategy    string `yaml:"strategy"`
	DevService  string `yaml:"dev_service"`
}

type Config struct {
	SSHForwarding       bool                `yaml:"ssh_forwarding"`
	GitConfigForwarding bool               `yaml:"git_config_forwarding"`
	ComposeIntegration  *ComposeIntegration `yaml:"compose_integration"`
}

func defaultConfig() *Config {
	return &Config{
		SSHForwarding:       true,
		GitConfigForwarding: true,
	}
}
