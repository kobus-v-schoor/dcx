// Package ghproxy implements a reverse proxy for the GitHub API that enforces
// repository-level scoping on the user's GitHub token. The proxy runs inside
// the dcx process, listens on HTTPS with a self-signed certificate, and
// forwards allowed requests to api.github.com after rewriting the Host header
// and injecting the host's GitHub token. Requests targeting repositories other
// than the configured one are rejected with 403 Forbidden.
package ghproxy

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ProxyHost is the hostname used in the TLS certificate's Subject Alternative
// Name. The proxy generates a self-signed certificate with this hostname so
// the gh CLI can verify the TLS connection. In practice, GH_HOST is set to
// the Docker gateway IP (detected at runtime) plus the proxy port, so the
// certificate also includes an IP SAN for the gateway. The hostname SAN
// provides a fallback for environments where host.docker.internal is
// routable.
const ProxyHost = "host.docker.internal"

// CACertMountPath is the container path where the CA certificate is copied
// so that the gh CLI trusts the proxy's self-signed TLS certificate. The
// certificate is referenced by both SSL_CERT_FILE (for Go-based programs
// like the gh CLI binary) and NODE_EXTRA_CA_CERTS (for Node.js-based tools).
const CACertMountPath = "/opt/dcx/gh-proxy/ca.crt"

// Proxy handles HTTP/HTTPS requests from the gh CLI inside the devcontainer,
// enforces repository-level scoping, and forwards allowed requests to
// api.github.com. It runs as an HTTPS server with a self-signed certificate
// generated on startup.
type Proxy struct {
	// listener is the TCP listener the proxy server uses. Bound to a random
	// available port on 0.0.0.0 so the proxy is reachable from the Docker
	// container via the gateway IP.
	listener net.Listener

	// server is the HTTPS server that serves the proxy. It uses a self-signed
	// TLS certificate generated in-memory on startup.
	server *http.Server

	// caCertPEM is the PEM-encoded CA certificate, made available inside the
	// container so the gh CLI trusts the proxy's TLS certificate.
	caCertPEM []byte

	// token is the user's GitHub token read from the host (via gh auth token
	// or the GITHUB_TOKEN/GH_TOKEN env var). It is never written to disk or
	// logged. The proxy injects it as the Authorization header on forwarded
	// requests, replacing whatever the container-side gh CLI sends.
	token string

	// repository is the allowed repository in "owner/repo" format. Requests
	// targeting a different repository are rejected with 403 Forbidden. If
	// empty, all repository-scoped requests are allowed (no scoping enforced).
	repository string

	// gatewayIP is the host's IP address on the Docker bridge network. The
	// container uses this IP to reach the proxy. It is included in the TLS
	// certificate's IP SANs so the gh CLI can verify the connection.
	gatewayIP string

	// mu protects the done channel so Start and Shutdown can be called safely
	// from different goroutines.
	mu sync.Mutex

	// done is closed when the proxy server has fully shut down, allowing
	// callers to wait for clean termination.
	done chan struct{}
}

// New creates a new Proxy that enforces scoping to the given repository and
// forwards requests to api.github.com using the provided token. The token is
// used to set the Authorization header on forwarded requests, replacing
// whatever token the container-side gh CLI provides. The repository should be
// in "owner/repo" format; if empty, no repository scoping is enforced.
// The gatewayIP is the host's IP on the Docker bridge network, used in the
// TLS certificate's IP SANs so the container can verify the connection.
// Call Start to begin serving requests, and Shutdown to stop the proxy.
func New(token, repository, gatewayIP string) *Proxy {
	return &Proxy{
		token:      token,
		repository: repository,
		gatewayIP:  gatewayIP,
		done:       make(chan struct{}),
	}
}

// Start generates a self-signed TLS certificate, binds to a random available
// port on localhost, and starts the HTTPS proxy server. It returns the port
// number the proxy is listening on. The caller should call Shutdown to stop
// the proxy when the devcontainer session ends. The CA certificate PEM bytes
// are available via CACertPEM after Start returns.
func (p *Proxy) Start() (int, error) {
	// Generate a self-signed CA certificate and server certificate in-memory.
	// The CA cert is made available inside the container so the gh CLI trusts
	// the proxy's TLS certificate via NODE_EXTRA_CA_CERTS.
	caCert, caKey, err := generateCA()
	if err != nil {
		return 0, fmt.Errorf("generating CA certificate: %w", err)
	}

	serverCert, serverKey, err := generateServerCert(caCert, caKey, ProxyHost, p.gatewayIP)
	if err != nil {
		return 0, fmt.Errorf("generating server certificate: %w", err)
	}

	// Store the CA certificate PEM for mounting into the container.
	p.caCertPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caCert.Raw,
	})

	tlsCert := tls.Certificate{
		Certificate: [][]byte{serverCert.Raw},
		PrivateKey:  serverKey,
		Leaf:        serverCert,
	}

	// Bind to a random available port on all interfaces so the proxy is
	// reachable from the Docker container via host.docker.internal.
	p.listener, err = net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return 0, fmt.Errorf("binding proxy listener: %w", err)
	}

	port := p.listener.Addr().(*net.TCPAddr).Port

	// Build the reverse proxy that forwards requests to api.github.com.
	// The Director rewrites the request so it targets the real GitHub API,
	// and the transport enforces repository-level scoping.
	target, _ := url.Parse("https://api.github.com")
	reverseProxy := httputil.NewSingleHostReverseProxy(target)

	// Override the transport so we can intercept and check requests before
	// they are forwarded. The default transport would send requests directly
	// to api.github.com; we need to enforce repository scoping and inject
	// the host token.
	reverseProxy.Transport = &scopingTransport{
		repository: p.repository,
		token:      p.token,
	}

	reverseProxy.Director = p.director(target)
	reverseProxy.ErrorHandler = p.errorHandler

	p.server = &http.Server{
		Handler: reverseProxy,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{tlsCert},
			MinVersion:   tls.VersionTLS12,
		},
	}

	// Start serving in a goroutine so Start can return immediately.
	go func() {
		defer close(p.done)
		// ServeTLS is used instead of Serve because the proxy must speak
		// HTTPS — the gh CLI requires HTTPS when connecting to a custom
		// GH_HOST. The TLS certificate and key are already loaded into
		// the server's TLSConfig, so the certFile and keyFile parameters
		// are empty strings.
		if err := p.server.ServeTLS(p.listener, "", ""); err != nil {
			if err != http.ErrServerClosed {
				slog.Error("proxy server error", "error", err)
			}
		}
	}()

	slog.Info("GitHub API proxy started", "port", port, "repository", p.repository)

	return port, nil
}

// Shutdown gracefully stops the proxy server. It waits for in-flight requests
// to complete (with a 5-second timeout) and then closes the listener. Call
// this when the devcontainer session ends to clean up the proxy.
func (p *Proxy) Shutdown() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.server == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.server.Shutdown(ctx); err != nil {
		slog.Error("proxy shutdown error", "error", err)
	}

	// Wait for the serving goroutine to finish.
	<-p.done

	slog.Info("GitHub API proxy stopped")
}

// CACertPEM returns the PEM-encoded CA certificate. This is copied into
// the container at CACertMountPath so the gh CLI trusts the proxy's
// self-signed TLS certificate via SSL_CERT_FILE and NODE_EXTRA_CA_CERTS.
func (p *Proxy) CACertPEM() []byte {
	return p.caCertPEM
}

// director returns a function that rewrites the incoming request so it targets
// api.github.com instead of the proxy host. It replaces the request scheme,
// host, and URL path, and clears the Authorization header so the scoping
// transport can inject the host token. When the gh CLI connects to a
// custom GH_HOST (not github.com), it prefixes all API paths with
// "/api/v3/" (GitHub Enterprise convention). Since we forward to
// api.github.com which does not use this prefix, we strip it here.
// This is used as the reverse proxy's Director function.
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

// errorHandler logs proxy errors and returns a 502 Bad Gateway response to
// the client. This provides clearer error messages than the default reverse
// proxy error handler, which returns a generic "internal server error".
func (p *Proxy) errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("proxy error", "path", r.URL.Path, "error", err)
	http.Error(w, fmt.Sprintf("proxy error: %v", err), http.StatusBadGateway)
}

// scopingTransport is an http.RoundTripper that enforces repository-level
// scoping on GitHub API requests. Before forwarding each request, it checks
// whether the request targets the allowed repository. Requests targeting a
// different repository are rejected with 403 Forbidden. Allowed requests
// have the host's GitHub token injected as the Authorization header.
type scopingTransport struct {
	// repository is the allowed repository in "owner/repo" format. If empty,
	// no repository scoping is enforced and all requests are forwarded.
	repository string

	// token is the host's GitHub token, injected as the Authorization header
	// on forwarded requests. It replaces whatever the container-side gh CLI
	// sends (the director already cleared the incoming Authorization header).
	token string
}

// RoundTrip implements http.RoundTripper. It checks the request path for
// repository scoping and either forwards the request to api.github.com or
// rejects it with 403 Forbidden. The host token is always injected as the
// Authorization header on forwarded requests.
func (t *scopingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Enforce repository scoping if a repository is configured.
	if t.repository != "" {
		if err := t.checkRepoScope(req.URL.Path); err != nil {
			return &http.Response{
				StatusCode: http.StatusForbidden,
				Header:     make(http.Header),
				Body:       ioBody(fmt.Sprintf("dcx-proxy: %s\n", err.Error())),
			}, nil
		}
	}

	// Inject the host's GitHub token as the Authorization header. The director
	// already cleared any incoming Authorization header, so this is the only
	// token the forwarded request will carry.
	req.Header.Set("Authorization", "Bearer "+t.token)

	// Use the default transport to forward the request to api.github.com.
	return http.DefaultTransport.RoundTrip(req)
}

// checkRepoScope validates that the request path targets the allowed
// repository. GitHub API v3 paths for repository operations follow the pattern
// /repos/{owner}/{repo}/... . The function extracts the owner/repo from the
// path and compares it against the allowed repository. Requests that don't
// target a specific repository (e.g. /user, /app, /graphql) are allowed —
// these are needed for gh auth status and similar commands.
func (t *scopingTransport) checkRepoScope(path string) error {
	repo, ok := extractRepo(path)
	if !ok {
		// Request does not target a specific repository (e.g. /user, /app,
		// /graphql) — allow it through. These are needed for gh auth status
		// and similar commands.
		return nil
	}

	if repo != t.repository {
		return fmt.Errorf("repository %q is not in the allowed scope (%q)", repo, t.repository)
	}

	return nil
}

// extractRepo parses a GitHub API v3 request path to extract the owner/repo
// segment. GitHub API paths for repository operations follow the pattern
// /repos/{owner}/{repo}/... . Returns the "owner/repo" string and true if
// found, or empty string and false if the path does not target a specific
// repository (e.g. /user, /app, /graphql).
func extractRepo(path string) (string, bool) {
	// Normalize: remove leading slash and split on slashes.
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")

	// GitHub API v3 repository paths start with "repos/{owner}/{repo}".
	if len(segments) < 3 || segments[0] != "repos" {
		return "", false
	}

	owner := segments[1]
	repo := segments[2]

	// The repo name may have additional segments after it (e.g.
	// /repos/owner/repo/issues), but we only need owner/repo.
	if owner == "" || repo == "" {
		return "", false
	}

	return owner + "/" + repo, true
}

// ioBody returns an io.ReadCloser that yields the given string. Used to
// construct response bodies for rejected requests without allocating a
// bytes.Buffer or pipe.
func ioBody(s string) io.ReadCloser {
	return io.NopCloser(strings.NewReader(s))
}

// generateCA creates a self-signed CA certificate and ECDSA private key. The
// certificate is valid for 24 hours (ephemeral — only for the duration of the
// proxy session). The key uses P-256 curve which is widely supported and
// provides adequate security for short-lived certificates. Returns the parsed
// certificate and private key. Called once on proxy startup.
func generateCA() (*x509.Certificate, *ecdsa.PrivateKey, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generating CA key: %w", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("generating CA serial number: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"dcx proxy CA"},
			CommonName:   "dcx proxy CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("creating CA certificate: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing CA certificate: %w", err)
	}

	return cert, key, nil
}

// generateServerCert creates a TLS server certificate signed by the given CA
// certificate and key. The certificate is valid for 24 hours and includes the
// given host as a DNS SAN (Subject Alternative Name) and the gatewayIP as an
// IP SAN. The IP SAN is needed because the gh CLI connects to the proxy via
// the Docker bridge gateway IP, and TLS verification checks the SAN against
// the connection target. The key uses P-256 curve.
// Returns the parsed certificate and private key. Called once on proxy startup
// after generateCA.
func generateServerCert(caCert *x509.Certificate, caKey *ecdsa.PrivateKey, host, gatewayIP string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generating server key: %w", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("generating server serial number: %w", err)
	}

	// Build the SAN lists. The DNS name is the hostname (host.docker.internal)
	// and the IP address is the Docker bridge gateway. Both are needed so the
	// gh CLI can verify the TLS connection regardless of which address it
	// uses to reach the proxy.
	dnsNames := []string{host}
	var ipAddrs []net.IP
	if gatewayIP != "" {
		ip := net.ParseIP(gatewayIP)
		if ip != nil {
			ipAddrs = append(ipAddrs, ip)
		}
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"dcx proxy"},
			CommonName:   host,
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    dnsNames,
		IPAddresses: ipAddrs,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("creating server certificate: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing server certificate: %w", err)
	}

	return cert, key, nil
}
