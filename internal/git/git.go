package git

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/kobus-v-schoor/dcx/internal/config"
	"github.com/kobus-v-schoor/dcx/internal/mounts"
)

// GitResult holds the mounts and environment variables produced by
// DetectConfigs. If no git config files are found or the feature is
// disabled, the slices will be empty. GIT_CONFIG_GLOBAL is set for the first
// configured file that exists on the host, regardless of its basename.
type GitResult struct {
	Mounts   []*mounts.ResolvedMount
	EnvName  string
	EnvValue string
}

// DetectConfigs scans the host for git configuration files listed in the
// config. Each existing file is bind-mounted into the container under
// <mountBase>/<index>-<basename>. The index prefix prevents collisions when
// multiple files share the same basename. The first file in the Configs list
// that exists on the host gets its container path assigned to
// GIT_CONFIG_GLOBAL, regardless of its filename. Missing files are silently
// skipped with a warning. When Configs is empty, no mounts or env vars are
// produced (InjectConfigs is effectively false). Called by the flags package
// during dcx up flag assembly when Git.InjectConfigs is true. The mount base
// directory is read from cfg.MountBase (defaulted by the config package to
// /opt/dcx/git).
func DetectConfigs(cfg config.GitConfig) GitResult {
	if !cfg.InjectConfigs || len(cfg.Configs) == 0 {
		return GitResult{}
	}

	mountBase := cfg.MountBase

	var result GitResult
	globalSet := false
	mountIdx := 0

	for _, rawPath := range cfg.Configs {
		expanded := mounts.ExpandHome(rawPath)
		expanded = filepath.Clean(expanded)

		if _, err := os.Stat(expanded); err != nil {
			slog.Warn("skipping git config: path does not exist", "path", expanded)
			continue
		}

		basename := filepath.Base(expanded)
		target := filepath.Join(mountBase, fmt.Sprintf("%d-%s", mountIdx, basename))
		mountIdx++

		result.Mounts = append(result.Mounts, &mounts.ResolvedMount{
			Source:   expanded,
			Target:   target,
			ReadOnly: true,
		})

		if !globalSet {
			result.EnvName = "GIT_CONFIG_GLOBAL"
			result.EnvValue = target
			globalSet = true
		}
	}

	return result
}
