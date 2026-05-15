package config

import "dario.cat/mergo"

func merge(user *Config, project *Config) *Config {
	merged := *user

	if project.ComposeIntegration != nil {
		if merged.ComposeIntegration == nil {
			merged.ComposeIntegration = &ComposeIntegration{}
		}
		_ = mergo.Merge(merged.ComposeIntegration, project.ComposeIntegration, mergo.WithOverride)
	}

	return &merged
}
