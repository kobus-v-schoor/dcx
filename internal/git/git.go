package git

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

// DetectRemoteURL returns the raw URL of the origin remote. It runs
// `git remote get-url origin` in the current directory. Returns the URL
// string and true if successfully detected, or empty string and false if
// not in a git repository or the command fails.
func DetectRemoteURL() (string, bool) {
	out, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

// DetectRepo detects the current repository's owner/name from the git remote
// URL of the origin remote. It runs `git remote get-url origin` in the
// current directory and parses the output. Returns "owner/repo" and true if
// successfully detected, or empty string and false if not in a git repository
// or the remote URL cannot be parsed.
func DetectRepo() (string, bool) {
	remoteURL, ok := DetectRemoteURL()
	if !ok {
		return "", false
	}
	return RepoFromURL(remoteURL)
}

// RepoFromURL extracts the "owner/repo" identifier from a Git remote URL.
// Supports HTTPS (https://github.com/owner/repo.git) and SSH
// (git@github.com:owner/repo.git or ssh://git@github.com/owner/repo.git)
// formats. Returns the owner/repo string and true on success, or empty
// string and false if the URL cannot be parsed.
func RepoFromURL(rawURL string) (string, bool) {
	// Trim the .git suffix if present.
	rawURL = strings.TrimSuffix(rawURL, ".git")

	// Handle the scp-like SSH format: git@host:path
	// This is not a valid URL per url.Parse (the colon before the path
	// makes it fail), so we detect it explicitly.
	if strings.Contains(rawURL, "@") && !strings.HasPrefix(rawURL, "ssh://") && !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		idx := strings.Index(rawURL, ":")
		if idx == -1 {
			return "", false
		}
		path := rawURL[idx+1:]
		segments := strings.Split(strings.Trim(path, "/"), "/")
		if len(segments) >= 2 {
			return segments[len(segments)-2] + "/" + segments[len(segments)-1], true
		}
		return "", false
	}

	// Handle standard URL formats (https://, ssh://, etc.).
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", false
	}

	path := strings.Trim(u.Path, "/")
	segments := strings.Split(path, "/")
	if len(segments) >= 2 {
		return segments[len(segments)-2] + "/" + segments[len(segments)-1], true
	}

	return "", false
}
