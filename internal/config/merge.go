package config

import "dario.cat/mergo"

// merge combines user and project configs. Project-level values override
// user-level values only when explicitly set (non-nil). Fields left unset in
// the project config are inherited from the user config.
func merge(user *Config, project *Config) *Config {
	merged := *user
	_ = mergo.Merge(&merged, project, mergo.WithOverride)
	return &merged
}
