package config

type ComposeIntegration struct {
	ComposeFile string `yaml:"compose_file"`
	Strategy    string `yaml:"strategy"`
	DevService  string `yaml:"dev_service"`
}

type Config struct {
	SSHForwarding       *bool               `yaml:"ssh_forwarding"`
	GitConfigForwarding *bool               `yaml:"git_config_forwarding"`
	ComposeIntegration  *ComposeIntegration `yaml:"compose_integration"`
}

func (c *Config) ApplyDefaults() {
	if c.SSHForwarding == nil {
		v := true
		c.SSHForwarding = &v
	}
	if c.GitConfigForwarding == nil {
		v := true
		c.GitConfigForwarding = &v
	}
}

func (c *Config) SSHForwardingValue() bool {
	if c.SSHForwarding == nil {
		return true
	}
	return *c.SSHForwarding
}

func (c *Config) GitConfigForwardingValue() bool {
	if c.GitConfigForwarding == nil {
		return true
	}
	return *c.GitConfigForwarding
}
