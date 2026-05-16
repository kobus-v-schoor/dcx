package features

import (
	"encoding/json"
	"testing"

	"github.com/kobus-v-schoor/dcx/internal/config"
)

func TestBuildJSONEmpty(t *testing.T) {
	got, err := BuildJSON(nil)
	if err != nil {
		t.Fatalf("BuildJSON(nil) error: %v", err)
	}
	if got != "" {
		t.Errorf("BuildJSON(nil) = %q, want empty string", got)
	}

	got, err = BuildJSON([]config.Feature{})
	if err != nil {
		t.Fatalf("BuildJSON([]) error: %v", err)
	}
	if got != "" {
		t.Errorf("BuildJSON([]) = %q, want empty string", got)
	}
}

func TestBuildJSONSingleFeatureWithOptions(t *testing.T) {
	features := []config.Feature{
		{
			ID: "ghcr.io/devcontainers/features/github-cli:1",
			Options: map[string]interface{}{
				"version": "latest",
			},
		},
	}

	got, err := BuildJSON(features)
	if err != nil {
		t.Fatalf("BuildJSON() error: %v", err)
	}

	result := map[string]map[string]interface{}{}
	if err := json.Unmarshal([]byte(got), &result); err != nil {
		t.Fatalf("parsing result JSON: %v", err)
	}

	opts, ok := result["ghcr.io/devcontainers/features/github-cli:1"]
	if !ok {
		t.Fatal("expected feature ID in result")
	}
	if opts["version"] != "latest" {
		t.Errorf("version = %v, want latest", opts["version"])
	}
}

func TestBuildJSONEmptyOptions(t *testing.T) {
	features := []config.Feature{
		{
			ID:      "ghcr.io/opencode/devcontainer-feature/opencode",
			Options: nil,
		},
	}

	got, err := BuildJSON(features)
	if err != nil {
		t.Fatalf("BuildJSON() error: %v", err)
	}

	result := map[string]map[string]interface{}{}
	if err := json.Unmarshal([]byte(got), &result); err != nil {
		t.Fatalf("parsing result JSON: %v", err)
	}

	key := "ghcr.io/opencode/devcontainer-feature/opencode:latest"
	opts, ok := result[key]
	if !ok {
		t.Fatalf("expected feature ID %q in result, got keys: %v", key, mapKeys(result))
	}
	if len(opts) != 0 {
		t.Errorf("options = %v, want empty map {}", opts)
	}

	raw := string(got)
	if findStr(raw, ":null") {
		t.Errorf("JSON should not contain null, got: %s", raw)
	}
}

func TestBuildJSONExplicitEmptyOptions(t *testing.T) {
	features := []config.Feature{
		{
			ID:      "ghcr.io/devcontainers/features/docker-in-docker:2",
			Options: map[string]interface{}{},
		},
	}

	got, err := BuildJSON(features)
	if err != nil {
		t.Fatalf("BuildJSON() error: %v", err)
	}

	result := map[string]map[string]interface{}{}
	if err := json.Unmarshal([]byte(got), &result); err != nil {
		t.Fatalf("parsing result JSON: %v", err)
	}

	opts := result["ghcr.io/devcontainers/features/docker-in-docker:2"]
	if len(opts) != 0 {
		t.Errorf("options = %v, want empty map {}", opts)
	}
}

func TestBuildJSONLatestTagAppended(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want string
	}{
		{
			name: "no tag gets :latest",
			id:   "ghcr.io/devcontainers/features/github-cli",
			want: "ghcr.io/devcontainers/features/github-cli:latest",
		},
		{
			name: "explicit version tag preserved",
			id:   "ghcr.io/devcontainers/features/github-cli:1",
			want: "ghcr.io/devcontainers/features/github-cli:1",
		},
		{
			name: "explicit latest tag preserved",
			id:   "ghcr.io/devcontainers/features/github-cli:latest",
			want: "ghcr.io/devcontainers/features/github-cli:latest",
		},
		{
			name: "registry port in URL not treated as version tag",
			id:   "ghcr.io:443/devcontainers/features/github-cli",
			want: "ghcr.io:443/devcontainers/features/github-cli:latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			features := []config.Feature{
				{ID: tt.id, Options: map[string]interface{}{}},
			}

			got, err := BuildJSON(features)
			if err != nil {
				t.Fatalf("BuildJSON() error: %v", err)
			}

			result := map[string]map[string]interface{}{}
			if err := json.Unmarshal([]byte(got), &result); err != nil {
				t.Fatalf("parsing result JSON: %v", err)
			}

			if _, ok := result[tt.want]; !ok {
				t.Errorf("expected key %q in result, got keys: %v", tt.want, mapKeys(result))
			}
		})
	}
}

func TestBuildJSONMultipleFeatures(t *testing.T) {
	features := []config.Feature{
		{
			ID: "ghcr.io/devcontainers/features/github-cli:1",
			Options: map[string]interface{}{
				"version": "2",
			},
		},
		{
			ID:      "ghcr.io/devcontainers/features/docker-in-docker:2",
			Options: map[string]interface{}{},
		},
		{
			ID:      "ghcr.io/opencode/devcontainer-feature/opencode",
			Options: nil,
		},
	}

	got, err := BuildJSON(features)
	if err != nil {
		t.Fatalf("BuildJSON() error: %v", err)
	}

	result := map[string]map[string]interface{}{}
	if err := json.Unmarshal([]byte(got), &result); err != nil {
		t.Fatalf("parsing result JSON: %v", err)
	}

	if len(result) != 3 {
		t.Errorf("expected 3 features, got %d", len(result))
	}

	expectedKeys := []string{
		"ghcr.io/devcontainers/features/github-cli:1",
		"ghcr.io/devcontainers/features/docker-in-docker:2",
		"ghcr.io/opencode/devcontainer-feature/opencode:latest",
	}
	for _, k := range expectedKeys {
		if _, ok := result[k]; !ok {
			t.Errorf("expected key %q in result", k)
		}
	}
}

func mapKeys[M ~map[K]V, K comparable, V any](m M) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func findStr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
