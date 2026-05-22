// Package github implements a transparent MITM proxy provider for the GitHub
// API. It registers itself as a proxy.Provider so that proxy.SetupAllProxies
// can discover and configure it automatically. The provider matches requests
// to GitHub domains and injects the host's GitHub token as the Authorization
// header.
//
// All generic proxy infrastructure (CA certificates, container injection,
// proxy lifecycle) is handled by the parent proxy package. This package only
// provides the GitHub-specific domain list and token injection logic.
package github

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/kobus-v-schoor/dcx/internal/config"
	"github.com/kobus-v-schoor/dcx/internal/proxy"
)

func init() {
	proxy.RegisterProvider(&githubProvider{})
}

// githubProvider implements proxy.Provider for the GitHub API.
type githubProvider struct{}

// Name returns the provider name "github".
func (g *githubProvider) Name() string { return "github" }

// Enabled returns true if the GitHub proxy is enabled in the config.
func (g *githubProvider) Enabled(cfg *config.Config) bool {
	return cfg.Proxy.GitHub.Enabled
}

// Domains returns the list of GitHub domains to intercept. If the config
// specifies custom domains (for GitHub Enterprise), those are used;
// otherwise a default set of public GitHub domains is returned.
func (g *githubProvider) Domains(cfg *config.Config) []string {
	if len(cfg.Proxy.GitHub.Domains) > 0 {
		return cfg.Proxy.GitHub.Domains
	}
	return []string{
		"github.com",
		"api.github.com",
		"uploads.github.com",
		"raw.githubusercontent.com",
		"gist.github.com",
	}
}

// PrepareRequest injects the host's GitHub token as the Authorization header
// on intercepted requests. Any existing Authorization header is replaced.
// Returns an error if no token is available on the host.
func (g *githubProvider) PrepareRequest(req *http.Request, cfg *config.Config) error {
	token, ok := DetectToken()
	if !ok {
		return fmt.Errorf("no GitHub token available on host")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return nil
}

// FilterRequest inspects the intercepted GitHub API request and optionally
// returns a synthetic response to block it. When the GitHub proxy
// permissions list is non-empty, the request is matched to a semantic
// action name and repository, then blocked unless a matching permission
// entry grants access. Returns nil, nil when the request should proceed.
func (g *githubProvider) FilterRequest(req *http.Request, cfg *config.Config) (*http.Response, error) {
	return filterRequest(req, cfg)
}

// EnvVars returns GH_TOKEN=dummy so that the gh CLI inside the container
// makes API requests. The proxy replaces the dummy token with the real host
// token at the network layer.
func (g *githubProvider) EnvVars(cfg *config.Config) []string {
	return []string{"GH_TOKEN=dummy"}
}

// DetectToken reads the user's GitHub token from the host environment. It
// checks three sources in order: (1) the GH_TOKEN environment variable,
// (2) the GITHUB_TOKEN environment variable, and (3) the gh auth token CLI
// command. Returns the token string and true if found, or empty string and
// false if no token is available. The token is never logged per project
// security rules.
func DetectToken() (string, bool) {
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
