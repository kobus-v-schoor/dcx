package config

import (
	"fmt"
	"os"
	"strconv"
)

// applyEnvOverrides overrides config fields with values from DCX_*
// environment variables. Bool fields use envBool for parsing; string fields
// are set directly when the variable is non-empty. Invalid boolean values are
// warned and ignored.
func applyEnvOverrides(cfg *Config) {
	if v, ok := envBool("DCX_SSH_FORWARDING"); ok {
		cfg.SSHForwarding = boolPtr(v)
	}
	if v, ok := envBool("DCX_GIT_CONFIG_FORWARDING"); ok {
		cfg.GitConfigForwarding = boolPtr(v)
	}
	if v := os.Getenv("DCX_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
}

// envBool parses a boolean environment variable. It returns (value, true) when
// the variable is set and valid, and (false, false) when unset or invalid.
func envBool(key string) (bool, bool) {
	val := os.Getenv(key)
	if val == "" {
		return false, false
	}

	parsed, err := strconv.ParseBool(val)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: ignoring invalid %s=%q: %v\n", key, val, err)
		return false, false
	}

	return parsed, true
}
