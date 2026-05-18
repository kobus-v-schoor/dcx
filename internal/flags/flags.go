package flags

import (
	"path/filepath"

	"github.com/kobus-v-schoor/dcx/internal/config"
	"github.com/kobus-v-schoor/dcx/internal/features"
	"github.com/kobus-v-schoor/dcx/internal/git"
	"github.com/kobus-v-schoor/dcx/internal/mounts"
	"github.com/kobus-v-schoor/dcx/internal/ssh"
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

// buildMounts returns --mount flags based on config and auto-detected mounts.
// It resolves user-configured bind mounts (expanding ~ and ${VAR} in source
// paths, skipping non-existent sources), then appends SSH agent and git config
// auto-detection mounts when enabled. Returns nil when no mounts are produced.
func buildMounts(cfg *config.Config) []string {
	var flags []string

	flags = append(flags, mounts.BuildFlags(cfg.Mounts)...)

	if cfg.SSH.ForwardAgent {
		agentResult := ssh.DetectAgent(cfg.SSH)
		if agentResult.Mount != nil {
			flags = append(flags, "--mount", mounts.Format(*agentResult.Mount))
		}
	}

	if cfg.Git.InjectConfigs {
		gitResult := git.DetectConfigs(cfg.Git)
		for _, m := range gitResult.Mounts {
			flags = append(flags, "--mount", mounts.Format(*m))
		}
	}

	if len(flags) == 0 {
		return nil
	}

	return flags
}
