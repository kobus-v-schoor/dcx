package config

import (
	"fmt"
	"path/filepath"
)

func Load(cwd string) (*Config, error) {
	absCWD, err := filepath.Abs(cwd)
	if err != nil {
		return nil, fmt.Errorf("resolving path %s: %w", cwd, err)
	}

	user, err := loadUserConfig()
	if err != nil {
		return nil, err
	}

	project, err := loadProjectConfig(absCWD)
	if err != nil {
		return nil, err
	}

	cfg := merge(user, project)
	applyEnvOverrides(cfg)

	return cfg, nil
}
