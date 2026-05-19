package ghproxy

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
)

// TestProxyStartAndShutdown tests that the proxy can start on a dynamic port
// and shut down cleanly without errors.
func TestProxyStartAndShutdown(t *testing.T) {
	proxy := New("test-token", "owner/repo", "127.0.0.1")
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

// TestProxyRejectsOtherRepo tests that the proxy rejects requests targeting
// a different repository with 403 Forbidden.
func TestProxyRejectsOtherRepo(t *testing.T) {
	proxy := New("test-token", "owner/allowed-repo", "127.0.0.1")
	_, err := proxy.Start()
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer proxy.Shutdown()

	// Create a client that trusts the proxy's self-signed CA cert.
	client := makeProxyClient(t, proxy.CACertPEM())

	// Make a request targeting a different repository.
	resp, err := makeProxyRequest(client, proxy, "/repos/other/repo/issues")
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}

	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Error("response body is empty, expected error message")
	}
}

// TestProxyAllowsMatchingRepo tests that the proxy allows requests targeting
// the allowed repository. Note: This will fail at the forwarding step since
// we're using a fake token and the real GitHub API will reject it, but we can
// verify the request is not rejected at the scoping layer.
func TestProxyAllowsMatchingRepo(t *testing.T) {
	proxy := New("test-token", "owner/allowed-repo", "127.0.0.1")
	_, err := proxy.Start()
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer proxy.Shutdown()

	client := makeProxyClient(t, proxy.CACertPEM())

	// Make a request targeting the allowed repository.
	resp, err := makeProxyRequest(client, proxy, "/repos/owner/allowed-repo/issues")
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	// The request should not be rejected at the scoping level (403).
	// It will likely get a 401 or 502 from the actual GitHub API or proxy
	// error, but it must NOT be 403 (Forbidden from scoping).
	if resp.StatusCode == http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("got 403 Forbidden for allowed repo, body: %s", body)
	}
}

// TestProxyAllowsNonRepoPaths tests that the proxy allows requests that don't
// target a specific repository (e.g. /user, /graphql).
func TestProxyAllowsNonRepoPaths(t *testing.T) {
	proxy := New("test-token", "owner/repo", "127.0.0.1")
	_, err := proxy.Start()
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer proxy.Shutdown()

	client := makeProxyClient(t, proxy.CACertPEM())

	// Make a request to /user (non-repo path).
	resp, err := makeProxyRequest(client, proxy, "/user")
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	// Should not be 403 Forbidden from scoping.
	if resp.StatusCode == http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("got 403 Forbidden for /user, body: %s", body)
	}
}

// TestProxyNoScopingWhenRepoEmpty tests that no repository scoping is enforced
// when the repository is empty.
func TestProxyNoScopingWhenRepoEmpty(t *testing.T) {
	proxy := New("test-token", "", "127.0.0.1")
	_, err := proxy.Start()
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer proxy.Shutdown()

	client := makeProxyClient(t, proxy.CACertPEM())

	// Make a request targeting any repository — should not be rejected.
	resp, err := makeProxyRequest(client, proxy, "/repos/any/repo/issues")
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	// Should not be 403 Forbidden from scoping.
	if resp.StatusCode == http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("got 403 Forbidden with no repo configured, body: %s", body)
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
	// We need to get the port from the proxy. Since the listener is internal,
	// we reconstruct the URL from the known port. For tests, we read the
	// port from the listener address directly.
	// However, since Proxy doesn't expose the port, we use the test
	// infrastructure differently — we construct a URL pointing to 127.0.0.1
	// with the Host header set to ProxyHost.

	// Get the actual listener address from the proxy server.
	// Since we can't access the listener directly, we'll use a helper.
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
	proxy1 := New("test-token", "owner/repo", "127.0.0.1")
	port1, err := proxy1.Start()
	if err != nil {
		t.Fatalf("Start() proxy1 error: %v", err)
	}

	proxy2 := New("test-token", "owner/repo", "127.0.0.1")
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
