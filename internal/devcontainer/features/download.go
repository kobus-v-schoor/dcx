package features

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// FeatureMeta is the parsed devcontainer-feature.json.
type FeatureMeta struct {
	ID               string                   `json:"id"`
	Version          string                   `json:"version"`
	Name             string                   `json:"name"`
	Description      string                   `json:"description"`
	DocumentationURL string                   `json:"documentationURL"`
	LicenseURL       string                   `json:"licenseURL"`
	Keywords         []string                 `json:"keywords"`
	Options          map[string]FeatureOption `json:"options"`
	ContainerEnv     map[string]string        `json:"containerEnv"`
	Privileged       bool                     `json:"privileged"`
	Init             bool                     `json:"init"`
	CapAdd           []string                 `json:"capAdd"`
	SecurityOpt      []string                 `json:"securityOpt"`
	Entrypoint       string                   `json:"entrypoint"`
	Mounts           json.RawMessage          `json:"mounts"`
	DependsOn        map[string]interface{}   `json:"dependsOn"`
	InstallsAfter    []string                 `json:"installsAfter"`
	LegacyIds        []string                 `json:"legacyIds"`
	Deprecated       bool                     `json:"deprecated"`
	Customizations   json.RawMessage          `json:"customizations"`
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
func Resolve(ctx context.Context, ref *FeatureRef, workspaceFolder string) (*ResolvedFeature, error) {
	cacheDir, err := featureCacheDir()
	if err != nil {
		return nil, fmt.Errorf("resolving feature cache dir: %w", err)
	}

	var path, digest string
	switch ref.Source {
	case SourceOCI:
		client := &ociClient{http: http.DefaultClient}
		path, digest, err = client.fetchFeature(ctx, ref, cacheDir)
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
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", "", fmt.Errorf("HTTP %d downloading %s", resp.StatusCode, ref.RawID)
	}

	if err := os.MkdirAll(dest, 0755); err != nil {
		return "", "", err
	}

	// Stream through tar (optionally gzipped). Peek at the first two bytes
	// to detect gzip without consuming data that the tar reader needs.
	magic := make([]byte, 2)
	if _, err := io.ReadFull(resp.Body, magic); err != nil {
		return "", "", fmt.Errorf("reading tarball header: %w", err)
	}
	body := io.MultiReader(bytes.NewReader(magic), resp.Body)

	if magic[0] == 0x1f && magic[1] == 0x8b {
		gzr, err := gzip.NewReader(body)
		if err != nil {
			return "", "", fmt.Errorf("creating gzip reader: %w", err)
		}
		defer gzr.Close()
		body = gzr
	}
	tr := tar.NewReader(body)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", "", fmt.Errorf("reading tar entry: %w", err)
		}

		if strings.Contains(hdr.Name, "..") {
			continue
		}

		target := filepath.Join(dest, filepath.Clean(hdr.Name))
		if hdr.FileInfo().IsDir() {
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return "", "", fmt.Errorf("creating directory %s: %w", target, err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return "", "", fmt.Errorf("creating parent directory for %s: %w", target, err)
		}

		f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
		if err != nil {
			return "", "", fmt.Errorf("creating file %s: %w", target, err)
		}
		if _, err := io.Copy(f, tr); err != nil {
			_ = f.Close()
			return "", "", fmt.Errorf("writing file %s: %w", target, err)
		}
		_ = f.Close()
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
	defer f.Close()

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
