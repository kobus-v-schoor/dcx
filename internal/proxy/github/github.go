// Package github implements a reverse proxy for the GitHub API that injects
// the host's GitHub token into requests from the gh CLI inside the devcontainer.
// It builds on the generic proxy infrastructure from the parent proxy package,
// adding GitHub-specific request rewriting (director), token injection
// (transport), environment variable configuration, and token detection.
//
// The user's token is never exposed inside the container — it exists only in
// the host-side dcx process memory and is never written to disk or logged.
package github

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/kobus-v-schoor/dcx/internal/proxy"
)

// Options holds the configuration for creating and starting a GitHub API
// proxy. All fields have sensible defaults — zero values fall back to those
// defaults. Construct via DefaultOptions for a starting point with defaults
// pre-filled.
type Options struct {
	// Token is the user's GitHub token read from the host (via gh auth token
	// or the GITHUB_TOKEN/GH_TOKEN env var). It is never written to disk or
	// logged. The proxy injects it as the Authorization header on forwarded
	// requests, replacing whatever the container-side gh CLI sends.
	Token string

	// GatewayIP is the host's IP address on the Docker bridge network. The
	// container uses this IP to reach the proxy. It is included in the TLS
	// certificate's IP SANs so the gh CLI can verify the connection.
	GatewayIP string

	// BindAddr is the address the proxy listens on. Defaults to GatewayIP
	// (more secure — only reachable from the container's network) if empty.
	// Set to "0.0.0.0" to listen on all interfaces (needed in some Docker
	// network setups).
	BindAddr string

	// APIURL is the GitHub API URL to forward requests to. Defaults to
	// "https://api.github.com" if empty. Override for GitHub Enterprise
	// Server (e.g. "https://github.example.com/api/v3").
	APIURL string

	// CACertPath is the container path where the CA certificate is copied.
	// Defaults to "/opt/dcx/gh-proxy/ca.crt" if empty. The CA cert is
	// referenced by both NODE_EXTRA_CA_CERTS (for Node.js-based tools) and
	// the combined CA bundle referenced by SSL_CERT_FILE (for Go-based
	// programs).
	CACertPath string

	// CertExpiry is the duration for which the generated TLS certificates
	// (both CA and server) are valid. Defaults to 24 hours if zero. The
	// certificates are ephemeral — they only need to last for the duration
	// of a dcx exec session.
	CertExpiry time.Duration
}

// Proxy wraps a proxy.Service with GitHub-specific configuration and request
// handling. It provides the GitHub API director, token-injecting transport,
// and environment variable construction for the gh CLI and git inside the
// devcontainer.
type Proxy struct {
	// svc is the underlying generic proxy service that handles TLS, lifecycle,
	// and request forwarding.
	svc *proxy.Service

	// opts holds the GitHub-specific proxy options including token and
	// network settings.
	opts Options
}

// New creates a new GitHub API Proxy with the given options. The token from
// opts is used to set the Authorization header on forwarded requests, replacing
// whatever token the container-side gh CLI provides. Call Start to begin
// serving requests, and Shutdown to stop the proxy.
func New(opts Options) *Proxy {
	// Build the generic proxy options from the GitHub-specific options.
	proxyOpts := proxy.Options{
		GatewayIP:  opts.GatewayIP,
		BindAddr:   opts.BindAddr,
		APIURL:     opts.apiURL(),
		CACertPath: opts.caCertPath(),
		CertExpiry: opts.certExpiry(),
	}

	return &Proxy{
		svc:  proxy.NewService("github", proxyOpts),
		opts: opts,
	}
}

// Start creates the reverse proxy handler with GitHub-specific director and
// transport, then starts the underlying proxy service. It returns the port
// number the proxy is listening on. The caller should call Shutdown to stop
// the proxy when the devcontainer session ends. The CA certificate PEM bytes
// are available via CACertPEM after Start returns.
func (p *Proxy) Start() (int, error) {
	// Build the reverse proxy that forwards requests to the configured GitHub
	// API URL. The Director rewrites the request so it targets the real GitHub
	// API, and the transport injects the host token.
	target, err := url.Parse(p.opts.apiURL())
	if err != nil {
		return 0, fmt.Errorf("parsing GitHub API URL %q: %w", p.opts.apiURL(), err)
	}

	director := p.director(target)
	transport := &tokenTransport{token: p.opts.Token}
	handler := proxy.NewReverseProxy(target, director, transport, "github")

	return p.svc.Start(handler)
}

// Shutdown gracefully stops the proxy server. It waits for in-flight requests
// to complete and then closes the listener. Call this when the devcontainer
// session ends to clean up the proxy.
func (p *Proxy) Shutdown() {
	p.svc.Shutdown()
}

// CACertPEM returns the PEM-encoded CA certificate. This is copied into
// the container at the configured CA cert path so the gh CLI trusts the
// proxy's self-signed TLS certificate via SSL_CERT_FILE and
// NODE_EXTRA_CA_CERTS.
func (p *Proxy) CACertPEM() []byte {
	return p.svc.CACertPEM()
}

// Opts returns a copy of the proxy's options. Used by callers that need
// access to the resolved configuration (e.g. CA cert paths for building
// remote env vars).
func (p *Proxy) Opts() Options {
	return p.opts
}

// ServiceOpts returns the underlying proxy service options. Used by callers
// that need access to the generic proxy configuration (e.g. for Docker copy
// operations or CA bundle creation).
func (p *Proxy) ServiceOpts() proxy.Options {
	return p.svc.Opts()
}

// BuildRemoteEnv constructs the environment variable flags that configure the
// gh CLI and git inside the container to route all GitHub API requests through
// the proxy. These are passed as --remote-env flags to devcontainer exec.
// Returns a slice of "--remote-env=NAME=VALUE" strings.
//
// The proxyPort is the port the proxy is listening on (returned by Start).
// The gateway IP is taken from the proxy's options.
//
// SSL_CERT_FILE points to a combined CA bundle (system certs + proxy CA) so
// that Go-based programs trust both the system CAs and the proxy's
// self-signed CA. This is necessary because Go's SSL_CERT_FILE replaces the
// system CA pool entirely rather than appending to it. NODE_EXTRA_CA_CERTS
// points to the proxy CA cert alone, since Node.js appends to the system
// trust store.
func (p *Proxy) BuildRemoteEnv(proxyPort int) []string {
	var envVars []string
	opts := p.opts
	svcOpts := p.svc.Opts()

	// GH_HOST tells the gh CLI which GitHub host to target. The gh CLI
	// constructs the API URL as https://GH_HOST/api/v3/... so including
	// the port directs it to the proxy. Using the gateway IP (not
	// host.docker.internal) ensures connectivity in all environments.
	ghHost := fmt.Sprintf("%s:%d", opts.GatewayIP, proxyPort)
	envVars = append(envVars, fmt.Sprintf("--remote-env=GH_HOST=%s", ghHost))

	// SSL_CERT_FILE tells Go-based programs to trust the combined CA bundle
	// (system certs + proxy CA). Go's SSL_CERT_FILE replaces the system CA
	// pool entirely, so we must include the system CAs in the bundle to avoid
	// breaking HTTPS connectivity for other Go programs in the container.
	// NODE_EXTRA_CA_CERTS is also set for Node.js-based tools; unlike Go,
	// Node.js appends this cert to the system trust store.
	envVars = append(envVars, fmt.Sprintf("--remote-env=SSL_CERT_FILE=%s", svcOpts.CABundlePathResolved()))
	envVars = append(envVars, fmt.Sprintf("--remote-env=NODE_EXTRA_CA_CERTS=%s", svcOpts.CACertPathResolved()))

	// GIT_CONFIG_COUNT, GIT_CONFIG_KEY_0, and GIT_CONFIG_VALUE_0 configure
	// git to rewrite GitHub URLs so that git operations (clone, push, pull)
	// also route through the proxy. The insteadOf directive maps
	// https://GH_HOST/ URLs to https://github.com/ URLs so git can reach
	// the proxy when users clone/push to GitHub remotes.
	envVars = append(envVars, "--remote-env=GIT_CONFIG_COUNT=1")
	envVars = append(envVars, fmt.Sprintf("--remote-env=GIT_CONFIG_KEY_0=url.https://%s/.insteadOf", ghHost))
	envVars = append(envVars, "--remote-env=GIT_CONFIG_VALUE_0=https://github.com/")

	return envVars
}

// director returns a function that rewrites the incoming request so it targets
// the configured GitHub API URL instead of the proxy host. It replaces the
// request scheme, host, and URL path, and clears the Authorization header so
// the scoping transport can inject the host token. When the gh CLI connects to
// a custom GH_HOST (not github.com), it prefixes all API paths with
// "/api/v3/" (GitHub Enterprise convention). Since we forward to the
// configured API URL which does not use this prefix, we strip it here.
// This is used as the reverse proxy's Director function.
//
// The implementation was inspired by the gh-aw-mcpg proxy mode:
// https://github.com/github/gh-aw-mcpg/blob/main/docs/PROXY_MODE.md
func (p *Proxy) director(target *url.URL) func(*http.Request) {
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

// DetectToken reads the user's GitHub token from the host environment. It
// checks three sources in order: (1) the GH_TOKEN environment variable,
// (2) the GITHUB_TOKEN environment variable, and (3) the gh auth token CLI
// command. Returns the token string and true if found, or empty string and
// false if no token is available. The token is never logged per project
// security rules. Called by dcx exec when the github proxy is enabled.
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

// WriteCACertToFile writes the PEM-encoded CA certificate to a temporary file
// on the host. Delegates to the generic proxy package's WriteCACertToFile.
// The caller should clean up the file when the proxy is shut down.
func WriteCACertToFile(caCertPEM []byte) (string, error) {
	return proxy.WriteCACertToFile(caCertPEM)
}

// caCertPath returns the CA cert container path. Defaults to
// "/opt/dcx/gh-proxy/ca.crt" if empty.
func (o Options) caCertPath() string {
	if o.CACertPath == "" {
		return "/opt/dcx/gh-proxy/ca.crt"
	}
	return o.CACertPath
}

// apiURL returns the GitHub API URL to forward requests to. Defaults to
// "https://api.github.com" if empty.
func (o Options) apiURL() string {
	if o.APIURL == "" {
		return "https://api.github.com"
	}
	return o.APIURL
}

// certExpiry returns the certificate expiry duration. Defaults to 24 hours
// if zero.
func (o Options) certExpiry() time.Duration {
	if o.CertExpiry == 0 {
		return 24 * time.Hour
	}
	return o.CertExpiry
}
