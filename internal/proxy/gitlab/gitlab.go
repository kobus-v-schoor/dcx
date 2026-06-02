// Package gitlab implements a transparent MITM proxy provider for the GitLab
// API. It registers itself as a proxy.Provider so that proxy.SetupAllProxies
// can discover and configure it automatically. The provider matches requests
// to GitLab domains and injects the host's GitLab token as the Authorization
// header.
//
// All generic proxy infrastructure (CA certificates, container injection,
// proxy lifecycle) is handled by the parent proxy package. This package only
// provides the GitLab-specific domain list and token injection logic.
package gitlab

import (
	"encoding/base64"
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
	proxy.RegisterProvider(&gitlabProvider{})
}

// gitlabProvider implements proxy.Provider for the GitLab API.
type gitlabProvider struct{}

// Name returns the provider name "gitlab".
func (g *gitlabProvider) Name() string { return "gitlab" }

// Enabled returns true if the GitLab proxy is enabled in the config.
func (g *gitlabProvider) Enabled(cfg *config.Config) bool {
	return cfg.Proxy.GitLab.Enabled
}

// Domains returns the list of GitLab domains to intercept. If the config
// specifies custom domains (for GitLab self-managed), those are used;
// otherwise a default set of public GitLab domains is returned.
func (g *gitlabProvider) Domains(cfg *config.Config) []string {
	if len(cfg.Proxy.GitLab.Domains) > 0 {
		return cfg.Proxy.GitLab.Domains
	}
	return []string{
		"gitlab.com",
		"registry.gitlab.com",
	}
}

// PrepareRequest injects the host's GitLab token into intercepted requests.
// glab API calls send the token via PRIVATE-TOKEN when using the env var,
// so the proxy replaces that header with the real token. For git-over-HTTPS
// and other requests without an existing auth header, the proxy injects
// Authorization: Basic with the token as the password so that both GitLab
// API and git endpoints accept it.
// Returns an error if no token is available on the host.
func (g *gitlabProvider) PrepareRequest(req *http.Request, cfg *config.Config) error {
	token, ok := DetectToken()
	if !ok {
		return fmt.Errorf("no GitLab token available on host")
	}
	// glab sends the token via PRIVATE-TOKEN when using the env var.
	if req.Header.Get("Private-Token") != "" || req.Header.Get("PRIVATE-TOKEN") != "" {
		req.Header.Set("PRIVATE-TOKEN", token)
	} else {
		// Use basic auth for both GitLab API and git-over-HTTPS
		// compatibility. GitLab accepts a PAT as the password with any
		// username; "oauth2" is the conventional choice.
		auth := base64.StdEncoding.EncodeToString([]byte("oauth2:" + token))
		req.Header.Set("Authorization", "Basic "+auth)
	}
	return nil
}

// EnvVars returns GITLAB_TOKEN=dummy and GLAB_TOKEN=dummy so that the glab CLI
// inside the container makes API requests. The proxy replaces the dummy token
// with the real host token at the network layer.
func (g *gitlabProvider) EnvVars(cfg *config.Config) []string {
	return []string{"GITLAB_TOKEN=dummy", "GLAB_TOKEN=dummy"}
}

// DetectToken reads the user's GitLab token from the host environment. It
// checks three sources in order: (1) the GITLAB_TOKEN environment variable,
// (2) the GLAB_TOKEN environment variable, and (3) the glab config get token
// CLI command. Returns the token string and true if found, or empty string and
// false if no token is available. The token is never logged per project
// security rules.
func DetectToken() (string, bool) {
	if token := os.Getenv("GITLAB_TOKEN"); token != "" {
		slog.Debug("using GITLAB_TOKEN for GitLab API proxy")
		return token, true
	}

	if token := os.Getenv("GLAB_TOKEN"); token != "" {
		slog.Debug("using GLAB_TOKEN for GitLab API proxy")
		return token, true
	}

	// Fall back to glab config get token, which reads the token from the
	// glab CLI's own credential store.
	path, err := exec.LookPath("glab")
	if err != nil {
		slog.Warn("glab CLI not found on PATH, cannot detect GitLab token")
		return "", false
	}

	// Try --host gitlab.com first since glab stores per-host tokens in the
	// OS keyring and --host reads the resolved value including keyring storage.
	// Fall back to plain "config get token" for self-managed instances or
	// older glab versions.
	out, err := exec.Command(path, "config", "get", "token", "--host", "gitlab.com").Output()
	if err != nil {
		// Try without --host for older glab versions or global tokens.
		out, err = exec.Command(path, "config", "get", "token").Output()
		if err != nil {
			slog.Warn("glab config get token failed, no GitLab token available", "error", err)
			return "", false
		}
	}

	token := strings.TrimSpace(string(out))
	if token == "" {
		slog.Warn("glab config get token returned empty string")
		return "", false
	}

	slog.Debug("using glab config get token for GitLab API proxy")
	return token, true
}
