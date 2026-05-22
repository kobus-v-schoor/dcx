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

// detectRemoteURLFunc is the function used to detect the current repository's
// origin remote URL. It is a variable so tests can override it.
var detectRemoteURLFunc = git.DetectRemoteURL

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
	apiTarget, err := url.Parse(opts.APIURLResolved())
	if err != nil {
		return nil, fmt.Errorf("parsing GitHub API URL %q: %w", opts.APIURLResolved(), err)
	}

	// Git HTTPS operations target the main GitHub host, not the API endpoint.
	gitTarget, err := url.Parse(resolvedGitURL(opts.APIURLResolved()))
	if err != nil {
		return nil, fmt.Errorf("parsing GitHub git URL: %w", err)
	}

	director := newDirector(apiTarget, gitTarget)
	transport := &tokenTransport{token: token}
	return proxy.NewReverseProxy(apiTarget, director, transport, "github"), nil
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

	// Configure git to route HTTPS operations through the proxy.
	// git's url.<base>.insteadOf directive rewrites URLs starting with
	// the origin host to point at the proxy instead. http.sslCAInfo tells
	// git to trust the proxy's self-signed TLS certificate.
	if remoteURL, ok := detectRemoteURLFunc(); ok {
		u, err := url.Parse(remoteURL)
		if err == nil && u.Scheme == "https" {
			proxyURL := fmt.Sprintf("https://%s/", ghHost)
			originPrefix := fmt.Sprintf("https://%s/", u.Host)

			envVars = append(envVars, "--remote-env=GIT_CONFIG_COUNT=2")
			envVars = append(envVars, fmt.Sprintf("--remote-env=GIT_CONFIG_KEY_0=url.%s.insteadOf", proxyURL))
			envVars = append(envVars, fmt.Sprintf("--remote-env=GIT_CONFIG_VALUE_0=%s", originPrefix))
			envVars = append(envVars, "--remote-env=GIT_CONFIG_KEY_1=http.sslCAInfo")
			envVars = append(envVars, fmt.Sprintf("--remote-env=GIT_CONFIG_VALUE_1=%s", opts.CACertPathResolved()))
		} else {
			slog.Warn("github proxy: git remote is not HTTPS, git operations will not be proxied",
				"remote", remoteURL)
		}
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
// targets either the GitHub API or the GitHub git host, depending on whether
// the request is a git HTTPS operation. For API requests it replaces the
// scheme, host, and URL path (stripping /api/v3 and /api prefixes that the gh
// CLI adds for custom GH_HOST). For git requests it forwards to the main
// GitHub host without path rewriting. In both cases it clears the
// Authorization and Proxy-Connection headers so the transport can inject the
// correct credentials.
func newDirector(apiTarget, gitTarget *url.URL) func(*http.Request) {
	return func(req *http.Request) {
		var target *url.URL
		if isGitRequest(req) {
			target = gitTarget
		} else {
			target = apiTarget
		}

		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host

		if target == apiTarget {
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
		}

		// Clear the incoming Authorization header so the scoping transport
		// can inject the host token. The container-side client may send its
		// own (invalid) token — we always replace it with the real one.
		req.Header.Del("Authorization")

		// Remove the proxy-specific headers that the gh CLI may add when
		// connecting through a custom host. These are not needed by the
		// real GitHub API and could cause issues if forwarded.
		req.Header.Del("Proxy-Connection")
	}
}

// isGitRequest reports whether the request uses git's smart HTTP protocol.
// Git HTTPS operations are identified by URL paths ending in
// /git-upload-pack, /git-receive-pack, or /info/refs with a service=git-
// query parameter. These requests are forwarded to the main GitHub host
// rather than the API endpoint, and use basic auth instead of bearer tokens.
func isGitRequest(req *http.Request) bool {
	path := req.URL.Path
	if strings.HasSuffix(path, "/git-upload-pack") || strings.HasSuffix(path, "/git-receive-pack") {
		return true
	}
	if strings.HasSuffix(path, "/info/refs") && strings.Contains(req.URL.RawQuery, "service=git-") {
		return true
	}
	return false
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

// RoundTrip implements http.RoundTripper. It injects the host's GitHub
// token into the request and forwards it to the upstream GitHub host via the
// default transport. For git HTTPS requests it uses basic auth with the token
// as the password (GitHub accepts oauth2/<token>). For API requests it uses a
// bearer token. The director already cleared any incoming Authorization header,
// so this is the only auth the forwarded request will carry.
func (t *tokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if isGitRequest(req) {
		// Git over HTTPS uses basic auth. GitHub accepts the token as the
		// password with any username; using "oauth2" matches convention.
		req.SetBasicAuth("oauth2", t.token)
	} else {
		// API requests use a bearer token.
		req.Header.Set("Authorization", "Bearer "+t.token)
	}

	// Use the default transport to forward the request to the upstream.
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

// resolvedGitURL returns the GitHub host URL for git HTTPS operations.
// When the API URL is the public GitHub API, git operations target
// github.com directly. For GitHub Enterprise or other custom API URLs,
// the git host is derived from the API URL's scheme and host.
func resolvedGitURL(apiURL string) string {
	if apiURL == "" || apiURL == "https://api.github.com" {
		return "https://github.com"
	}
	u, err := url.Parse(apiURL)
	if err != nil {
		return "https://github.com"
	}
	return fmt.Sprintf("%s://%s", u.Scheme, u.Host)
}

// resolvedCertExpiry returns the certificate expiry duration. Defaults to
// 24 hours if the configured value is zero.
func resolvedCertExpiry(certExpiry time.Duration) time.Duration {
	if certExpiry == 0 {
		return 24 * time.Hour
	}
	return certExpiry
}
