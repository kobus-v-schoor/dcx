package ghproxy

import (
	"strings"
	"testing"
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

func TestScopingTransportCheckRepoScope(t *testing.T) {
	tests := []struct {
		name       string
		repository string
		path       string
		wantErr    bool
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
			name:       "non-repo path allowed",
			repository: "owner/repo",
			path:       "/user",
			wantErr:    false,
		},
		{
			name:       "graphql allowed",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := &scopingTransport{repository: tt.repository, token: "test-token"}
			err := tr.checkRepoScope(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkRepoScope(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), tt.repository) {
				t.Errorf("error should mention allowed repository %q, got: %s", tt.repository, err.Error())
			}
		})
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
