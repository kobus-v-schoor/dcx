package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Load reads and merges configuration from user-level, project-level, and
// environment sources (in increasing precedence order). It uses viper to handle
// YAML parsing, environment variable binding, and config merging. The returned
// Config has the fully-resolved values ready for use by the CLI.
//
// The precedence chain (low to high) is: defaults → user config → project
// config → environment variables → CLI flags. Viper manages this chain
// natively; the only custom post-processing is the union-merge of
// DefaultFeatures (project wins on ID conflict) and compose_file path
// validation.
func Load(cwd string) (*Config, error) {
	absCWD, err := filepath.Abs(cwd)
	if err != nil {
		return nil, fmt.Errorf("resolving path %s: %w", cwd, err)
	}

	v := viper.New()

	v.SetEnvPrefix("DCX")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Set defaults for fields that have meaningful non-zero defaults.
	// Setting a default also registers the key with viper, enabling
	// AutomaticEnv to resolve DCX_* env vars for that key even when the
	// corresponding YAML key is absent from all config files. Keys without
	// explicit defaults are registered automatically when a config file
	// provides a value.
	v.SetDefault("ssh.forward_agent", true)
	v.SetDefault("ssh.agent_socket_target", "/opt/dcx/sockets/ssh-agent.sock")
	v.SetDefault("git.inject_configs", true)
	v.SetDefault("git.configs", []string{"~/.gitconfig"})
	v.SetDefault("git.mount_base", "/opt/dcx/git")
	v.SetDefault("log_level", "")

	// Capture user-level features, mounts, and environment before project
	// config is merged on top. Viper replaces slices on merge rather than
	// union-merging them, so we need these lists separately for our custom
	// merge logic.
	userCaptured, err := loadAndCaptureUserConfig(v)
	if err != nil {
		return nil, err
	}

	// Merge project config on top of user config. Viper's MergeInConfig
	// replaces values at the same key path, which matches the project >
	// user precedence rule.
	projectCaptured, err := mergeProjectConfig(v, absCWD)
	if err != nil {
		return nil, err
	}

	// Validate compose_file path if present.
	if v.IsSet("compose_integration.compose_file") {
		composeFile := v.GetString("compose_integration.compose_file")
		if err := validateComposeFilePath(absCWD, composeFile); err != nil {
			return nil, err
		}
	}

	// Unmarshal the fully-merged viper state into the Config struct.
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}

	// Apply custom union-merge for features: both user and project features
	// are combined; project wins on ID conflict.
	cfg.DefaultFeatures = mergeFeatures(userCaptured.Features, projectCaptured.Features)

	// Concatenate user and project mounts; warn on duplicate targets but let
	// Docker handle the conflict at runtime.
	cfg.Mounts = mergeMounts(userCaptured.Mounts, projectCaptured.Mounts)

	// Concatenate user and project environment variables; later entries for
	// the same container-side name take precedence (project wins over user).
	cfg.Environment = mergeEnvVars(userCaptured.Environment, projectCaptured.Environment)

	return &cfg, nil
}

// capturedConfig holds the features, mounts, and environment variables
// extracted from a config source (user or project) before viper's merge
// logic overwrites them. Using a struct avoids unwieldy multi-return
// signatures and makes it straightforward to add new captured fields later.
type capturedConfig struct {
	Features     []Feature
	Mounts       []Mount
	Environment  []EnvVar
}

// loadAndCaptureUserConfig reads the user-level config from
// $XDG_CONFIG_HOME/dcx/config.yaml (or ~/.config/dcx/config.yaml) into the
// viper instance. It returns the user's DefaultFeatures, Mounts, and
// Environment before any project config overwrites them, so the caller can
// apply custom merge logic. Returns nil features/mounts/env when no user
// config file exists.
func loadAndCaptureUserConfig(v *viper.Viper) (*capturedConfig, error) {
	configPath, err := userConfigDir()
	if err != nil {
		return nil, fmt.Errorf("finding user config directory: %w", err)
	}

	v.SetConfigName("config")
	v.AddConfigPath(configPath)

	// ReadInConfig returns an error for missing files; treat that as "no
	// user config" rather than a fatal error.
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading user config: %w", err)
		}
		// No user config file — return empty captured config, viper defaults
		// still apply.
		return &capturedConfig{}, nil
	}

	// Capture the user features and mounts before project config overwrites
	// the keys.
	var captured capturedConfig

	if v.IsSet("default_features") {
		if err := v.UnmarshalKey("default_features", &captured.Features); err != nil {
			return nil, fmt.Errorf("parsing user default_features: %w", err)
		}
	}

	if v.IsSet("mounts") {
		if err := v.UnmarshalKey("mounts", &captured.Mounts); err != nil {
			return nil, fmt.Errorf("parsing user mounts: %w", err)
		}
	}

	if v.IsSet("environment") {
		if err := v.UnmarshalKey("environment", &captured.Environment); err != nil {
			return nil, fmt.Errorf("parsing user environment: %w", err)
		}
	}

	return &captured, nil
}

// mergeProjectConfig merges the project-level config from
// <cwd>/.devcontainer/dcx.yaml into the viper instance. It returns the
// project's DefaultFeatures, Mounts, and Environment so the caller can apply
// custom merge logic. Returns an empty capturedConfig when no project config
// file exists.
func mergeProjectConfig(v *viper.Viper, cwd string) (*capturedConfig, error) {
	v.SetConfigName("dcx")
	v.AddConfigPath(filepath.Join(cwd, ".devcontainer"))

	// Reset the config name/path so we read from the project location.
	// MergeInConfig merges on top of existing values.
	if err := v.MergeInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading project config: %w", err)
		}
		// No project config file — return empty captured config.
		return &capturedConfig{}, nil
	}

	var captured capturedConfig

	if v.IsSet("default_features") {
		if err := v.UnmarshalKey("default_features", &captured.Features); err != nil {
			return nil, fmt.Errorf("parsing project default_features: %w", err)
		}
	}

	if v.IsSet("mounts") {
		if err := v.UnmarshalKey("mounts", &captured.Mounts); err != nil {
			return nil, fmt.Errorf("parsing project mounts: %w", err)
		}
	}

	if v.IsSet("environment") {
		if err := v.UnmarshalKey("environment", &captured.Environment); err != nil {
			return nil, fmt.Errorf("parsing project environment: %w", err)
		}
	}

	return &captured, nil
}

// userConfigDir resolves the directory containing the user config file.
// os.UserConfigDir respects XDG_CONFIG_HOME on Unix and the appropriate
// AppData directory on Windows, so no manual XDG handling is needed.
// Returns the directory (not the file path), since viper's SetConfigName +
// AddConfigPath expect a directory.
func userConfigDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("determining user config directory: %w", err)
	}
	return filepath.Join(dir, "dcx"), nil
}

// validateComposeFilePath warns if the compose_file path resolves outside
// the project directory. This is a validation step called after config loading
// to catch misconfigured paths that might reference files outside the project.
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

// mergeMounts concatenates user and project mount lists. Unlike features,
// mounts are not union-merged on conflict — Docker will handle duplicate
// targets at runtime. Duplicate target warnings are emitted by the mounts
// package during flag assembly. The order is: all user mounts first, then all
// project mounts.
func mergeMounts(user, project []Mount) []Mount {
	if len(user) == 0 {
		return project
	}
	if len(project) == 0 {
		return user
	}

	result := make([]Mount, 0, len(user)+len(project))
	result = append(result, user...)
	result = append(result, project...)
	return result
}

// mergeEnvVars concatenates user and project environment variable lists.
// Like mounts, env vars are concatenated rather than union-merged on conflict.
// When the same container-side name appears in both user and project lists,
// the project entry appears later in the --remote-env flags and therefore
// takes precedence (the devcontainer CLI uses the last value for duplicate
// names). The order is: all user env vars first, then all project env vars.
func mergeEnvVars(user, project []EnvVar) []EnvVar {
	if len(user) == 0 {
		return project
	}
	if len(project) == 0 {
		return user
	}

	result := make([]EnvVar, 0, len(user)+len(project))
	result = append(result, user...)
	result = append(result, project...)
	return result
}
