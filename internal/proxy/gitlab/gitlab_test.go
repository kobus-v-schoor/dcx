package gitlab

import (
	"net/http"
	"testing"

	"github.com/kobus-v-schoor/dcx/internal/config"
)

// TestProviderName tests that the provider returns the correct name.
func TestProviderName(t *testing.T) {
	p := &gitlabProvider{}
	if got := p.Name(); got != "gitlab" {
		t.Errorf("Name() = %q, want %q", got, "gitlab")
	}
}

// TestProviderEnabled tests that the provider reports enabled based on config.
func TestProviderEnabled(t *testing.T) {
	p := &gitlabProvider{}

	cfg := &config.Config{Proxy: config.ProxyConfig{GitLab: config.GitLabProxyConfig{Enabled: true}}}
	if !p.Enabled(cfg) {
		t.Error("Enabled() = false, want true when proxy.gitlab.enabled is true")
	}

	cfg.Proxy.GitLab.Enabled = false
	if p.Enabled(cfg) {
		t.Error("Enabled() = true, want false when proxy.gitlab.enabled is false")
	}
}

// TestProviderDomainsDefaults tests that Domains returns the default public
// GitLab domains when none are configured.
func TestProviderDomainsDefaults(t *testing.T) {
	p := &gitlabProvider{}
	cfg := &config.Config{}

	domains := p.Domains(cfg)
	if len(domains) == 0 {
		t.Fatal("Domains() returned empty slice, want default domains")
	}

	expected := map[string]bool{
		"gitlab.com":          true,
		"registry.gitlab.com": true,
	}
	for _, d := range domains {
		if !expected[d] {
			t.Errorf("unexpected default domain %q", d)
		}
		delete(expected, d)
	}
	if len(expected) > 0 {
		for d := range expected {
			t.Errorf("missing expected default domain %q", d)
		}
	}
}

// TestProviderDomainsCustom tests that custom domains from config are used.
func TestProviderDomainsCustom(t *testing.T) {
	p := &gitlabProvider{}
	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			GitLab: config.GitLabProxyConfig{
				Domains: []string{"gitlab.example.com", "registry.gitlab.example.com"},
			},
		},
	}

	domains := p.Domains(cfg)
	if len(domains) != 2 {
		t.Errorf("Domains() returned %d domains, want 2", len(domains))
	}
	if domains[0] != "gitlab.example.com" {
		t.Errorf("Domains()[0] = %q, want %q", domains[0], "gitlab.example.com")
	}
	if domains[1] != "registry.gitlab.example.com" {
		t.Errorf("Domains()[1] = %q, want %q", domains[1], "registry.gitlab.example.com")
	}
}

// TestProviderPrepareRequest tests that PrepareRequest injects the
// Authorization header using Bearer auth.
func TestProviderPrepareRequest(t *testing.T) {
	t.Setenv("GITLAB_TOKEN", "test-token-123")
	p := &gitlabProvider{}
	cfg := &config.Config{}

	req, _ := http.NewRequest("GET", "https://gitlab.com/api/v4/user", nil)
	if err := p.PrepareRequest(req, cfg); err != nil {
		t.Fatalf("PrepareRequest() error: %v", err)
	}

	expectedAuth := "Bearer test-token-123"
	auth := req.Header.Get("Authorization")
	if auth != expectedAuth {
		t.Errorf("Authorization header = %q, want %q", auth, expectedAuth)
	}
}

// TestProviderPrepareRequestReplacesExistingAuth tests that PrepareRequest
// replaces any existing Authorization header.
func TestProviderPrepareRequestReplacesExistingAuth(t *testing.T) {
	t.Setenv("GITLAB_TOKEN", "real-token")
	p := &gitlabProvider{}
	cfg := &config.Config{}

	req, _ := http.NewRequest("GET", "https://gitlab.com/api/v4/user", nil)
	req.Header.Set("Authorization", "Bearer fake-token")
	if err := p.PrepareRequest(req, cfg); err != nil {
		t.Fatalf("PrepareRequest() error: %v", err)
	}

	expectedAuth := "Bearer real-token"
	auth := req.Header.Get("Authorization")
	if auth != expectedAuth {
		t.Errorf("Authorization header = %q, want %q", auth, expectedAuth)
	}
}

// TestProviderPrepareRequestNoToken tests that PrepareRequest returns an
// error when no token is available.
func TestProviderPrepareRequestNoToken(t *testing.T) {
	t.Setenv("GITLAB_TOKEN", "")
	t.Setenv("GLAB_TOKEN", "")
	// Point HOME to a temp dir so glab cannot find any stored token.
	t.Setenv("HOME", t.TempDir())
	p := &gitlabProvider{}
	cfg := &config.Config{}

	req, _ := http.NewRequest("GET", "https://gitlab.com/api/v4/user", nil)
	if err := p.PrepareRequest(req, cfg); err == nil {
		t.Error("PrepareRequest() = nil, want error when no token available")
	}
}

// TestProviderEnvVars tests that EnvVars returns GITLAB_TOKEN=dummy and
// GLAB_TOKEN=dummy.
func TestProviderEnvVars(t *testing.T) {
	p := &gitlabProvider{}
	cfg := &config.Config{}

	vars := p.EnvVars(cfg)
	if len(vars) != 2 {
		t.Fatalf("EnvVars() returned %d vars, want 2", len(vars))
	}
	if vars[0] != "GITLAB_TOKEN=dummy" {
		t.Errorf("EnvVars()[0] = %q, want %q", vars[0], "GITLAB_TOKEN=dummy")
	}
	if vars[1] != "GLAB_TOKEN=dummy" {
		t.Errorf("EnvVars()[1] = %q, want %q", vars[1], "GLAB_TOKEN=dummy")
	}
}

// TestDetectTokenEnvVar tests token detection from environment variables.
func TestDetectTokenEnvVar(t *testing.T) {
	// GITLAB_TOKEN takes precedence.
	t.Setenv("GITLAB_TOKEN", "gitlab-token")
	t.Setenv("GLAB_TOKEN", "glab-token")
	if token, ok := DetectToken(); !ok || token != "gitlab-token" {
		t.Errorf("DetectToken() = %q, %v, want %q, true", token, ok, "gitlab-token")
	}

	// GLAB_TOKEN fallback.
	t.Setenv("GITLAB_TOKEN", "")
	if token, ok := DetectToken(); !ok || token != "glab-token" {
		t.Errorf("DetectToken() = %q, %v, want %q, true", token, ok, "glab-token")
	}

	// No token. Point HOME to a temp dir so glab cannot discover a stored
	// token on the host.
	t.Setenv("GLAB_TOKEN", "")
	t.Setenv("HOME", t.TempDir())
	if _, ok := DetectToken(); ok {
		t.Error("DetectToken() = true, want false when no token available")
	}
}
