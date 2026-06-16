package features

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hashicorp/go-extract"
	"github.com/kobus-v-schoor/dcx/internal/devcontainer/spec"
)

// FeatureMeta is the parsed devcontainer-feature.json.
type FeatureMeta struct {
	ID                   string                   `json:"id"`
	Version              string                   `json:"version"`
	Name                 string                   `json:"name"`
	Description          string                   `json:"description"`      // Currently not supported.
	DocumentationURL     string                   `json:"documentationURL"` // Currently not supported.
	LicenseURL           string                   `json:"licenseURL"`       // Currently not supported.
	Keywords             []string                 `json:"keywords"`         // Currently not supported.
	Options              map[string]FeatureOption `json:"options"`
	ContainerEnv         map[string]string        `json:"containerEnv"`
	Privileged           bool                     `json:"privileged"`
	Init                 bool                     `json:"init"`
	CapAdd               []string                 `json:"capAdd"`
	SecurityOpt          []string                 `json:"securityOpt"`
	Entrypoint           string                   `json:"entrypoint"`
	Mounts               json.RawMessage          `json:"mounts"`
	DependsOn            map[string]interface{}   `json:"dependsOn"`
	InstallsAfter        []string                 `json:"installsAfter"`
	LegacyIds            []string                 `json:"legacyIds"`      // Currently not supported.
	Deprecated           bool                     `json:"deprecated"`     // Currently not supported.
	Customizations       json.RawMessage          `json:"customizations"` // Currently not supported.
	OnCreateCommand      spec.LifecycleCommand    `json:"onCreateCommand,omitempty"`
	UpdateContentCommand spec.LifecycleCommand    `json:"updateContentCommand,omitempty"`
	PostCreateCommand    spec.LifecycleCommand    `json:"postCreateCommand,omitempty"`
	PostStartCommand     spec.LifecycleCommand    `json:"postStartCommand,omitempty"`
	PostAttachCommand    spec.LifecycleCommand    `json:"postAttachCommand,omitempty"`
}

// FeatureOption describes a single option in devcontainer-feature.json.
type FeatureOption struct {
	Type        string      `json:"type"`
	Description string      `json:"description"`
	Proposals   []string    `json:"proposals"`
	Enum        []string    `json:"enum"`
	Default     interface{} `json:"default"`
}

// ResolvedFeature is a feature that has been downloaded and parsed.
type ResolvedFeature struct {
	Ref    FeatureRef
	Path   string      // local path to extracted feature directory
	Meta   FeatureMeta // parsed devcontainer-feature.json
	Digest string      // manifest digest, for cache invalidation
}

// Resolve downloads (or reads from cache) the feature and returns its
// extracted directory plus parsed metadata. For OCI features it uses the
// OCI registry client; for local features it resolves relative to the
// workspace folder; for direct tarball features it downloads and extracts.
// If lockfile is non-nil and contains an entry for the feature, the
// resolved digest from the lockfile is used to pin the feature.
func Resolve(ctx context.Context, ref *FeatureRef, workspaceFolder string, lockfile *Lockfile) (*ResolvedFeature, error) {
	cacheDir, err := featureCacheDir()
	if err != nil {
		return nil, fmt.Errorf("resolving feature cache dir: %w", err)
	}

	var path, digest string
	switch ref.Source {
	case SourceOCI:
		client := &ociClient{}
		path, digest, err = client.fetchFeature(ctx, ref, cacheDir, lockfile)
		if err != nil {
			return nil, err
		}
	case SourceLocal:
		path = filepath.Join(workspaceFolder, ".devcontainer", ref.RawID)
		digest = ""
	case SourceDirectTarball:
		path, digest, err = resolveDirectTarball(ctx, ref, cacheDir)
		if err != nil {
			return nil, err
		}
	}

	metaPath := filepath.Join(path, "devcontainer-feature.json")
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", metaPath, err)
	}

	var meta FeatureMeta
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", metaPath, err)
	}

	// Merge default options from metadata with user-provided options.
	mergedOpts := make(map[string]interface{})
	for k, opt := range meta.Options {
		mergedOpts[k] = opt.Default
	}
	for k, v := range ref.Options {
		mergedOpts[k] = v
	}
	ref.Options = mergedOpts

	return &ResolvedFeature{
		Ref:    *ref,
		Path:   path,
		Meta:   meta,
		Digest: digest,
	}, nil
}

// resolveDirectTarball downloads a feature tarball directly and extracts it.
func resolveDirectTarball(ctx context.Context, ref *FeatureRef, cacheDir string) (string, string, error) {
	h := sha256.Sum256([]byte(ref.RawID))
	key := hex.EncodeToString(h[:])
	dest := filepath.Join(cacheDir, "tarball", key)

	if info, err := os.Stat(dest); err == nil && info.IsDir() {
		return dest, "", nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ref.RawID, nil)
	if err != nil {
		return "", "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("downloading tarball: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return "", "", fmt.Errorf("HTTP %d downloading %s", resp.StatusCode, ref.RawID)
	}

	if err := os.MkdirAll(dest, 0755); err != nil {
		return "", "", err
	}

	cfg := extract.NewConfig(extract.WithCreateDestination(true))
	if err := extract.Unpack(ctx, dest, resp.Body, cfg); err != nil {
		return "", "", fmt.Errorf("extracting tarball: %w", err)
	}

	return dest, "", nil
}

// sanitizeOptionName transforms a feature option name into a shell-safe
// variable name by replacing non-alphanumeric/underscore characters with
// underscores, uppercasing, and ensuring it does not start with a digit.
func sanitizeOptionName(name string) string {
	re := regexp.MustCompile(`[^\w_]`)
	s := re.ReplaceAllString(name, "_")
	re2 := regexp.MustCompile(`^[\d_]+`)
	s = re2.ReplaceAllString(s, "")
	if s == "" {
		s = "_"
	}
	return strings.ToUpper(s)
}

// writeFeatureEnvFile writes the merged options as a shell source-able env
// file. Each line is SANITIZED_NAME=JSON-encoded-value so strings are properly
// quoted for bash `source`.
func writeFeatureEnvFile(path string, opts map[string]interface{}) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating env file %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	for k, v := range opts {
		name := sanitizeOptionName(k)
		valBytes, err := json.Marshal(v)
		if err != nil {
			continue
		}
		_, _ = fmt.Fprintf(f, "%s=%s\n", name, string(valBytes))
	}
	return nil
}

// writeInstallWrapper writes a small bash wrapper that sources the env file
// and then executes install.sh.
func writeInstallWrapper(featureDir string) error {
	wrapperPath := filepath.Join(featureDir, "devcontainer-features-install.sh")
	content := `#!/bin/bash
set -e
cd "$(dirname "$0")"
source devcontainer-features.env 2>/dev/null || true
exec ./install.sh
`
	if err := os.WriteFile(wrapperPath, []byte(content), 0755); err != nil {
		return fmt.Errorf("writing wrapper %s: %w", wrapperPath, err)
	}
	return nil
}

// featureCacheDir returns the directory where feature tarballs are cached.
func featureCacheDir() (string, error) {
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Join(xdg, "dcx", "features"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "dcx", "features"), nil
}
