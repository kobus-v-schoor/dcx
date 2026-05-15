package flags

import (
	"fmt"
	"path/filepath"

	"github.com/kobus-v-schoor/dcx/internal/config"
)

// Build assembles the devcontainer up CLI flags from the resolved config and
// override directory. It returns a slice of string arguments suitable for
// passing to exec.Command. Scope: CLI flag construction. Called by dcx up after
// config loading and override directory creation.
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
// Placeholder for issue #5. Scope: feature flag generation.
func buildAdditionalFeatures(_ *config.Config) []string {
	return nil
}

// buildMounts returns --mount flags based on config (SSH socket, gitconfig,
// shell configs). Placeholder for issues #6 and #8. Scope: mount flag
// generation.
func buildMounts(_ *config.Config) []string {
	return nil
}

// buildRemoteEnv returns --remote-env flags for env var passthrough and
// resolved secrets. Placeholder for issue #7. Scope: remote-env flag
// generation.
func buildRemoteEnv(_ *config.Config) []string {
	return nil
}

// FormatRemoteEnv formats a single --remote-env flag value. Exported for use
// by future packages (env, secrets). Scope: flag value formatting.
func FormatRemoteEnv(name, value string) string {
	return fmt.Sprintf("%s=%s", name, value)
}
