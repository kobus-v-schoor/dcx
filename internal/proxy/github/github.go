// Package github implements a reverse proxy for the GitHub API that injects
// the host's GitHub token into requests from the gh CLI inside the devcontainer.
// It provides the bare minimum that is GitHub-specific: token detection, request
// rewriting (director), token injection (transport), and GitHub-specific
// environment variable configuration.
//
// All generic proxy infrastructure (TLS, CA certificates, container injection,
// CA bundle creation, service lifecycle) is handled by the parent proxy package.
// This package registers itself as a Provider via an init() function so that
// proxy.SetupAllProxies() can discover and set it up without the caller needing
// to import this package directly.
//
// The request rewriting logic (director) was inspired by the gh-aw-mcpg proxy
// mode: https://github.com/github/gh-aw-mcpg/blob/main/docs/PROXY_MODE.md
package github

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/kobus-v-schoor/dcx/internal/config"
	"github.com/kobus-v-schoor/dcx/internal/git"
	"github.com/kobus-v-schoor/dcx/internal/proxy"
)

// detectRepoFunc is the function used to detect the current repository.
// It is a variable so tests can override it.
var detectRepoFunc = git.DetectRepo

func init() {
	proxy.RegisterProvider(&githubProvider{})
}

// githubProvider implements proxy.Provider for the GitHub API reverse proxy.
// It is registered via init() so that proxy.SetupAllProxies() can discover
// and set it up without the caller importing this package directly.
type githubProvider struct{}

// Name returns the provider name "github", used in log messages.
func (g *githubProvider) Name() string { return "github" }

// Enabled returns true if the GitHub proxy is enabled in the config.
func (g *githubProvider) Enabled(cfg *config.Config) bool {
	return cfg.Proxy.GitHub.Enabled
}

// ServiceOptions returns the proxy.Options for the GitHub service based on the
// config and gateway IP. The GitHub proxy always uses TLS since the github cli
// enforces connections over TLS (won't connect to an HTTP proxy).
func (g *githubProvider) ServiceOptions(cfg *config.Config, gatewayIP string) proxy.Options {
	return proxy.Options{
		TLSEnabled: true,
		GatewayIP:  gatewayIP,
		BindAddr:   cfg.Proxy.GitHub.BindAddr,
		APIURL:     resolvedAPIURL(cfg.Proxy.GitHub.APIURL),
		CACertPath: resolvedCACertPath(cfg.Proxy.GitHub.CACertPath),
		CertExpiry: resolvedCertExpiry(cfg.Proxy.GitHub.CertExpiry),
	}
}

// CreateHandler creates the HTTP handler for the GitHub API reverse proxy.
// The handler rewrites incoming requests to target the GitHub API and injects
// the host's GitHub token as the Authorization header. Returns an error if no
// token is available on the host.
func (g *githubProvider) CreateHandler(opts proxy.Options, cfg *config.Config) (http.Handler, error) {
	// Detect the host's GitHub token. If no token is available, the proxy
	// cannot function — it needs the token to inject into forwarded requests.
	token, ok := DetectToken()
	if !ok {
		return nil, fmt.Errorf("no GitHub token available on host")
	}

	// Build the reverse proxy that forwards requests to the GitHub API.
	target, err := url.Parse(opts.APIURLResolved())
	if err != nil {
		return nil, fmt.Errorf("parsing GitHub API URL %q: %w", opts.APIURLResolved(), err)
	}

	director := newDirector(target)
	transport := &tokenTransport{token: token}
	return proxy.NewReverseProxy(target, director, transport, "github"), nil
}

// RemoteEnvVars returns the GitHub-specific remote environment variables for
// the container. These configure the gh CLI to route GitHub API requests
// through the proxy:
//   - GH_HOST: tells the gh CLI which host to target
//   - GH_REPO: tells the gh CLI which repository to operate on, bypassing
//     remote URL inference that fails when GH_HOST does not match the
//     origin remote host
//
// On non-Linux hosts GH_HOST uses host.docker.internal (the standard way for
// containers to reach the host on Docker Desktop / Colima). On Linux it falls
// back to the gateway IP since host.docker.internal is not always available.
//
// Generic TLS env vars (SSL_CERT_FILE, NODE_EXTRA_CA_CERTS) are added by
// the proxy infrastructure and should not be included here.
func (g *githubProvider) RemoteEnvVars(port int, opts proxy.Options, cfg *config.Config) []string {
	var envVars []string

	// GH_HOST tells the gh CLI which GitHub host to target. The gh CLI
	// constructs the API URL as https://GH_HOST/api/v3/... so including
	// the port directs it to the proxy.
	var ghHost string
	if runtime.GOOS == "linux" {
		ghHost = fmt.Sprintf("%s:%d", opts.GatewayIP, port)
	} else {
		ghHost = fmt.Sprintf("%s:%d", proxy.ProxyHost, port)
	}
	envVars = append(envVars, fmt.Sprintf("--remote-env=GH_HOST=%s", ghHost))

	// GH_REPO bypasses the gh CLI's remote URL inference. When GH_HOST is
	// set to a custom host (our proxy), the gh CLI compares the inferred
	// repo host with GH_HOST and refuses operations if they differ. Setting
	// GH_REPO explicitly avoids this mismatch.
	if repo, ok := detectRepoFunc(); ok {
		envVars = append(envVars, fmt.Sprintf("--remote-env=GH_REPO=%s", repo))
	}

	return envVars
}

// DetectToken reads the user's GitHub token from the host environment. It
// checks three sources in order: (1) the GH_TOKEN environment variable,
// (2) the GITHUB_TOKEN environment variable, and (3) the gh auth token CLI
// command. Returns the token string and true if found, or empty string and
// false if no token is available. The token is never logged per project
// security rules. Called by the github provider when creating the handler.
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

// newDirector returns a function that rewrites the incoming request so it
// targets the configured GitHub API URL instead of the proxy host. It replaces
// the request scheme, host, and URL path, and clears the Authorization header
// so the scoping transport can inject the host token. When the gh CLI connects
// to a custom GH_HOST (not github.com), it prefixes all API paths with
// "/api/v3/" (GitHub Enterprise convention). Since we forward to the
// configured API URL which does not use this prefix, we strip it here.
func newDirector(target *url.URL) func(*http.Request) {
	return func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host

		// Strip the "/api/v3/" prefix that the gh CLI adds when GH_HOST
		// is a custom host (GHE convention). The api.github.com endpoint
		// does not use this prefix — API paths are at the root (e.g.
		// /repos/owner/repo, not /api/v3/repos/owner/repo).
		req.URL.Path = strings.TrimPrefix(req.URL.Path, "/api/v3")

		// Also strip the "/api" prefix used by the gh CLI for GraphQL
		// requests. When GH_HOST is a custom host, the gh CLI sends
		// GraphQL requests to https://GH_HOST/api/graphql, but
		// api.github.com serves GraphQL at /graphql (without /api prefix).
		// This must come after the /api/v3 strip so that REST paths are
		// handled first.
		if strings.HasPrefix(req.URL.Path, "/api/") {
			req.URL.Path = strings.TrimPrefix(req.URL.Path, "/api")
		}

		if req.URL.Path == "" {
			req.URL.Path = "/"
		}

		// Clear the incoming Authorization header so the scoping transport
		// can inject the host token. The container-side gh CLI may send its
		// own (invalid) token — we always replace it with the real one.
		req.Header.Del("Authorization")

		// Remove the proxy-specific headers that the gh CLI may add when
		// connecting through a custom host. These are not needed by the
		// real GitHub API and could cause issues if forwarded.
		req.Header.Del("Proxy-Connection")
	}
}

// tokenTransport is an http.RoundTripper that injects the host's GitHub
// token as the Authorization header on every forwarded request. The director
// already cleared any incoming Authorization header, so this is the only
// token the forwarded request will carry. All requests are forwarded without
// scoping — the proxy's purpose is to keep the token on the host side and
// inject it at the network layer, not to restrict access.
type tokenTransport struct {
	// token is the host's GitHub token, injected as the Authorization header
	// on forwarded requests.
	token string
}

// RoundTrip implements http.RoundTripper. It injects the host's GitHub token
// as the Authorization header and forwards the request to the GitHub API via
// the default transport.
func (t *tokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Inject the host's GitHub token as the Authorization header. The director
	// already cleared any incoming Authorization header, so this is the only
	// token the forwarded request will carry.
	req.Header.Set("Authorization", "Bearer "+t.token)

	// Use the default transport to forward the request to the GitHub API.
	return http.DefaultTransport.RoundTrip(req)
}

// resolvedAPIURL returns the GitHub API URL to forward requests to. Defaults
// to "https://api.github.com" if the configured value is empty.
func resolvedAPIURL(apiURL string) string {
	if apiURL == "" {
		return "https://api.github.com"
	}
	return apiURL
}

// resolvedCACertPath returns the CA cert container path. Defaults to
// "/opt/dcx/gh-proxy/ca.crt" if the configured value is empty.
func resolvedCACertPath(caCertPath string) string {
	if caCertPath == "" {
		return "/opt/dcx/gh-proxy/ca.crt"
	}
	return caCertPath
}

// resolvedCertExpiry returns the certificate expiry duration. Defaults to
// 24 hours if the configured value is zero.
func resolvedCertExpiry(certExpiry time.Duration) time.Duration {
	if certExpiry == 0 {
		return 24 * time.Hour
	}
	return certExpiry
}
