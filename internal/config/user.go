package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
)

func loadUserConfig() (*Config, error) {
	path, err := userConfigPath()
	if err != nil {
		return nil, fmt.Errorf("finding user config: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := &Config{}
			cfg.ApplyDefaults()
			return cfg, nil
		}
		return nil, fmt.Errorf("reading user config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing user config %s: %w", path, err)
	}

	cfg.ApplyDefaults()
	return &cfg, nil
}

func userConfigPath() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "dcx", "config.yaml"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determining home directory: %w", err)
	}

	return filepath.Join(home, ".config", "dcx", "config.yaml"), nil
}
