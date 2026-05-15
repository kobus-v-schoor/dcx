package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
)

// loadUserConfig reads the user-level config from
// $XDG_CONFIG_HOME/dcx/config.yaml (or ~/.config/dcx/config.yaml).
// Returns defaultConfig() when no file is found.
func loadUserConfig() (*Config, error) {
	path, err := userConfigPath()
	if err != nil {
		return nil, fmt.Errorf("finding user config: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultConfig(), nil
		}
		return nil, fmt.Errorf("reading user config %s: %w", path, err)
	}

	cfg := defaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing user config %s: %w", path, err)
	}

	return cfg, nil
}

// userConfigPath resolves the path to the user config file,
// respecting XDG_CONFIG_HOME when set.
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
