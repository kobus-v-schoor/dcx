package ssh

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/kobus-v-schoor/dcx/internal/config"
	"github.com/kobus-v-schoor/dcx/internal/mounts"
)

const (
	// gitMountBase is the container directory under which git config files are
	// bind-mounted. Placed under /opt/dcx/ to avoid conflicts with
	// container-installed software, per the project's mount namespace convention.
	gitMountBase = "/opt/dcx/git"
)

// GitResult holds the mounts and environment variables produced by
// DetectGitConfigs. If no git config files are found or the feature is
// disabled, the slices will be empty. GIT_CONFIG_GLOBAL is set only for the
// first gitconfig file found in the Configs list.
type GitResult struct {
	Mounts   []*mounts.ResolvedMount
	EnvName  string
	EnvValue string
}

// DetectGitConfigs scans the host for git configuration files listed in the
// config. Each existing file is bind-mounted into the container under
// /opt/dcx/git/<basename>. The first file whose basename contains "gitconfig"
// (e.g. ~/.gitconfig) gets its path assigned to GIT_CONFIG_GLOBAL inside the
// container. Missing files are silently skipped with a warning. Called by the
// flags package during dcx up flag assembly when Git.InjectConfigs is true.
func DetectGitConfigs(cfg config.GitConfig) GitResult {
	if !cfg.InjectConfigs {
		return GitResult{}
	}

	configs := cfg.Configs
	if len(configs) == 0 {
		configs = []string{"~/.gitconfig"}
	}

	var result GitResult
	globalSet := false

	for _, rawPath := range configs {
		expanded := expandGitHome(rawPath)
		expanded = filepath.Clean(expanded)

		if _, err := os.Stat(expanded); err != nil {
			slog.Warn("skipping git config: path does not exist", "path", expanded)
			continue
		}

		basename := filepath.Base(expanded)
		target := filepath.Join(gitMountBase, basename)

		result.Mounts = append(result.Mounts, &mounts.ResolvedMount{
			Source:   expanded,
			Target:   target,
			ReadOnly: true,
		})

		// Set GIT_CONFIG_GLOBAL to the first gitconfig-named file only.
		if !globalSet && strings.Contains(basename, "gitconfig") {
			result.EnvName = "GIT_CONFIG_GLOBAL"
			result.EnvValue = target
			globalSet = true
		}
	}

	return result
}

// FormatGitEnv formats the --remote-env flag value for git config forwarding.
// Returns the string in NAME=VALUE format suitable for the devcontainer CLI.
// Returns an empty string if no env var is set (e.g. no gitconfig file was
// found in the config list).
func FormatGitEnv(result GitResult) string {
	if result.EnvName == "" {
		return ""
	}
	return fmt.Sprintf("%s=%s", result.EnvName, result.EnvValue)
}

// expandGitHome replaces a leading ~/ in the path with the user's home
// directory. This is a copy of mounts.expandHome to avoid exporting an
// internal helper; git config paths use the same ~ convention as mount sources.
func expandGitHome(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return path
	}

	return filepath.Join(homeDir, path[2:])
}
