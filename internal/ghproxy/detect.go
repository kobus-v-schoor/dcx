// Package ghproxy implements a reverse proxy for the GitHub API that enforces
// repository-level scoping on the user's GitHub token. The proxy runs inside
// the dcx process, listens on HTTPS with a self-signed certificate, and
// forwards allowed requests to api.github.com after rewriting the Host header
// and injecting the host's GitHub token. Requests targeting repositories other
// than the configured one are rejected with 403 Forbidden.
package ghproxy

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

// DetectToken reads the user's GitHub token from the host environment. It
// checks three sources in order: (1) the GH_TOKEN environment variable,
// (2) the GITHUB_TOKEN environment variable, and (3) the gh auth token CLI
// command. Returns the token string and true if found, or empty string and
// false if no token is available. The token is never logged per project
// security rules. Called by dcx exec when the github_cli proxy is enabled.
func DetectToken() (string, bool) {
	// GH_TOKEN takes precedence over GITHUB_TOKEN, matching the gh CLI's own
	// precedence. See: https://cli.github.com/manual/gh_help_environment
	if token := os.Getenv("GH_TOKEN"); token != "" {
		slog.Debug("using GH_TOKEN for GitHub API proxy")
		return token, true
	}

	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		slog.Debug("using GITHUB_TOKEN for GitHub API proxy")
		return token, true
	}

	// Fall back to gh auth token, which reads the token from the gh CLI's
	// own credential store.
	path, err := exec.LookPath("gh")
	if err != nil {
		slog.Warn("gh CLI not found on PATH, cannot detect GitHub token")
		return "", false
	}

	out, err := exec.Command(path, "auth", "token").Output()
	if err != nil {
		slog.Warn("gh auth token failed, no GitHub token available", "error", err)
		return "", false
	}

	token := strings.TrimSpace(string(out))
	if token == "" {
		slog.Warn("gh auth token returned empty string")
		return "", false
	}

	slog.Debug("using gh auth token for GitHub API proxy")
	return token, true
}

// DetectRepository extracts the repository in "owner/repo" format from the
// git remote (origin URL) in the given workspace directory. It supports both
// HTTPS and SSH remote URL formats. Returns the repository string and true if
// found, or empty string and false if git is not available, the directory is
// not a git repository, or there is no origin remote. Called by dcx exec when
// the github_cli config has an empty repository field (auto-detection).
func DetectRepository(workspaceDir string) (string, bool) {
	path, err := exec.LookPath("git")
	if err != nil {
		slog.Warn("git not found on PATH, cannot detect repository")
		return "", false
	}

	out, err := exec.Command(path, "-C", workspaceDir, "remote", "get-url", "origin").Output()
	if err != nil {
		slog.Warn("cannot detect git remote origin", "dir", workspaceDir, "error", err)
		return "", false
	}

	remoteURL := strings.TrimSpace(string(out))
	repo, err := parseGitRemoteURL(remoteURL)
	if err != nil {
		slog.Warn("cannot parse git remote URL", "url", remoteURL, "error", err)
		return "", false
	}

	slog.Debug("detected repository from git remote", "repository", repo)
	return repo, true
}

// parseGitRemoteURL extracts the "owner/repo" segment from a git remote URL.
// Supports three common formats:
//   - HTTPS: https://github.com/owner/repo.git
//   - SSH:   git@github.com:owner/repo.git
//   - SSH with scheme: ssh://git@github.com/owner/repo.git
//
// The .git suffix is stripped if present. Returns an error for URLs that
// cannot be parsed or do not contain a recognizable owner/repo segment.
func parseGitRemoteURL(rawURL string) (string, error) {
	// Strip trailing .git suffix if present.
	u := strings.TrimSuffix(rawURL, ".git")

	// SSH format: git@github.com:owner/repo
	if strings.Contains(u, ":") && !strings.HasPrefix(u, "http") && !strings.HasPrefix(u, "ssh://") {
		parts := strings.SplitN(u, ":", 2)
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid SSH URL: %s", rawURL)
		}
		path := parts[1]
		return extractRepoFromPath(path)
	}

	// ssh://git@github.com/owner/repo
	if strings.HasPrefix(u, "ssh://") {
		parts := strings.SplitN(u, "/", 4)
		if len(parts) < 4 {
			return "", fmt.Errorf("invalid SSH URL: %s", rawURL)
		}
		return extractRepoFromPath(parts[3])
	}

	// HTTPS format: https://github.com/owner/repo
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		parts := strings.SplitN(u, "/", 5)
		if len(parts) < 5 {
			return "", fmt.Errorf("invalid HTTPS URL: %s", rawURL)
		}
		return extractRepoFromPath(parts[3] + "/" + parts[4])
	}

	return "", fmt.Errorf("unrecognized remote URL format: %s", rawURL)
}

// extractRepoFromPath validates and returns the "owner/repo" string from a
// path segment. The path should be in the form "owner/repo" (possibly with
// additional trailing path segments which are ignored). Returns an error if
// the path does not contain at least two non-empty segments.
func extractRepoFromPath(path string) (string, error) {
	// Remove any trailing slashes and split on the first two segments.
	path = strings.TrimSuffix(path, "/")
	segments := strings.SplitN(path, "/", 3)
	if len(segments) < 2 || segments[0] == "" || segments[1] == "" {
		return "", fmt.Errorf("path %q does not contain owner/repo", path)
	}

	return segments[0] + "/" + segments[1], nil
}
