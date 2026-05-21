package proxy

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime"
	"testing"
	"time"
)

// TestServiceStartAndShutdownTLS tests that a TLS-enabled proxy service can
// start on a dynamic port and shut down cleanly, with CA cert available.
func TestServiceStartAndShutdownTLS(t *testing.T) {
	svc := NewService("test", Options{
		TLSEnabled: true,
		GatewayIP:  "127.0.0.1",
		APIURL:     "https://api.example.com",
		CACertPath: "/opt/dcx/test-proxy/ca.crt",
	})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	port, err := svc.Start(handler)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	if port <= 0 {
		t.Errorf("port = %d, want > 0", port)
	}

	// Verify CA cert is available when TLS is enabled.
	caPEM := svc.CACertPEM()
	if len(caPEM) == 0 {
		t.Error("CACertPEM() returned empty bytes")
	}

	svc.Shutdown()
}

// TestServiceStartAndShutdownNoTLS tests that a non-TLS proxy service can
// start on a dynamic port and shut down cleanly, with no CA cert generated.
func TestServiceStartAndShutdownNoTLS(t *testing.T) {
	svc := NewService("test", Options{
		TLSEnabled: false,
		GatewayIP:  "127.0.0.1",
		APIURL:     "https://api.example.com",
	})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	port, err := svc.Start(handler)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	if port <= 0 {
		t.Errorf("port = %d, want > 0", port)
	}

	// Verify no CA cert when TLS is disabled.
	caPEM := svc.CACertPEM()
	if len(caPEM) != 0 {
		t.Error("CACertPEM() should return empty bytes when TLS is disabled")
	}

	svc.Shutdown()
}

// TestServicePortIsDynamic tests that starting two services allocates
// different ports (verifying dynamic port allocation).
func TestServicePortIsDynamic(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	svc1 := NewService("test1", Options{
		TLSEnabled: true,
		GatewayIP:  "127.0.0.1",
		BindAddr:   "127.0.0.1",
		APIURL:     "https://api.example.com",
		CACertPath: "/opt/dcx/test1/ca.crt",
	})
	port1, err := svc1.Start(handler)
	if err != nil {
		t.Fatalf("Start() svc1 error: %v", err)
	}

	svc2 := NewService("test2", Options{
		TLSEnabled: true,
		GatewayIP:  "127.0.0.1",
		BindAddr:   "127.0.0.1",
		APIURL:     "https://api.example.com",
		CACertPath: "/opt/dcx/test2/ca.crt",
	})
	port2, err := svc2.Start(handler)
	if err != nil {
		svc1.Shutdown()
		t.Fatalf("Start() svc2 error: %v", err)
	}

	defer svc1.Shutdown()
	defer svc2.Shutdown()

	if port1 == port2 {
		t.Errorf("both services got the same port %d, expected different dynamic ports", port1)
	}
}

// TestServiceForwardsRequestsHTTPS tests that a TLS-enabled proxy service
// forwards requests to the upstream server via HTTPS.
func TestServiceForwardsRequestsHTTPS(t *testing.T) {
	// Start a fake upstream server.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("upstream response"))
	}))
	defer upstream.Close()

	upstreamURL, _ := url.Parse(upstream.URL)

	svc := NewService("test", Options{
		TLSEnabled: true,
		GatewayIP:  "127.0.0.1",
		APIURL:     upstream.URL,
		CACertPath: "/opt/dcx/test-proxy/ca.crt",
	})

	reverseProxy := NewReverseProxy(upstreamURL, func(req *http.Request) {
		req.URL.Scheme = upstreamURL.Scheme
		req.URL.Host = upstreamURL.Host
		req.Host = upstreamURL.Host
	}, http.DefaultTransport, "test")

	port, err := svc.Start(reverseProxy)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer svc.Shutdown()

	// Connect to the proxy with a client that trusts its CA cert.
	client := makeTestClient(t, svc.CACertPEM())

	resp, err := client.Get(fmt.Sprintf("https://127.0.0.1:%d/test", port))
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

// TestServiceForwardsRequestsHTTP tests that a non-TLS proxy service
// forwards requests to the upstream server via plain HTTP.
func TestServiceForwardsRequestsHTTP(t *testing.T) {
	// Start a fake upstream server.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("upstream response"))
	}))
	defer upstream.Close()

	upstreamURL, _ := url.Parse(upstream.URL)

	svc := NewService("test", Options{
		TLSEnabled: false,
		GatewayIP:  "127.0.0.1",
		APIURL:     upstream.URL,
	})

	reverseProxy := NewReverseProxy(upstreamURL, func(req *http.Request) {
		req.URL.Scheme = upstreamURL.Scheme
		req.URL.Host = upstreamURL.Host
		req.Host = upstreamURL.Host
	}, http.DefaultTransport, "test")

	port, err := svc.Start(reverseProxy)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer svc.Shutdown()

	// Connect to the proxy via plain HTTP (no TLS).
	client := &http.Client{}

	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/test", port))
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

// TestOptionsCABundlePath tests the CA bundle path derivation from the CA
// cert path when TLS is enabled.
func TestOptionsCABundlePath(t *testing.T) {
	tests := []struct {
		name       string
		caCertPath string
		want       string
	}{
		{
			name:       "standard path",
			caCertPath: "/opt/dcx/gh-proxy/ca.crt",
			want:       "/opt/dcx/gh-proxy/ca-bundle.crt",
		},
		{
			name:       "custom path",
			caCertPath: "/custom/path/cert.crt",
			want:       "/custom/path/cert-bundle.crt",
		},
		{
			name:       "path without crt extension",
			caCertPath: "/custom/path/cert.pem",
			want:       "/custom/path/cert.pem-bundle",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := Options{TLSEnabled: true, CACertPath: tt.caCertPath}
			got := opts.CABundlePathResolved()
			if got != tt.want {
				t.Errorf("CABundlePathResolved() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestOptionsCABundlePathNoTLS tests that CA bundle and CA cert paths return
// empty when TLS is disabled.
func TestOptionsCABundlePathNoTLS(t *testing.T) {
	opts := Options{TLSEnabled: false, CACertPath: "/opt/dcx/gh-proxy/ca.crt"}

	if got := opts.CABundlePathResolved(); got != "" {
		t.Errorf("CABundlePathResolved() = %q, want empty string when TLS disabled", got)
	}
	if got := opts.CACertPathResolved(); got != "" {
		t.Errorf("CACertPathResolved() = %q, want empty string when TLS disabled", got)
	}
}

// TestOptionsDefaults tests the resolved defaults for Options fields.
func TestOptionsDefaults(t *testing.T) {
	opts := Options{
		TLSEnabled: true,
		GatewayIP:  "172.17.0.1",
		CACertPath: "/opt/dcx/gh-proxy/ca.crt",
		APIURL:     "https://api.github.com",
		CertExpiry: 24 * time.Hour,
	}

	if got := opts.CACertPathResolved(); got != "/opt/dcx/gh-proxy/ca.crt" {
		t.Errorf("CACertPathResolved() = %q, want %q", got, "/opt/dcx/gh-proxy/ca.crt")
	}
	if got := opts.APIURLResolved(); got != "https://api.github.com" {
		t.Errorf("APIURLResolved() = %q, want %q", got, "https://api.github.com")
	}
	if got := opts.CertExpiryResolved(); got != 24*time.Hour {
		t.Errorf("CertExpiryResolved() = %v, want %v", got, 24*time.Hour)
	}

	// BindAddr default is platform-dependent.
	var wantDefaultBindAddr string
	if runtime.GOOS == "linux" {
		wantDefaultBindAddr = "172.17.0.1"
	} else {
		wantDefaultBindAddr = "127.0.0.1"
	}
	if got := opts.BindAddrResolved(); got != wantDefaultBindAddr {
		t.Errorf("BindAddrResolved() = %q, want %q", got, wantDefaultBindAddr)
	}

	// Override bind address.
	opts.BindAddr = "0.0.0.0"
	if got := opts.BindAddrResolved(); got != "0.0.0.0" {
		t.Errorf("BindAddrResolved() = %q, want %q", got, "0.0.0.0")
	}

	// Default cert expiry when zero.
	opts.CertExpiry = 0
	if got := opts.CertExpiryResolved(); got != 24*time.Hour {
		t.Errorf("CertExpiryResolved() = %v, want %v (default)", got, 24*time.Hour)
	}
}

// TestWriteCACertToFile tests that the CA cert can be written to a temp file.
func TestWriteCACertToFile(t *testing.T) {
	// Generate a CA cert via a TLS-enabled proxy service.
	svc := NewService("test", Options{
		TLSEnabled: true,
		GatewayIP:  "127.0.0.1",
		APIURL:     "https://api.example.com",
		CACertPath: "/opt/dcx/test/ca.crt",
	})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	_, err := svc.Start(handler)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer svc.Shutdown()

	path, err := WriteCACertToFile(svc.CACertPEM())
	if err != nil {
		t.Fatalf("WriteCACertToFile() error: %v", err)
	}
	defer func() { _ = removeFile(path) }()

	if path == "" {
		t.Error("WriteCACertToFile() returned empty path")
	}
}

// makeTestClient creates an http.Client that trusts the given CA certificate
// for TLS connections. Used in tests to connect to the proxy's self-signed
// HTTPS server.
func makeTestClient(t *testing.T, caCertPEM []byte) *http.Client {
	t.Helper()

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCertPEM) {
		t.Fatal("failed to append CA cert to pool")
	}

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:            caCertPool,
				InsecureSkipVerify: true, // Skip name verification in tests.
			},
		},
	}
}

// removeFile is a test helper that removes a file, used for cleaning up temp files.
func removeFile(path string) error {
	return os.Remove(path)
}
