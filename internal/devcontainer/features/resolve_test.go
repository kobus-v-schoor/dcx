package features

import (
	"encoding/json"
	"testing"
)

func TestParseOCIFeature(t *testing.T) {
	tests := []struct {
		name          string
		rawID         string
		rawOpts       string
		wantRegistry  string
		wantNamespace string
		wantID        string
		wantVersion   string
		wantSource    SourceType
		wantOptionLen int
	}{
		{
			name:          "github-cli with version in key",
			rawID:         "ghcr.io/devcontainers/features/github-cli:1",
			rawOpts:       "{}",
			wantRegistry:  "ghcr.io",
			wantNamespace: "devcontainers/features",
			wantID:        "github-cli",
			wantVersion:   "1",
			wantSource:    SourceOCI,
			wantOptionLen: 0,
		},
		{
			name:          "github-cli without version",
			rawID:         "ghcr.io/devcontainers/features/github-cli",
			rawOpts:       "\"1\"",
			wantRegistry:  "ghcr.io",
			wantNamespace: "devcontainers/features",
			wantID:        "github-cli",
			wantVersion:   "1",
			wantSource:    SourceOCI,
			wantOptionLen: 0,
		},
		{
			name:          "feature with options",
			rawID:         "ghcr.io/devcontainers/features/github-cli:1",
			rawOpts:       `{"version":"latest","foo":"bar"}`,
			wantRegistry:  "ghcr.io",
			wantNamespace: "devcontainers/features",
			wantID:        "github-cli",
			wantVersion:   "1",
			wantSource:    SourceOCI,
			wantOptionLen: 2,
		},
		{
			name:          "local feature",
			rawID:         "./myFeature",
			rawOpts:       "null",
			wantRegistry:  "",
			wantNamespace: "",
			wantID:        "",
			wantVersion:   "latest",
			wantSource:    SourceLocal,
			wantOptionLen: 0,
		},
		{
			name:          "direct tarball",
			rawID:         "https://example.com/feature.tgz",
			rawOpts:       "null",
			wantRegistry:  "",
			wantNamespace: "",
			wantID:        "",
			wantVersion:   "",
			wantSource:    SourceDirectTarball,
			wantOptionLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := Parse(tt.rawID, json.RawMessage(tt.rawOpts))
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			if ref.Registry != tt.wantRegistry {
				t.Errorf("registry = %q, want %q", ref.Registry, tt.wantRegistry)
			}
			if ref.Namespace != tt.wantNamespace {
				t.Errorf("namespace = %q, want %q", ref.Namespace, tt.wantNamespace)
			}
			if ref.ID != tt.wantID {
				t.Errorf("id = %q, want %q", ref.ID, tt.wantID)
			}
			if ref.Version != tt.wantVersion {
				t.Errorf("version = %q, want %q", ref.Version, tt.wantVersion)
			}
			if ref.Source != tt.wantSource {
				t.Errorf("source = %d, want %d", ref.Source, tt.wantSource)
			}
			if len(ref.Options) != tt.wantOptionLen {
				t.Errorf("options len = %d, want %d", len(ref.Options), tt.wantOptionLen)
			}
		})
	}
}

func TestParseEmptyOptions(t *testing.T) {
	ref, err := Parse("ghcr.io/devcontainers/features/github-cli:1", nil)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(ref.Options) != 0 {
		t.Errorf("expected empty options, got %v", ref.Options)
	}
	if ref.Version != "1" {
		t.Errorf("version = %q, want 1", ref.Version)
	}
}

func TestParseNullOptions(t *testing.T) {
	ref, err := Parse("ghcr.io/devcontainers/features/github-cli:1", json.RawMessage("null"))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(ref.Options) != 0 {
		t.Errorf("expected empty options for null, got %v", ref.Options)
	}
}

func TestSanitizeOptionName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"version", "VERSION"},
		{"install-from", "INSTALL_FROM"},
		{"123abc", "ABC"},
		{"_leading_underscore", "LEADING_UNDERSCORE"},
		{"special!@#chars", "SPECIAL___CHARS"},
	}
	for _, tt := range tests {
		got := sanitizeOptionName(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeOptionName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFeatureRefString(t *testing.T) {
	ref := FeatureRef{
		Registry:  "ghcr.io",
		Namespace: "devcontainers/features",
		ID:        "github-cli",
		Version:   "1",
	}
	if got := ref.String(); got != "ghcr.io/devcontainers/features/github-cli:1" {
		t.Errorf("String() = %q", got)
	}
}
