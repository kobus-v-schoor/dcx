package config

import (
	"fmt"
	"os"
	"strconv"
)

func applyEnvOverrides(cfg *Config) {
	if v, ok := envBool("DCX_SSH_FORWARDING"); ok {
		cfg.SSHForwarding = v
	}
	if v, ok := envBool("DCX_GIT_CONFIG_FORWARDING"); ok {
		cfg.GitConfigForwarding = v
	}
}

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
