// Package proxy provides a transparent MITM proxy infrastructure for
// injecting secrets into API requests from the devcontainer. A single proxy
// server runs inside the dcx process during dcx exec sessions. It intercepts
// HTTPS traffic to configured domains, decrypts it using a temporary CA
// certificate, injects credentials via registered providers, and re-encrypts
// the traffic before forwarding to the real destination.
//
// The proxy is designed to be extended by service-specific sub-packages
// (e.g. github) that register a Provider with the domains they handle and
// the logic to inject credentials for those domains.
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
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/elazarl/goproxy"
)

// Server is a single MITM proxy server that intercepts HTTPS traffic for
// configured domains and injects credentials into matching requests. It uses
// goproxy under the hood and generates a temporary CA certificate on startup
// that is injected into the container's system trust store so clients inside
// the container trust the proxy's dynamically-generated per-host certificates.
type Server struct {
	// listener is the TCP listener the proxy server uses. Bound to a random
	// available port so the proxy is reachable from the Docker container.
	listener net.Listener

	// srv is the HTTP server that serves the goproxy handler.
	srv *http.Server

	// caCert is the parsed CA certificate and key used to sign per-host
	// certificates during MITM interception.
	caCert tls.Certificate

	// caCertPEM is the PEM-encoded CA certificate, injected into the
	// container's system CA bundle so clients trust the proxy.
	caCertPEM []byte

	// mu protects shutdown and port.
	mu sync.Mutex

	// done is closed when the proxy server has fully shut down.
	done chan struct{}

	// port is the listening port assigned on Start.
	port int
}

// NewServer creates a new proxy Server with a freshly-generated CA
// certificate. The CA certificate is ephemeral — it only needs to last for the
// duration of a dcx exec session. The certExpiry controls how long the
// certificate remains valid.
func NewServer(certExpiry time.Duration) (*Server, error) {
	if certExpiry == 0 {
		certExpiry = 24 * time.Hour
	}

	caCertDER, caKey, err := generateCA(certExpiry)
	if err != nil {
		return nil, fmt.Errorf("generating CA certificate: %w", err)
	}

	caTLS := tls.Certificate{
		Certificate: [][]byte{caCertDER.Raw},
		PrivateKey:  caKey,
		Leaf:        caCertDER,
	}

	caPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caCertDER.Raw,
	})

	return &Server{
		caCert:    caTLS,
		caCertPEM: caPEM,
		done:      make(chan struct{}),
	}, nil
}

// Start binds to a random available port on the configured bind address and
// starts the MITM proxy. The domains list determines which hosts the proxy
// will intercept (MITM). Requests to matching domains have the onRequest
// callback applied so providers can filter and inject credentials.
// Non-matching domains are tunneled transparently without decryption.
//
// The onRequest callback receives the intercepted request and returns the
// (possibly modified) request and an optional response. If the response is
// non-nil, the proxy short-circuits and returns that response to the client
// without forwarding the request — this is how providers block disallowed
// requests.
//
// It returns the port number the proxy is listening on. The caller should
// call Shutdown when the devcontainer session ends.
func (s *Server) Start(gatewayIP, bindAddr string, domains []string, onRequest func(*http.Request) (*http.Request, *http.Response)) (int, error) {
	domainSet := make(map[string]struct{}, len(domains))
	for _, d := range domains {
		domainSet[strings.ToLower(d)] = struct{}{}
	}

	gpxy := goproxy.NewProxyHttpServer()
	gpxy.Verbose = false
	// Disable proxy chaining so the proxy always connects directly to
	// upstreams. goproxy's constructor sets ConnectDial from the
	// HTTPS_PROXY environment variable by default; clearing it prevents
	// chained proxy failures in environments where those vars are set.
	gpxy.ConnectDial = nil
	gpxy.ConnectDialWithReq = nil
	gpxy.Tr.Proxy = nil

	// Configure MITM for matching domains. For non-matching domains the
	// handler returns nil so goproxy falls through to the default
	// ConnectAccept behaviour (plain tunnel).
	mitmAction := &goproxy.ConnectAction{
		Action:    goproxy.ConnectMitm,
		TLSConfig: goproxy.TLSConfigFromCA(&s.caCert),
	}
	gpxy.OnRequest().HandleConnect(goproxy.FuncHttpsHandler(func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
		if _, ok := domainSet[strings.ToLower(stripPort(host))]; ok {
			return mitmAction, host
		}
		return nil, ""
	}))

	// Intercept requests to matching domains and run the onRequest callback.
	gpxy.OnRequest().DoFunc(func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		host := strings.ToLower(stripPort(req.Host))
		if host == "" {
			host = strings.ToLower(stripPort(req.URL.Host))
		}
		if _, ok := domainSet[host]; ok {
			return onRequest(req)
		}
		return req, nil
	})

	// Bind to a random available port on the configured address.
	addr := bindAddrResolved(bindAddr, gatewayIP)
	ln, err := net.Listen("tcp", addr+":0")
	if err != nil {
		return 0, fmt.Errorf("binding proxy listener on %s: %w", addr, err)
	}

	s.mu.Lock()
	s.listener = ln
	s.port = ln.Addr().(*net.TCPAddr).Port
	s.mu.Unlock()

	s.srv = &http.Server{Handler: gpxy}

	go func() {
		defer close(s.done)
		if err := s.srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			slog.Error("proxy server error", "error", err)
		}
	}()

	slog.Info("proxy started", "port", s.port, "bind_addr", addr, "domains", domains)

	return s.port, nil
}

// Shutdown gracefully stops the proxy server. It waits for in-flight requests
// to complete (with a 5-second timeout) and then closes the listener. Call
// this when the devcontainer session ends to clean up the proxy.
func (s *Server) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.srv == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.srv.Shutdown(ctx); err != nil {
		slog.Error("proxy shutdown error", "error", err)
	}

	<-s.done

	slog.Info("proxy stopped")
}

// CACertPEM returns the PEM-encoded CA certificate. This is injected into the
// container's system CA bundle so clients trust the proxy's dynamically
// generated per-host certificates.
func (s *Server) CACertPEM() []byte {
	return s.caCertPEM
}

// Port returns the listening port assigned on Start. Returns 0 if the server
// has not been started.
func (s *Server) Port() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.port
}

// stripPort removes the port suffix from a host string. If the string has no
// port, it is returned unchanged.
func stripPort(host string) string {
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		return host
	}
	return h
}

// bindAddrResolved returns the effective bind address. If bindAddr is set
// explicitly, that value is used. Otherwise, on Linux the proxy binds to the
// Docker gateway IP, and on non-Linux hosts it binds to 127.0.0.1 since
// containers reach the host via host.docker.internal which routes to localhost
// on Docker Desktop / Colima.
func bindAddrResolved(bindAddr, gatewayIP string) string {
	if bindAddr != "" {
		return bindAddr
	}
	if runtime.GOOS != "linux" {
		return "127.0.0.1"
	}
	return gatewayIP
}

// WriteCACertToFile writes the PEM-encoded CA certificate to a temporary file
// on the host. Returns the path to the temp file. The caller should clean up
// the file when done.
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
