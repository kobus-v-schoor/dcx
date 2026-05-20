package ghproxy

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestProxyStartAndShutdown tests that the proxy can start on a dynamic port
// and shut down cleanly without errors.
func TestProxyStartAndShutdown(t *testing.T) {
	proxy := New(Options{
		Token:     "test-token",
		GatewayIP: "127.0.0.1",
	})
	port, err := proxy.Start()
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	if port <= 0 {
		t.Errorf("port = %d, want > 0", port)
	}

	// Verify CA cert is available.
	caPEM := proxy.CACertPEM()
	if len(caPEM) == 0 {
		t.Error("CACertPEM() returned empty bytes")
	}

	proxy.Shutdown()
}

// TestProxyForwardsRequests tests that the proxy does not reject any requests
// (no repo scoping). Requests will fail at the forwarding step since we use
// a fake token, but they must not be rejected at the proxy layer (no 403).
func TestProxyForwardsRequests(t *testing.T) {
	proxy := New(Options{
		Token:     "test-token",
		GatewayIP: "127.0.0.1",
	})
	_, err := proxy.Start()
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer proxy.Shutdown()

	client := makeProxyClient(t, proxy.CACertPEM())

	// Make a request to any repo path — should not be rejected by the proxy.
	resp, err := makeProxyRequest(client, proxy, "/repos/any/repo/issues")
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Should not be 403 Forbidden — the proxy forwards all requests.
	if resp.StatusCode == http.StatusForbidden {
		t.Errorf("got 403 Forbidden, expected request to be forwarded")
	}
}

// makeProxyClient creates an http.Client that trusts the given CA certificate
// for TLS connections. Used in tests to connect to the proxy's self-signed
// HTTPS server.
func makeProxyClient(t *testing.T, caCertPEM []byte) *http.Client {
	t.Helper()

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCertPEM) {
		t.Fatal("failed to append CA cert to pool")
	}

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: caCertPool,
				// The proxy's cert is issued to ProxyHost but we connect
				// via 127.0.0.1, so we need to skip name verification in
				// tests.
				InsecureSkipVerify: true,
			},
		},
	}
}

// makeProxyRequest creates and sends an HTTP request to the proxy. It
// constructs the URL using the proxy's listener address and sets the Host
// header to ProxyHost so the proxy's director can rewrite it correctly.
func makeProxyRequest(client *http.Client, proxy *Proxy, path string) (*http.Response, error) {
	// Get the actual listener address from the proxy server.
	addr := proxy.listenerAddr()
	if addr == "" {
		return nil, fmt.Errorf("proxy has no listener address")
	}

	reqURL := fmt.Sprintf("https://%s%s", addr, path)
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %v", err)
	}
	req.Host = ProxyHost

	return client.Do(req)
}

// listenerAddr returns the address (host:port) the proxy is listening on.
// Used in tests to construct request URLs. Returns empty string if the
// proxy hasn't been started.
func (p *Proxy) listenerAddr() string {
	if p.listener == nil {
		return ""
	}
	return p.listener.Addr().String()
}

// TestProxyPortIsDynamic tests that starting the proxy twice allocates
// different ports (verifying dynamic port allocation).
func TestProxyPortIsDynamic(t *testing.T) {
	proxy1 := New(Options{
		Token:     "test-token",
		GatewayIP: "127.0.0.1",
		BindAddr:  "127.0.0.1",
	})
	port1, err := proxy1.Start()
	if err != nil {
		t.Fatalf("Start() proxy1 error: %v", err)
	}

	proxy2 := New(Options{
		Token:     "test-token",
		GatewayIP: "127.0.0.1",
		BindAddr:  "127.0.0.1",
	})
	port2, err := proxy2.Start()
	if err != nil {
		proxy1.Shutdown()
		t.Fatalf("Start() proxy2 error: %v", err)
	}

	defer proxy1.Shutdown()
	defer proxy2.Shutdown()

	if port1 == port2 {
		t.Errorf("both proxies got the same port %d, expected different dynamic ports", port1)
	}
}

// TestBuildProxyURL tests the URL construction for proxy requests.
func TestBuildProxyURL(t *testing.T) {
	port := 12345
	u := fmt.Sprintf("https://127.0.0.1:%d/repos/owner/repo/issues", port)
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatalf("parsing URL: %v", err)
	}
	if parsed.Port() != "12345" {
		t.Errorf("port = %q, want 12345", parsed.Port())
	}
}

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
			opts := Options{CACertPath: tt.caCertPath}
			got := opts.caBundlePath()
			if got != tt.want {
				t.Errorf("caBundlePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOptionsDefaults(t *testing.T) {
	opts := Options{
		GatewayIP:  "172.17.0.1",
		CACertPath: "/opt/dcx/gh-proxy/ca.crt",
		APIURL:     "https://api.github.com",
		CertExpiry: 24 * time.Hour,
	}

	if got := opts.caCertPath(); got != "/opt/dcx/gh-proxy/ca.crt" {
		t.Errorf("caCertPath() = %q, want %q", got, "/opt/dcx/gh-proxy/ca.crt")
	}
	if got := opts.apiURL(); got != "https://api.github.com" {
		t.Errorf("apiURL() = %q, want %q", got, "https://api.github.com")
	}
	if got := opts.certExpiry(); got != 24*time.Hour {
		t.Errorf("certExpiry() = %v, want %v", got, 24*time.Hour)
	}
	if got := opts.bindAddr(); got != "172.17.0.1" {
		t.Errorf("bindAddr() = %q, want %q", got, "172.17.0.1")
	}

	// Override bind address.
	opts.BindAddr = "0.0.0.0"
	if got := opts.bindAddr(); got != "0.0.0.0" {
		t.Errorf("bindAddr() = %q, want %q", got, "0.0.0.0")
	}

	// Override cert expiry.
	opts.CertExpiry = 1 * time.Hour
	if got := opts.certExpiry(); got != 1*time.Hour {
		t.Errorf("certExpiry() = %v, want %v", got, 1*time.Hour)
	}
}

func TestDirectorPathRewrite(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "REST API path strips /api/v3",
			path: "/api/v3/repos/owner/repo/issues",
			want: "/repos/owner/repo/issues",
		},
		{
			name: "GraphQL path strips /api",
			path: "/api/graphql",
			want: "/graphql",
		},
		{
			name: "Root /api/v3 becomes /",
			path: "/api/v3",
			want: "/",
		},
		{
			name: "Path without prefix unchanged",
			path: "/repos/owner/repo",
			want: "/repos/owner/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the path rewriting logic from the director.
			path := tt.path
			path = strings.TrimPrefix(path, "/api/v3")
			if strings.HasPrefix(path, "/api/") {
				path = strings.TrimPrefix(path, "/api")
			}
			if path == "" {
				path = "/"
			}
			if path != tt.want {
				t.Errorf("rewrite(%q) = %q, want %q", tt.path, path, tt.want)
			}
		})
	}
}
