package proxy

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// TestNewServerGeneratesCA tests that creating a Server generates a CA
// certificate.
func TestNewServerGeneratesCA(t *testing.T) {
	srv, err := NewServer(1 * time.Hour)
	if err != nil {
		t.Fatalf("NewServer() error: %v", err)
	}

	caPEM := srv.CACertPEM()
	if len(caPEM) == 0 {
		t.Fatal("CACertPEM() returned empty bytes")
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		t.Fatal("failed to parse generated CA cert")
	}
}

// TestServerStartAndShutdown tests that the proxy can start on a dynamic port
// and shut down cleanly.
func TestServerStartAndShutdown(t *testing.T) {
	srv, err := NewServer(1 * time.Hour)
	if err != nil {
		t.Fatalf("NewServer() error: %v", err)
	}

	onRequest := func(req *http.Request) (*http.Request, *http.Response) { return req, nil }
	port, err := srv.Start("127.0.0.1", "127.0.0.1", []string{"example.com"}, onRequest)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if port <= 0 {
		t.Errorf("port = %d, want > 0", port)
	}

	srv.Shutdown()
}

// TestServerPortIsDynamic tests that starting two servers allocates different
// ports.
func TestServerPortIsDynamic(t *testing.T) {
	onRequest := func(req *http.Request) (*http.Request, *http.Response) { return req, nil }

	srv1, _ := NewServer(1 * time.Hour)
	port1, err := srv1.Start("127.0.0.1", "127.0.0.1", []string{"a.com"}, onRequest)
	if err != nil {
		t.Fatalf("Start() srv1 error: %v", err)
	}
	defer srv1.Shutdown()

	srv2, _ := NewServer(1 * time.Hour)
	port2, err := srv2.Start("127.0.0.1", "127.0.0.1", []string{"b.com"}, onRequest)
	if err != nil {
		t.Fatalf("Start() srv2 error: %v", err)
	}
	defer srv2.Shutdown()

	if port1 == port2 {
		t.Errorf("both servers got the same port %d, expected different dynamic ports", port1)
	}
}

// TestServerMITMInterceptsMatchingDomain tests that HTTPS traffic to a
// matching domain is MITM-intercepted and the injector is called.
func TestServerMITMInterceptsMatchingDomain(t *testing.T) {
	injected := false
	onRequest := func(req *http.Request) (*http.Request, *http.Response) {
		injected = true
		return req, nil
	}

	srv, _ := NewServer(1 * time.Hour)
	port, err := srv.Start("127.0.0.1", "127.0.0.1", []string{"example.com"}, onRequest)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer srv.Shutdown()

	// Simulate a CONNECT + TLS handshake through the proxy.
	// 1. Plain TCP to proxy.
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("dial proxy error: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// 2. Send CONNECT request.
	_, err = fmt.Fprintf(conn, "CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\n\r\n")
	if err != nil {
		t.Fatalf("write CONNECT error: %v", err)
	}

	// 3. Read 200 response.
	buf := make([]byte, 1024)
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline error: %v", err)
	}
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read CONNECT response error: %v", err)
	}
	if !strings.Contains(string(buf[:n]), "200") {
		t.Fatalf("expected 200 from CONNECT, got: %s", string(buf[:n]))
	}

	// 4. Upgrade to TLS with the proxy's CA cert and SNI for example.com.
	tlsConn := tls.Client(conn, &tls.Config{
		RootCAs:    makeTestPool(t, srv.CACertPEM()),
		ServerName: "example.com",
	})
	if err := tlsConn.Handshake(); err != nil {
		t.Fatalf("TLS handshake error: %v", err)
	}
	defer func() { _ = tlsConn.Close() }()

	// 5. Send an HTTP request over the TLS connection.
	_, _ = fmt.Fprintf(tlsConn, "GET /test HTTP/1.1\r\nHost: example.com\r\n\r\n")
	if err := tlsConn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline error: %v", err)
	}
	n, _ = tlsConn.Read(buf)
	if n == 0 {
		t.Fatal("no response from proxy after MITM TLS handshake")
	}
	if !injected {
		t.Fatal("injector was not called for matching domain")
	}
}

// TestServerTunnelsNonMatchingDomain tests that HTTPS traffic to a
// non-matching domain is tunneled without MITM.
func TestServerTunnelsNonMatchingDomain(t *testing.T) {
	// Start a real TLS upstream so the proxy can tunnel to it.
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("upstream"))
	}))
	defer upstream.Close()

	onRequest := func(req *http.Request) (*http.Request, *http.Response) {
		t.Error("onRequest should not be called for non-matching domain")
		return req, nil
	}

	srv, _ := NewServer(1 * time.Hour)
	port, err := srv.Start("127.0.0.1", "127.0.0.1", []string{"example.com"}, onRequest)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer srv.Shutdown()

	proxyURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	client := &http.Client{
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(mustParseURL(proxyURL)),
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	resp, err := client.Get(upstream.URL + "/test")
	if err != nil {
		t.Fatalf("request through proxy error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "upstream" {
		t.Errorf("response body = %q, want %q", string(body), "upstream")
	}
}

// TestWriteCACertToFile tests that the CA cert can be written to a temp file.
func TestWriteCACertToFile(t *testing.T) {
	srv, _ := NewServer(1 * time.Hour)
	path, err := WriteCACertToFile(srv.CACertPEM())
	if err != nil {
		t.Fatalf("WriteCACertToFile() error: %v", err)
	}
	defer func() { _ = os.Remove(path) }()

	if path == "" {
		t.Error("WriteCACertToFile() returned empty path")
	}
}

// makeTestPool creates an x509.CertPool containing the given CA cert.
func makeTestPool(t *testing.T, caCertPEM []byte) *x509.CertPool {
	t.Helper()
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caCertPEM) {
		t.Fatal("failed to append CA cert to pool")
	}
	return pool
}

// mustParseURL parses a URL string and panics on error.
func mustParseURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return u
}
