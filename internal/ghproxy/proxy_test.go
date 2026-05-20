package ghproxy

import (
	"strings"
	"testing"
	"time"
)

func TestExtractRepo(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		want   string
		wantOK bool
	}{
		{
			name:   "repos owner repo issues",
			path:   "/repos/owner/repo/issues",
			want:   "owner/repo",
			wantOK: true,
		},
		{
			name:   "repos owner repo",
			path:   "/repos/owner/repo",
			want:   "owner/repo",
			wantOK: true,
		},
		{
			name:   "repos owner repo pull requests",
			path:   "/repos/owner/repo/pulls/123",
			want:   "owner/repo",
			wantOK: true,
		},
		{
			name:   "repos owner repo contents",
			path:   "/repos/owner/repo/contents/README.md",
			want:   "owner/repo",
			wantOK: true,
		},
		{
			name:   "repos owner repo git refs",
			path:   "/repos/owner/repo/git/refs/heads/main",
			want:   "owner/repo",
			wantOK: true,
		},
		{
			name:   "user endpoint no repo",
			path:   "/user",
			want:   "",
			wantOK: false,
		},
		{
			name:   "app endpoint no repo",
			path:   "/app",
			want:   "",
			wantOK: false,
		},
		{
			name:   "graphql endpoint no repo",
			path:   "/graphql",
			want:   "",
			wantOK: false,
		},
		{
			name:   "root path",
			path:   "/",
			want:   "",
			wantOK: false,
		},
		{
			name:   "repos only one segment after",
			path:   "/repos/owner",
			want:   "",
			wantOK: false,
		},
		{
			name:   "empty path",
			path:   "",
			want:   "",
			wantOK: false,
		},
		{
			name:   "repos with hyphenated names",
			path:   "/repos/my-org/my-repo/actions/runs",
			want:   "my-org/my-repo",
			wantOK: true,
		},
		{
			name:   "repos with dots",
			path:   "/repos/owner/repo.js/pulls",
			want:   "owner/repo.js",
			wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := extractRepo(tt.path)
			if ok != tt.wantOK {
				t.Errorf("extractRepo(%q) ok = %v, want %v", tt.path, ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("extractRepo(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestScopingTransportCheckScope(t *testing.T) {
	tests := []struct {
		name         string
		repository   string
		allowedPaths []string
		path         string
		wantErr      bool
	}{
		{
			name:       "matching repository",
			repository: "owner/repo",
			path:       "/repos/owner/repo/issues",
			wantErr:    false,
		},
		{
			name:       "non-matching repository",
			repository: "owner/repo",
			path:       "/repos/other/repo/issues",
			wantErr:    true,
		},
		{
			name:       "non-repo path allowed by default",
			repository: "owner/repo",
			path:       "/user",
			wantErr:    false,
		},
		{
			name:       "graphql allowed by default",
			repository: "owner/repo",
			path:       "/graphql",
			wantErr:    false,
		},
		{
			name:       "matching with hyphens",
			repository: "my-org/my-repo",
			path:       "/repos/my-org/my-repo/pulls",
			wantErr:    false,
		},
		{
			name:       "different org same repo name",
			repository: "owner/repo",
			path:       "/repos/other-org/repo",
			wantErr:    true,
		},
		// Allowed paths tests.
		{
			name:         "non-repo path matching allowed path",
			repository:   "owner/repo",
			allowedPaths: []string{"/user", "/graphql"},
			path:         "/user",
			wantErr:      false,
		},
		{
			name:         "non-repo path not in allowed paths",
			repository:   "owner/repo",
			allowedPaths: []string{"/user", "/graphql"},
			path:         "/orgs/some-org",
			wantErr:      true,
		},
		{
			name:         "graphql in allowed paths",
			repository:   "owner/repo",
			allowedPaths: []string{"/user", "/graphql"},
			path:         "/graphql",
			wantErr:      false,
		},
		{
			name:         "sub-path of allowed path",
			repository:   "owner/repo",
			allowedPaths: []string{"/user"},
			path:         "/user/emails",
			wantErr:      false,
		},
		{
			name:         "repo path always allowed with allowed paths",
			repository:   "owner/repo",
			allowedPaths: []string{"/user"},
			path:         "/repos/owner/repo/issues",
			wantErr:      false,
		},
		{
			name:         "empty allowed paths allows all non-repo",
			repository:   "owner/repo",
			allowedPaths: []string{},
			path:         "/orgs/some-org",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := &scopingTransport{repository: tt.repository, token: "test-token", allowedPaths: tt.allowedPaths}
			err := tr.checkScope(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkScope(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), tt.repository) && len(tt.allowedPaths) == 0 {
				t.Errorf("error should mention allowed repository %q, got: %s", tt.repository, err.Error())
			}
		})
	}
}

func TestOptionsCABundlePath(t *testing.T) {
	tests := []struct {
		name       string
		caCertPath string
		want       string
	}{
		{
			name:       "default path",
			caCertPath: "",
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
	opts := Options{GatewayIP: "172.17.0.1"}

	if got := opts.caCertPath(); got != DefaultCACertPath {
		t.Errorf("caCertPath() = %q, want %q", got, DefaultCACertPath)
	}
	if got := opts.apiURL(); got != DefaultAPIURL {
		t.Errorf("apiURL() = %q, want %q", got, DefaultAPIURL)
	}
	if got := opts.certExpiry(); got != DefaultCertExpiry {
		t.Errorf("certExpiry() = %v, want %v", got, DefaultCertExpiry)
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

func TestParseGitRemoteURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{
			name: "https URL",
			url:  "https://github.com/owner/repo.git",
			want: "owner/repo",
		},
		{
			name: "https URL without .git",
			url:  "https://github.com/owner/repo",
			want: "owner/repo",
		},
		{
			name: "SSH URL",
			url:  "git@github.com:owner/repo.git",
			want: "owner/repo",
		},
		{
			name: "SSH URL without .git",
			url:  "git@github.com:owner/repo",
			want: "owner/repo",
		},
		{
			name: "SSH URL with scheme",
			url:  "ssh://git@github.com/owner/repo.git",
			want: "owner/repo",
		},
		{
			name: "hyphenated names",
			url:  "https://github.com/my-org/my-repo.git",
			want: "my-org/my-repo",
		},
		{
			name: "SSH hyphenated names",
			url:  "git@github.com:my-org/my-repo.git",
			want: "my-org/my-repo",
		},
		{
			name:    "invalid format",
			url:     "something-random",
			wantErr: true,
		},
		{
			name:    "HTTPS too short",
			url:     "https://github.com/owner",
			wantErr: true,
		},
		{
			name:    "SSH too short",
			url:     "git@github.com:owner",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseGitRemoteURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseGitRemoteURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseGitRemoteURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}
