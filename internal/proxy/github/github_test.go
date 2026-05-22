package github

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kobus-v-schoor/dcx/internal/config"
	"github.com/kobus-v-schoor/dcx/internal/proxy"
)

// TestProviderName tests that the provider returns the correct name.
func TestProviderName(t *testing.T) {
	p := &githubProvider{}
	if got := p.Name(); got != "github" {
		t.Errorf("Name() = %q, want %q", got, "github")
	}
}

// TestProviderEnabled tests that the provider reports enabled based on config.
func TestProviderEnabled(t *testing.T) {
	p := &githubProvider{}

	cfg := &config.Config{Proxy: config.ProxyConfig{GitHub: config.GitHubProxyConfig{Enabled: true}}}
	if !p.Enabled(cfg) {
		t.Error("Enabled() = false, want true when proxy.github.enabled is true")
	}

	cfg.Proxy.GitHub.Enabled = false
	if p.Enabled(cfg) {
		t.Error("Enabled() = true, want false when proxy.github.enabled is false")
	}
}

// TestProviderServiceOptions tests that ServiceOptions returns the expected
// proxy.Options with TLS enabled and config values populated.
func TestProviderServiceOptions(t *testing.T) {
	p := &githubProvider{}
	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			GitHub: config.GitHubProxyConfig{
				Enabled:    true,
				BindAddr:   "0.0.0.0",
				APIURL:     "https://github.example.com/api/v3",
				CACertPath: "/custom/ca.crt",
				CertExpiry: 1 * time.Hour,
			},
		},
	}

	opts := p.ServiceOptions(cfg, "172.17.0.1")

	if !opts.TLSEnabled {
		t.Error("TLSEnabled = false, want true (GitHub proxy always uses TLS)")
	}
	if opts.GatewayIP != "172.17.0.1" {
		t.Errorf("GatewayIP = %q, want %q", opts.GatewayIP, "172.17.0.1")
	}
	if opts.BindAddr != "0.0.0.0" {
		t.Errorf("BindAddr = %q, want %q", opts.BindAddr, "0.0.0.0")
	}
	if opts.APIURL != "https://github.example.com/api/v3" {
		t.Errorf("APIURL = %q, want %q", opts.APIURL, "https://github.example.com/api/v3")
	}
	if opts.CACertPath != "/custom/ca.crt" {
		t.Errorf("CACertPath = %q, want %q", opts.CACertPath, "/custom/ca.crt")
	}
	if opts.CertExpiry != 1*time.Hour {
		t.Errorf("CertExpiry = %v, want %v", opts.CertExpiry, 1*time.Hour)
	}
}

// TestProviderServiceOptionsDefaults tests that ServiceOptions fills in
// defaults when config values are empty/zero.
func TestProviderServiceOptionsDefaults(t *testing.T) {
	p := &githubProvider{}
	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			GitHub: config.GitHubProxyConfig{Enabled: true},
		},
	}

	opts := p.ServiceOptions(cfg, "172.17.0.1")

	if opts.APIURL != "https://api.github.com" {
		t.Errorf("APIURL = %q, want default %q", opts.APIURL, "https://api.github.com")
	}
	if opts.CACertPath != "/opt/dcx/gh-proxy/ca.crt" {
		t.Errorf("CACertPath = %q, want default %q", opts.CACertPath, "/opt/dcx/gh-proxy/ca.crt")
	}
	if opts.CertExpiry != 24*time.Hour {
		t.Errorf("CertExpiry = %v, want default %v", opts.CertExpiry, 24*time.Hour)
	}
}

// TestProviderRemoteEnvVars tests that RemoteEnvVars produces the correct
// GitHub-specific environment variable flags.
func TestProviderRemoteEnvVars(t *testing.T) {
	// Override repo and remote URL detection so the test is deterministic
	// regardless of whether it runs inside a git repository.
	oldDetectRepo := detectRepoFunc
	detectRepoFunc = func() (string, bool) { return "", false }
	defer func() { detectRepoFunc = oldDetectRepo }()

	oldDetectRemoteURL := detectRemoteURLFunc
	detectRemoteURLFunc = func() (string, bool) { return "", false }
	defer func() { detectRemoteURLFunc = oldDetectRemoteURL }()

	p := &githubProvider{}
	opts := proxy.Options{
		TLSEnabled: true,
		GatewayIP:  "172.17.0.1",
		CACertPath: "/opt/dcx/gh-proxy/ca.crt",
	}
	cfg := &config.Config{}

	envVars := p.RemoteEnvVars(12345, opts, cfg)

	// GH_HOST is platform-dependent.
	var expectedGHHost string
	if runtime.GOOS == "linux" {
		expectedGHHost = "--remote-env=GH_HOST=172.17.0.1:12345"
	} else {
		expectedGHHost = "--remote-env=GH_HOST=host.docker.internal:12345"
	}

	foundGHHost := false
	for _, env := range envVars {
		if strings.HasPrefix(env, "--remote-env=GH_HOST=") {
			foundGHHost = true
			if env != expectedGHHost {
				t.Errorf("GH_HOST = %q, want %q", env, expectedGHHost)
			}
		}
	}
	if !foundGHHost {
		t.Error("RemoteEnvVars missing GH_HOST")
	}

	// Should NOT contain SSL_CERT_FILE or NODE_EXTRA_CA_CERTS — those are
	// generic TLS env vars handled by the proxy infrastructure.
	for _, env := range envVars {
		if strings.Contains(env, "SSL_CERT_FILE") {
			t.Errorf("RemoteEnvVars should not contain SSL_CERT_FILE, got %q", env)
		}
		if strings.Contains(env, "NODE_EXTRA_CA_CERTS") {
			t.Errorf("RemoteEnvVars should not contain NODE_EXTRA_CA_CERTS, got %q", env)
		}
	}

	// Should NOT contain git config env vars.
	for _, env := range envVars {
		if strings.Contains(env, "GIT_CONFIG_") {
			t.Errorf("RemoteEnvVars should not contain GIT_CONFIG_*, got %q", env)
		}
	}

	// Should contain exactly one env var (GH_HOST) when no repo is detected.
	if len(envVars) != 1 {
		t.Errorf("RemoteEnvVars returned %d env vars, want 1", len(envVars))
	}
}

// TestProviderRemoteEnvVarsWithRepo tests that RemoteEnvVars includes
// GH_REPO when a repository is detected.
func TestProviderRemoteEnvVarsWithRepo(t *testing.T) {
	oldDetectRepo := detectRepoFunc
	detectRepoFunc = func() (string, bool) { return "owner/repo", true }
	defer func() { detectRepoFunc = oldDetectRepo }()

	oldDetectRemoteURL := detectRemoteURLFunc
	detectRemoteURLFunc = func() (string, bool) { return "", false }
	defer func() { detectRemoteURLFunc = oldDetectRemoteURL }()

	p := &githubProvider{}
	opts := proxy.Options{
		TLSEnabled: true,
		GatewayIP:  "172.17.0.1",
		CACertPath: "/opt/dcx/gh-proxy/ca.crt",
	}
	cfg := &config.Config{}

	envVars := p.RemoteEnvVars(12345, opts, cfg)

	var foundGHHost, foundGHRepo bool
	for _, env := range envVars {
		if strings.HasPrefix(env, "--remote-env=GH_HOST=") {
			foundGHHost = true
		}
		if env == "--remote-env=GH_REPO=owner/repo" {
			foundGHRepo = true
		}
	}
	if !foundGHHost {
		t.Error("RemoteEnvVars missing GH_HOST")
	}
	if !foundGHRepo {
		t.Error("RemoteEnvVars missing GH_REPO=owner/repo")
	}

	// Should contain exactly two env vars (GH_HOST and GH_REPO).
	if len(envVars) != 2 {
		t.Errorf("RemoteEnvVars returned %d env vars, want 2", len(envVars))
	}
}

// TestProviderRemoteEnvVarsWithHTTPSRemote tests that RemoteEnvVars includes
// git URL rewriting and SSL CA config when the origin remote is HTTPS.
func TestProviderRemoteEnvVarsWithHTTPSRemote(t *testing.T) {
	oldDetectRepo := detectRepoFunc
	detectRepoFunc = func() (string, bool) { return "owner/repo", true }
	defer func() { detectRepoFunc = oldDetectRepo }()

	oldDetectRemoteURL := detectRemoteURLFunc
	detectRemoteURLFunc = func() (string, bool) { return "https://github.com/owner/repo.git", true }
	defer func() { detectRemoteURLFunc = oldDetectRemoteURL }()

	p := &githubProvider{}
	opts := proxy.Options{
		TLSEnabled: true,
		GatewayIP:  "172.17.0.1",
		CACertPath: "/opt/dcx/gh-proxy/ca.crt",
	}
	cfg := &config.Config{}

	envVars := p.RemoteEnvVars(12345, opts, cfg)

	var foundGHHost, foundGHRepo bool
	var foundGitCount, foundGitKey0, foundGitVal0, foundGitKey1, foundGitVal1 bool
	for _, env := range envVars {
		switch {
		case strings.HasPrefix(env, "--remote-env=GH_HOST="):
			foundGHHost = true
		case env == "--remote-env=GH_REPO=owner/repo":
			foundGHRepo = true
		case env == "--remote-env=GIT_CONFIG_COUNT=2":
			foundGitCount = true
		case strings.HasPrefix(env, "--remote-env=GIT_CONFIG_KEY_0=url.") && strings.Contains(env, ".insteadOf"):
			foundGitKey0 = true
		case env == "--remote-env=GIT_CONFIG_VALUE_0=https://github.com/":
			foundGitVal0 = true
		case env == "--remote-env=GIT_CONFIG_KEY_1=http.sslCAInfo":
			foundGitKey1 = true
		case env == "--remote-env=GIT_CONFIG_VALUE_1=/opt/dcx/gh-proxy/ca.crt":
			foundGitVal1 = true
		}
	}
	if !foundGHHost {
		t.Error("RemoteEnvVars missing GH_HOST")
	}
	if !foundGHRepo {
		t.Error("RemoteEnvVars missing GH_REPO")
	}
	if !foundGitCount {
		t.Error("RemoteEnvVars missing GIT_CONFIG_COUNT")
	}
	if !foundGitKey0 {
		t.Error("RemoteEnvVars missing GIT_CONFIG_KEY_0")
	}
	if !foundGitVal0 {
		t.Error("RemoteEnvVars missing GIT_CONFIG_VALUE_0")
	}
	if !foundGitKey1 {
		t.Error("RemoteEnvVars missing GIT_CONFIG_KEY_1")
	}
	if !foundGitVal1 {
		t.Error("RemoteEnvVars missing GIT_CONFIG_VALUE_1")
	}

	// Should contain exactly 7 env vars (GH_HOST, GH_REPO, and 5 GIT_CONFIG_*).
	if len(envVars) != 7 {
		t.Errorf("RemoteEnvVars returned %d env vars, want 7", len(envVars))
	}
}

// TestProxyStartAndShutdown tests that the GitHub proxy can start on a dynamic
// port and shut down cleanly without errors, using the Provider interface.
func TestProxyStartAndShutdown(t *testing.T) {
	t.Setenv("GH_TOKEN", "fake-token-for-testing")
	p := &githubProvider{}
	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			GitHub: config.GitHubProxyConfig{Enabled: true},
		},
	}
	opts := p.ServiceOptions(cfg, "127.0.0.1")

	handler, err := p.CreateHandler(opts, cfg)
	if err != nil {
		t.Fatalf("CreateHandler() error: %v", err)
	}

	svc := proxy.NewService("github", opts)
	port, err := svc.Start(handler)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	if port <= 0 {
		t.Errorf("port = %d, want > 0", port)
	}

	// Verify CA cert is available (TLS is enabled for GitHub).
	caPEM := svc.CACertPEM()
	if len(caPEM) == 0 {
		t.Error("CACertPEM() returned empty bytes")
	}

	svc.Shutdown()
}

// TestProxyForwardsRequests tests that the proxy does not reject any requests
// (no repo scoping). Requests will fail at the forwarding step since we use
// a fake token, but they must not be rejected at the proxy layer (no 403).
func TestProxyForwardsRequests(t *testing.T) {
	t.Setenv("GH_TOKEN", "fake-token-for-testing")
	p := &githubProvider{}
	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			GitHub: config.GitHubProxyConfig{Enabled: true},
		},
	}
	opts := p.ServiceOptions(cfg, "127.0.0.1")

	handler, err := p.CreateHandler(opts, cfg)
	if err != nil {
		t.Fatalf("CreateHandler() error: %v", err)
	}

	svc := proxy.NewService("github", opts)
	_, err = svc.Start(handler)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer svc.Shutdown()

	client := makeProxyClient(t, svc.CACertPEM())

	resp, err := makeProxyRequest(client, svc, "/repos/any/repo/issues")
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Should not be 403 Forbidden — the proxy forwards all requests.
	if resp.StatusCode == http.StatusForbidden {
		t.Errorf("got 403 Forbidden, expected request to be forwarded")
	}
}

// TestProxyPortIsDynamic tests that starting the proxy twice allocates
// different ports (verifying dynamic port allocation).
func TestProxyPortIsDynamic(t *testing.T) {
	t.Setenv("GH_TOKEN", "fake-token-for-testing")
	p := &githubProvider{}
	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			GitHub: config.GitHubProxyConfig{Enabled: true},
		},
	}
	opts1 := p.ServiceOptions(cfg, "127.0.0.1")
	opts1.BindAddr = "127.0.0.1"

	handler1, err := p.CreateHandler(opts1, cfg)
	if err != nil {
		t.Fatalf("CreateHandler() opts1 error: %v", err)
	}

	svc1 := proxy.NewService("github1", opts1)
	port1, err := svc1.Start(handler1)
	if err != nil {
		t.Fatalf("Start() svc1 error: %v", err)
	}

	opts2 := p.ServiceOptions(cfg, "127.0.0.1")
	opts2.BindAddr = "127.0.0.1"

	handler2, err := p.CreateHandler(opts2, cfg)
	if err != nil {
		svc1.Shutdown()
		t.Fatalf("CreateHandler() opts2 error: %v", err)
	}

	svc2 := proxy.NewService("github2", opts2)
	port2, err := svc2.Start(handler2)
	if err != nil {
		svc1.Shutdown()
		t.Fatalf("Start() svc2 error: %v", err)
	}

	defer svc1.Shutdown()
	defer svc2.Shutdown()

	if port1 == port2 {
		t.Errorf("both proxies got the same port %d, expected different dynamic ports", port1)
	}
}

// TestDirectorPathRewrite tests the path rewriting logic from the director
// for API requests. Git requests should not have their paths rewritten.
func TestDirectorPathRewrite(t *testing.T) {
	apiTarget, _ := url.Parse("https://api.github.com")
	gitTarget, _ := url.Parse("https://github.com")
	director := newDirector(apiTarget, gitTarget)

	tests := []struct {
		name     string
		path     string
		query    string
		wantPath string
		wantHost string
	}{
		{
			name:     "REST API path strips /api/v3",
			path:     "/api/v3/repos/owner/repo/issues",
			wantPath: "/repos/owner/repo/issues",
			wantHost: "api.github.com",
		},
		{
			name:     "GraphQL path strips /api",
			path:     "/api/graphql",
			wantPath: "/graphql",
			wantHost: "api.github.com",
		},
		{
			name:     "Root /api/v3 becomes /",
			path:     "/api/v3",
			wantPath: "/",
			wantHost: "api.github.com",
		},
		{
			name:     "Path without prefix unchanged",
			path:     "/repos/owner/repo",
			wantPath: "/repos/owner/repo",
			wantHost: "api.github.com",
		},
		{
			name:     "Git upload-pack forwarded to git host",
			path:     "/owner/repo.git/git-upload-pack",
			wantPath: "/owner/repo.git/git-upload-pack",
			wantHost: "github.com",
		},
		{
			name:     "Git info/refs forwarded to git host",
			path:     "/owner/repo.git/info/refs",
			query:    "service=git-upload-pack",
			wantPath: "/owner/repo.git/info/refs",
			wantHost: "github.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "https://proxy"+tt.path, nil)
			req.URL.RawQuery = tt.query
			director(req)
			if req.URL.Path != tt.wantPath {
				t.Errorf("Path = %q, want %q", req.URL.Path, tt.wantPath)
			}
			if req.Host != tt.wantHost {
				t.Errorf("Host = %q, want %q", req.Host, tt.wantHost)
			}
		})
	}
}

// TestIsGitRequest tests that isGitRequest correctly identifies git HTTP
// protocol requests while leaving API requests alone.
func TestIsGitRequest(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		query string
		want  bool
	}{
		{"git-upload-pack", "/owner/repo.git/git-upload-pack", "", true},
		{"git-receive-pack", "/owner/repo.git/git-receive-pack", "", true},
		{"info/refs with service", "/owner/repo.git/info/refs", "service=git-upload-pack", true},
		{"info/refs without service", "/owner/repo.git/info/refs", "", false},
		{"API repo path", "/repos/owner/repo", "", false},
		{"API issues path", "/repos/owner/repo/issues", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "https://proxy"+tt.path, nil)
			req.URL.RawQuery = tt.query
			if got := isGitRequest(req); got != tt.want {
				t.Errorf("isGitRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestResolvedDefaults tests the resolved default values for helper functions.
func TestResolvedDefaults(t *testing.T) {
	if got := resolvedAPIURL(""); got != "https://api.github.com" {
		t.Errorf("resolvedAPIURL(\"\") = %q, want %q", got, "https://api.github.com")
	}
	if got := resolvedAPIURL("https://github.example.com/api/v3"); got != "https://github.example.com/api/v3" {
		t.Errorf("resolvedAPIURL() = %q, want %q", got, "https://github.example.com/api/v3")
	}

	if got := resolvedGitURL(""); got != "https://github.com" {
		t.Errorf("resolvedGitURL(\"\") = %q, want %q", got, "https://github.com")
	}
	if got := resolvedGitURL("https://api.github.com"); got != "https://github.com" {
		t.Errorf("resolvedGitURL(\"https://api.github.com\") = %q, want %q", got, "https://github.com")
	}
	if got := resolvedGitURL("https://github.example.com/api/v3"); got != "https://github.example.com" {
		t.Errorf("resolvedGitURL() = %q, want %q", got, "https://github.example.com")
	}

	if got := resolvedCACertPath(""); got != "/opt/dcx/gh-proxy/ca.crt" {
		t.Errorf("resolvedCACertPath(\"\") = %q, want %q", got, "/opt/dcx/gh-proxy/ca.crt")
	}
	if got := resolvedCACertPath("/custom/ca.crt"); got != "/custom/ca.crt" {
		t.Errorf("resolvedCACertPath() = %q, want %q", got, "/custom/ca.crt")
	}

	if got := resolvedCertExpiry(0); got != 24*time.Hour {
		t.Errorf("resolvedCertExpiry(0) = %v, want %v", got, 24*time.Hour)
	}
	if got := resolvedCertExpiry(1 * time.Hour); got != 1*time.Hour {
		t.Errorf("resolvedCertExpiry() = %v, want %v", got, 1*time.Hour)
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
// constructs the URL using the proxy service's listener address and sets the
// Host header to ProxyHost so the proxy's director can rewrite it correctly.
func makeProxyRequest(client *http.Client, svc *proxy.Service, path string) (*http.Response, error) {
	addr := svc.ListenerAddr()
	if addr == "" {
		return nil, fmt.Errorf("proxy has no listener address")
	}

	reqURL := fmt.Sprintf("https://%s%s", addr, path)
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %v", err)
	}
	req.Host = proxy.ProxyHost

	return client.Do(req)
}
