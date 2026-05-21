// Package proxy provides a generic HTTPS reverse proxy infrastructure for
// injecting secrets into API requests from the devcontainer. The proxy runs
// inside the dcx process, listens on HTTPS with a self-signed certificate,
// and forwards requests to a configurable upstream URL after applying
// service-specific request rewriting and header injection.
//
// The proxy is designed to be extended by service-specific sub-packages
// (e.g. github, openai) that provide the Director, Transport, and
// environment variable configuration for their respective APIs. Each service
// gets its own proxy instance listening on a separate port.
//
// The user's credentials are never exposed inside the container — they exist
// only in the host-side dcx process memory and are never written to disk or
// logged.
package proxy

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
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// ProxyHost is the hostname used in the TLS certificate's Subject Alternative
// Name. The proxy generates a self-signed certificate with this hostname so
// clients can verify the TLS connection. In practice, GH_HOST is set to
// the Docker gateway IP (detected at runtime) plus the proxy port, so the
// certificate also includes an IP SAN for the gateway. The hostname SAN
// provides a fallback for environments where host.docker.internal is
// routable.
const ProxyHost = "host.docker.internal"

// Service implements a service-specific HTTPS reverse proxy. Each service
// (e.g. GitHub, OpenAI) creates its own Service instance with a distinct
// port, TLS certificate, and request handling logic. The service runs as an
// HTTPS server with a self-signed certificate generated on startup.
type Service struct {
	// name is the human-readable name of the service (e.g. "github", "openai"),
	// used in log messages to identify which proxy instance is acting.
	name string

	// opts holds the proxy configuration including network and TLS settings.
	opts Options

	// listener is the TCP listener the proxy server uses. Bound to the
	// configured address on a random available port so the proxy is reachable
	// from the Docker container.
	listener net.Listener

	// server is the HTTPS server that serves the proxy. It uses a self-signed
	// TLS certificate generated in-memory on startup.
	server *http.Server

	// caCertPEM is the PEM-encoded CA certificate, made available inside the
	// container so clients trust the proxy's TLS certificate.
	caCertPEM []byte

	// mu protects the done channel so Start and Shutdown can be called safely
	// from different goroutines.
	mu sync.Mutex

	// done is closed when the proxy server has fully shut down, allowing
	// callers to wait for clean termination.
	done chan struct{}
}

// Options holds the configuration for creating and starting a proxy service.
// All fields have sensible defaults — zero values fall back to those defaults.
type Options struct {
	// TLSEnabled controls whether the proxy uses TLS (HTTPS). When true, the
	// proxy generates self-signed CA and server certificates on startup, and
	// the CA cert is injected into the container so clients trust the proxy.
	// When false, the proxy listens on plain HTTP and no certificate
	// generation or injection occurs. Not all proxies require TLS — for
	// example, a proxy that only routes non-sensitive traffic can run without
	// it. Defaults to false; each provider sets this as appropriate for its
	// service.
	TLSEnabled bool

	// GatewayIP is the host's IP address on the Docker bridge network. The
	// container uses this IP to reach the proxy. It is included in the TLS
	// certificate's IP SANs so clients can verify the connection. Only
	// relevant when TLSEnabled is true.
	GatewayIP string

	// BindAddr is the address the proxy listens on. Defaults to GatewayIP
	// (more secure — only reachable from the container's network) if empty.
	// Set to "0.0.0.0" to listen on all interfaces (needed in some Docker
	// network setups).
	BindAddr string

	// APIURL is the upstream API URL to forward requests to. Must be set by
	// the service-specific constructor (e.g. "https://api.github.com" for
	// the GitHub proxy).
	APIURL string

	// CACertPath is the container path where the CA certificate is copied.
	// Must be set by the service-specific constructor. Only relevant when
	// TLSEnabled is true.
	CACertPath string

	// CertExpiry is the duration for which the generated TLS certificates
	// (both CA and server) are valid. Defaults to 24 hours if zero. The
	// certificates are ephemeral — they only need to last for the duration
	// of a dcx exec session. Only relevant when TLSEnabled is true.
	CertExpiry time.Duration
}

// CACertPathResolved returns the CA cert container path. Returns empty
// string when TLS is disabled. Called by callers that need the resolved CA
// cert path for building remote env vars or Docker copy operations.
func (o Options) CACertPathResolved() string {
	if !o.TLSEnabled {
		return ""
	}
	return o.CACertPath
}

// CABundlePathResolved returns the container path for the combined CA bundle
// (system certs + proxy CA cert). Returns empty string when TLS is disabled.
// The combined bundle is used for SSL_CERT_FILE so that Go programs trust
// both the system CAs and the proxy's self-signed CA. The path is derived
// from the CA cert path by replacing the extension with "-bundle.crt".
// NODE_EXTRA_CA_CERTS does not need this — Node.js appends to the system
// trust store rather than replacing it.
func (o Options) CABundlePathResolved() string {
	if !o.TLSEnabled {
		return ""
	}
	base := o.CACertPath
	if strings.HasSuffix(base, ".crt") {
		return base[:len(base)-4] + "-bundle.crt"
	}
	return base + "-bundle"
}

// APIURLResolved returns the upstream API URL to forward requests to. Called
// by callers that need the resolved API URL.
func (o Options) APIURLResolved() string {
	return o.APIURL
}

// CertExpiryResolved returns the certificate expiry duration. Defaults to
// 24 hours if zero. The certificates are ephemeral — they only need to last
// for the duration of a dcx exec session.
func (o Options) CertExpiryResolved() time.Duration {
	if o.CertExpiry == 0 {
		return 24 * time.Hour
	}
	return o.CertExpiry
}

// BindAddrResolved returns the effective bind address, using GatewayIP if the
// option is empty. Binding to the gateway IP only (rather than 0.0.0.0) is
// more secure as it limits the proxy's attack surface to the Docker bridge
// network.
func (o Options) BindAddrResolved() string {
	if o.BindAddr != "" {
		return o.BindAddr
	}
	return o.GatewayIP
}

// NewService creates a new proxy Service with the given name and options.
// The name is used in log messages to identify which proxy instance is acting.
// Call Start to begin serving requests, and Shutdown to stop the proxy.
// The service-specific Director and Transport must be provided via the
// Handler method or set after construction.
func NewService(name string, opts Options) *Service {
	return &Service{
		name: name,
		opts: opts,
		done: make(chan struct{}),
	}
}

// Start binds to a random available port on the configured bind address and
// starts the proxy server. When TLSEnabled is true in the options, it
// generates a self-signed TLS certificate and starts an HTTPS server; when
// false, it starts a plain HTTP server. It returns the port number the proxy
// is listening on. The caller should call Shutdown to stop the proxy when the
// devcontainer session ends. When TLS is enabled, the CA certificate PEM
// bytes are available via CACertPEM after Start returns.
//
// The handler parameter is the http.Handler that processes incoming requests.
// Typically this is a httputil.ReverseProxy configured by the service-specific
// sub-package with the appropriate Director and Transport.
func (s *Service) Start(handler http.Handler) (int, error) {
	// Bind to a random available port on the configured address. By default
	// this is the gateway IP (more secure — only reachable from the container's
	// network). Users can override via Options.BindAddr to e.g. "0.0.0.0" if
	// needed for their Docker network setup.
	bindAddr := s.opts.BindAddrResolved()
	listener, err := net.Listen("tcp", bindAddr+":0")
	if err != nil {
		return 0, fmt.Errorf("binding proxy listener on %s: %w", bindAddr, err)
	}
	s.listener = listener

	port := s.listener.Addr().(*net.TCPAddr).Port

	s.server = &http.Server{Handler: handler}

	// When TLS is enabled, generate self-signed CA and server certificates
	// in-memory, and configure the server's TLSConfig. The CA cert is made
	// available inside the container so clients trust the proxy's TLS
	// certificate.
	if s.opts.TLSEnabled {
		expiry := s.opts.CertExpiryResolved()
		caCert, caKey, err := generateCA(expiry)
		if err != nil {
			return 0, fmt.Errorf("generating CA certificate: %w", err)
		}

		serverCert, serverKey, err := generateServerCert(caCert, caKey, ProxyHost, s.opts.GatewayIP, expiry)
		if err != nil {
			return 0, fmt.Errorf("generating server certificate: %w", err)
		}

		// Store the CA certificate PEM for mounting into the container.
		s.caCertPEM = pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: caCert.Raw,
		})

		s.server.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{
				{
					Certificate: [][]byte{serverCert.Raw},
					PrivateKey:  serverKey,
					Leaf:        serverCert,
				},
			},
			MinVersion: tls.VersionTLS12,
		}
	}

	// Start serving in a goroutine so Start can return immediately.
	go func() {
		defer close(s.done)
		if s.opts.TLSEnabled {
			// ServeTLS is used because the proxy must speak HTTPS when
			// TLS is enabled. The TLS certificate and key are already
			// loaded into the server's TLSConfig, so the certFile and
			// keyFile parameters are empty strings.
			if err := s.server.ServeTLS(s.listener, "", ""); err != nil {
				if err != http.ErrServerClosed {
					slog.Error("proxy server error", "service", s.name, "error", err)
				}
			}
		} else {
			// Serve plain HTTP when TLS is not required.
			if err := s.server.Serve(s.listener); err != nil {
				if err != http.ErrServerClosed {
					slog.Error("proxy server error", "service", s.name, "error", err)
				}
			}
		}
	}()

	slog.Info("proxy started",
		"service", s.name,
		"port", port,
		"bind_addr", bindAddr,
		"tls", s.opts.TLSEnabled,
		"api_url", s.opts.APIURL)

	return port, nil
}

// Shutdown gracefully stops the proxy server. It waits for in-flight requests
// to complete (with a 5-second timeout) and then closes the listener. Call
// this when the devcontainer session ends to clean up the proxy.
func (s *Service) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		slog.Error("proxy shutdown error", "service", s.name, "error", err)
	}

	// Wait for the serving goroutine to finish.
	<-s.done

	slog.Info("proxy stopped", "service", s.name)
}

// CACertPEM returns the PEM-encoded CA certificate. This is copied into
// the container at the configured CA cert path so clients trust the proxy's
// self-signed TLS certificate via SSL_CERT_FILE and NODE_EXTRA_CA_CERTS.
func (s *Service) CACertPEM() []byte {
	return s.caCertPEM
}

// Opts returns a copy of the proxy's options. Used by callers that need
// access to the resolved configuration (e.g. CA cert paths for building
// remote env vars).
func (s *Service) Opts() Options {
	return s.opts
}

// ListenerAddr returns the address (host:port) the proxy is listening on.
// Returns empty string if the proxy hasn't been started. Used by callers
// that need to construct request URLs for the proxy.
func (s *Service) ListenerAddr() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// WriteCACertToFile writes the PEM-encoded CA certificate to a temporary file
// on the host. The file is used as an intermediate step before copying the cert
// into the container so clients trust the proxy's self-signed TLS certificate.
// Returns the path to the temp file. The caller should clean up the file when
// the proxy is shut down.
func WriteCACertToFile(caCertPEM []byte) (string, error) {
	tmp, err := os.CreateTemp("", "dcx-proxy-ca-*.crt")
	if err != nil {
		return "", fmt.Errorf("creating temp file for CA cert: %w", err)
	}
	defer func() { _ = tmp.Close() }()

	if _, err := tmp.Write(caCertPEM); err != nil {
		_ = os.Remove(tmp.Name())
		return "", fmt.Errorf("writing CA cert: %w", err)
	}

	return tmp.Name(), nil
}

// ErrorHandler logs proxy errors and returns a 502 Bad Gateway response to
// the client. This provides clearer error messages than the default reverse
// proxy error handler, which returns a generic "internal server error".
// Used as the reverse proxy's ErrorHandler by service sub-packages.
func ErrorHandler(serviceName string) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		slog.Error("proxy error", "service", serviceName, "path", r.URL.Path, "error", err)
		http.Error(w, fmt.Sprintf("proxy error: %v", err), http.StatusBadGateway)
	}
}

// NewReverseProxy creates a httputil.ReverseProxy that forwards requests to
// the given target URL. The Director is provided by the caller (typically a
// service-specific function that rewrites requests appropriately). The
// transport and error handler are configured from the generic infrastructure.
// This is a convenience function used by service sub-packages to avoid
// duplicating the reverse proxy setup logic.
func NewReverseProxy(target *url.URL, director func(*http.Request), transport http.RoundTripper, serviceName string) *httputil.ReverseProxy {
	reverseProxy := httputil.NewSingleHostReverseProxy(target)
	reverseProxy.Director = director
	reverseProxy.Transport = transport
	reverseProxy.ErrorHandler = ErrorHandler(serviceName)
	return reverseProxy
}

// generateCA creates a self-signed CA certificate and ECDSA private key. The
// certificate is valid for the given expiry duration (ephemeral — only for the
// duration of the proxy session). The key uses P-256 curve which is widely
// supported and provides adequate security for short-lived certificates.
// Returns the parsed certificate and private key. Called once on proxy startup.
func generateCA(expiry time.Duration) (*x509.Certificate, *ecdsa.PrivateKey, error) {
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
		NotAfter:              time.Now().Add(expiry),
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
// certificate and key. The certificate is valid for the given expiry duration
// and includes the given host as a DNS SAN (Subject Alternative Name) and the
// gatewayIP as an IP SAN. The IP SAN is needed because clients connect to the
// proxy via the Docker bridge gateway IP, and TLS verification checks the SAN
// against the connection target. The key uses P-256 curve.
// Returns the parsed certificate and private key. Called once on proxy startup
// after generateCA.
func generateServerCert(caCert *x509.Certificate, caKey *ecdsa.PrivateKey, host, gatewayIP string, expiry time.Duration) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generating server key: %w", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("generating server serial number: %w", err)
	}

	// Build the SAN lists. The DNS name is the hostname (host.docker.internal)
	// and the IP address is the Docker bridge gateway. Both are needed so
	// clients can verify the TLS connection regardless of which address they
	// use to reach the proxy.
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
		NotAfter:    time.Now().Add(expiry),
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
