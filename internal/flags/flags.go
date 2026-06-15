package flags

import (
	"path/filepath"

	"github.com/kobus-v-schoor/dcx/internal/config"
	"github.com/kobus-v-schoor/dcx/internal/dcxfeatures"
)

// Build assembles the devcontainer up CLI flags from the resolved config,
// override directory, and rebuild flag. It returns a slice of string
// arguments suitable for passing to exec.Command. When rebuild is true,
// --remove-existing-container is appended so the devcontainer CLI
// recreates the container rather than reusing an existing one — this
// ensures config changes (env vars, mounts, features) take effect.
// Mounts are NOT included here — they are injected via the override
// config's mounts property instead, because the devcontainer CLI's
// --mount flag does not support options like readonly. Called by dcx up
// after config loading and override directory creation.
func Build(workspaceFolder string, cfg *config.Config, overrideDir string, rebuild bool) []string {
	var args []string

	args = append(args, "up")

	args = append(args, "--workspace-folder", workspaceFolder)

	overrideConfigPath := filepath.Join(overrideDir, "devcontainer.json")
	args = append(args, "--override-config", overrideConfigPath)

	args = append(args, buildAdditionalFeatures(cfg)...)

	if rebuild {
		args = append(args, "--remove-existing-container")
	}

	return args
}

// buildAdditionalFeatures returns --additional-features flags if configured.
// Serializes the feature list from config into the JSON format expected by the
// devcontainer CLI. Returns nil when no features are configured.
func buildAdditionalFeatures(cfg *config.Config) []string {
	if len(cfg.DefaultFeatures) == 0 {
		return nil
	}

	jsonStr, err := dcxfeatures.BuildJSON(cfg.DefaultFeatures)
	if err != nil {
		return nil
	}

	return []string{"--additional-features", jsonStr}
}
