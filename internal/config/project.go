package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
)

func loadProjectConfig(cwd string) (*Config, error) {
	path := filepath.Join(cwd, ".devcontainer", "dcx.yaml")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("reading project config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing project config %s: %w", path, err)
	}

	if cfg.ComposeIntegration != nil && cfg.ComposeIntegration.ComposeFile != "" {
		if err := validateComposeFilePath(cwd, cfg.ComposeIntegration.ComposeFile); err != nil {
			return nil, err
		}
	}

	return &cfg, nil
}

func validateComposeFilePath(cwd, composeFile string) error {
	resolved := filepath.Clean(filepath.Join(cwd, composeFile))
	absCWD, err := filepath.Abs(cwd)
	if err != nil {
		return fmt.Errorf("resolving cwd: %w", err)
	}

	rel, err := filepath.Rel(absCWD, resolved)
	if err != nil {
		return fmt.Errorf("resolving compose_file path: %w", err)
	}

	if rel == ".." || len(rel) >= 3 && rel[0:3] == "../" {
		_, _ = fmt.Fprintf(os.Stderr, "warning: compose_file %q resolves outside the project directory (%s)\n", composeFile, absCWD)
	}

	return nil
}
