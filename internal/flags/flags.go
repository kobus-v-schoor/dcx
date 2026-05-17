package flags

import (
	"fmt"
	"path/filepath"

	"github.com/kobus-v-schoor/dcx/internal/config"
	"github.com/kobus-v-schoor/dcx/internal/features"
	"github.com/kobus-v-schoor/dcx/internal/mounts"
)

// Build assembles the devcontainer up CLI flags from the resolved config and
// override directory. It returns a slice of string arguments suitable for
// passing to exec.Command. Called by dcx up after config loading and override
// directory creation.
func Build(workspaceFolder string, cfg *config.Config, overrideDir string) []string {
	var args []string

	args = append(args, "up")

	args = append(args, "--workspace-folder", workspaceFolder)

	overrideConfigPath := filepath.Join(overrideDir, "devcontainer.json")
	args = append(args, "--override-config", overrideConfigPath)

	args = append(args, buildAdditionalFeatures(cfg)...)
	args = append(args, buildMounts(cfg)...)
	args = append(args, buildRemoteEnv(cfg)...)

	return args
}

// buildAdditionalFeatures returns --additional-features flags if configured.
// Serializes the feature list from config into the JSON format expected by the
// devcontainer CLI. Returns nil when no features are configured.
func buildAdditionalFeatures(cfg *config.Config) []string {
	if len(cfg.DefaultFeatures) == 0 {
		return nil
	}

	jsonStr, err := features.BuildJSON(cfg.DefaultFeatures)
	if err != nil {
		return nil
	}

	return []string{"--additional-features", jsonStr}
}

// buildMounts returns --mount flags based on config. Resolves user-configured
// bind mounts, expanding ~ and ${VAR} in source paths and skipping mounts
// whose source path doesn't exist on the host. Also includes SSH socket,
// gitconfig, and shell config mounts. Returns nil when no mounts are
// configured or all are skipped.
func buildMounts(cfg *config.Config) []string {
	return mounts.BuildFlags(cfg.Mounts)
}

// buildRemoteEnv returns --remote-env flags for env var passthrough and
// resolved secrets. Placeholder for issue #7.
func buildRemoteEnv(_ *config.Config) []string {
	return nil
}

// FormatRemoteEnv formats a single --remote-env flag value. Exported for use
// by future packages (env, secrets).
func FormatRemoteEnv(name, value string) string {
	return fmt.Sprintf("%s=%s", name, value)
}
