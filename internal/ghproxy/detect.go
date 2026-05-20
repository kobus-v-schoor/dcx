// Package ghproxy implements a reverse proxy for the GitHub API that injects
// the host's GitHub token into requests from the gh CLI inside the devcontainer.
// The proxy runs inside the dcx process, listens on HTTPS with a self-signed
// certificate, and forwards all requests to api.github.com after rewriting
// the Host header and injecting the host's GitHub token. The user's token
// is never exposed inside the container — it exists only in the host-side
// dcx process memory and is never written to disk or logged.
package ghproxy

import (
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
