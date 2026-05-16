package config

import "dario.cat/mergo"

// merge combines user and project configs. Project-level values override
// user-level values only when explicitly set (non-nil). Fields left unset in
// the project config are inherited from the user config.
//
// DefaultFeatures uses union semantics: both user and project features are
// combined. When a feature ID appears in both, the project feature wins.
func merge(user *Config, project *Config) *Config {
	merged := *user
	_ = mergo.Merge(&merged, project, mergo.WithOverride)

	merged.DefaultFeatures = mergeFeatures(user.DefaultFeatures, project.DefaultFeatures)

	return &merged
}

// mergeFeatures combines user and project feature lists with union semantics.
// User features form the base; project features are appended, replacing any
// user feature with the same ID. The order is: all user features (preserving
// their order), then project-only features (in project order).
func mergeFeatures(user, project []Feature) []Feature {
	if len(user) == 0 {
		return project
	}
	if len(project) == 0 {
		return user
	}

	seen := make(map[string]int, len(user)+len(project))
	result := make([]Feature, 0, len(user)+len(project))

	for _, f := range user {
		result = append(result, f)
		seen[f.ID] = len(result) - 1
	}

	for _, f := range project {
		if idx, ok := seen[f.ID]; ok {
			result[idx] = f
		} else {
			result = append(result, f)
		}
	}

	return result
}
