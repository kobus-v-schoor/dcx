package github

import (
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/kobus-v-schoor/dcx/internal/config"
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

// TestProviderDomainsDefaults tests that Domains returns the default public
// GitHub domains when none are configured.
func TestProviderDomainsDefaults(t *testing.T) {
	p := &githubProvider{}
	cfg := &config.Config{}

	domains := p.Domains(cfg)
	if len(domains) == 0 {
		t.Fatal("Domains() returned empty slice, want default domains")
	}

	expected := map[string]bool{
		"github.com":                true,
		"api.github.com":            true,
		"uploads.github.com":        true,
		"raw.githubusercontent.com": true,
		"gist.github.com":           true,
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
	p := &githubProvider{}
	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			GitHub: config.GitHubProxyConfig{
				Domains: []string{"github.example.com", "api.github.example.com"},
			},
		},
	}

	domains := p.Domains(cfg)
	if len(domains) != 2 {
		t.Errorf("Domains() returned %d domains, want 2", len(domains))
	}
	if domains[0] != "github.example.com" {
		t.Errorf("Domains()[0] = %q, want %q", domains[0], "github.example.com")
	}
	if domains[1] != "api.github.example.com" {
		t.Errorf("Domains()[1] = %q, want %q", domains[1], "api.github.example.com")
	}
}

// TestProviderPrepareRequest tests that PrepareRequest injects the
// Authorization header using basic auth.
func TestProviderPrepareRequest(t *testing.T) {
	t.Setenv("GH_TOKEN", "test-token-123")
	p := &githubProvider{}
	cfg := &config.Config{}

	req, _ := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err := p.PrepareRequest(req, cfg); err != nil {
		t.Fatalf("PrepareRequest() error: %v", err)
	}

	expectedAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("git:test-token-123"))
	auth := req.Header.Get("Authorization")
	if auth != expectedAuth {
		t.Errorf("Authorization header = %q, want %q", auth, expectedAuth)
	}
}

// TestProviderPrepareRequestReplacesExistingAuth tests that PrepareRequest
// replaces any existing Authorization header.
func TestProviderPrepareRequestReplacesExistingAuth(t *testing.T) {
	t.Setenv("GH_TOKEN", "real-token")
	p := &githubProvider{}
	cfg := &config.Config{}

	req, _ := http.NewRequest("GET", "https://api.github.com/user", nil)
	req.Header.Set("Authorization", "Bearer fake-token")
	if err := p.PrepareRequest(req, cfg); err != nil {
		t.Fatalf("PrepareRequest() error: %v", err)
	}

	expectedAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("git:real-token"))
	auth := req.Header.Get("Authorization")
	if auth != expectedAuth {
		t.Errorf("Authorization header = %q, want %q", auth, expectedAuth)
	}
}

// TestProviderPrepareRequestNoToken tests that PrepareRequest returns an
// error when no token is available.
func TestProviderPrepareRequestNoToken(t *testing.T) {
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	p := &githubProvider{}
	cfg := &config.Config{}

	req, _ := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err := p.PrepareRequest(req, cfg); err == nil {
		t.Error("PrepareRequest() = nil, want error when no token available")
	}
}

// TestProviderEnvVars tests that EnvVars returns GH_TOKEN=dummy.
func TestProviderEnvVars(t *testing.T) {
	p := &githubProvider{}
	cfg := &config.Config{}

	vars := p.EnvVars(cfg)
	if len(vars) != 1 || vars[0] != "GH_TOKEN=dummy" {
		t.Errorf("EnvVars() = %v, want [GH_TOKEN=dummy]", vars)
	}
}

// TestDetectTokenEnvVar tests token detection from environment variables.
func TestDetectTokenEnvVar(t *testing.T) {
	// GH_TOKEN takes precedence.
	t.Setenv("GH_TOKEN", "gh-token")
	t.Setenv("GITHUB_TOKEN", "github-token")
	if token, ok := DetectToken(); !ok || token != "gh-token" {
		t.Errorf("DetectToken() = %q, %v, want %q, true", token, ok, "gh-token")
	}

	// GITHUB_TOKEN fallback.
	t.Setenv("GH_TOKEN", "")
	if token, ok := DetectToken(); !ok || token != "github-token" {
		t.Errorf("DetectToken() = %q, %v, want %q, true", token, ok, "github-token")
	}

	// No token.
	t.Setenv("GITHUB_TOKEN", "")
	if _, ok := DetectToken(); ok {
		t.Error("DetectToken() = true, want false when no token available")
	}
}
