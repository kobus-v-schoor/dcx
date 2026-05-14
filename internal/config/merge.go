package config

func merge(user *Config, project *Config) *Config {
	merged := *user

	if project.ComposeIntegration != nil {
		merged.ComposeIntegration = project.ComposeIntegration
	}

	return &merged
}
