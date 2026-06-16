package features

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/opencontainers/go-digest"
)

// authChallengeRe matches the Bearer challenge from a 401 response.
var authChallengeRe = regexp.MustCompile(`(?i)Bearer\s+realm="([^"]+)",\s*service="([^"]+)"(?:,\s*scope="([^"]+)")?`)

// tokenResponse represents the JSON from the token endpoint.
type tokenResponse struct {
	Token string `json:"token"`
}

// ociManifest is a minimal structure for the OCI manifest we need.
type ociManifest struct {
	SchemaVersion int    `json:"schemaVersion"`
	MediaType     string `json:"mediaType"`
	Config        struct {
		MediaType string `json:"mediaType"`
		Digest    string `json:"digest"`
		Size      int    `json:"size"`
	} `json:"config"`
	Layers []struct {
		MediaType   string            `json:"mediaType"`
		Digest      string            `json:"digest"`
		Size        int               `json:"size"`
		Annotations map[string]string `json:"annotations"`
	} `json:"layers"`
	Annotations map[string]string `json:"annotations"`
}

// ociClient fetches OCI artifacts from a registry.
type ociClient struct {
	http *http.Client
}

// fetchFeature downloads the feature tarball for an OCI reference.
// It returns the path to the extracted directory and the manifest digest.
// Cache hits are served from disk without network. If lockfile is non-nil
// and contains an entry for the feature, the digest from the lockfile is
// used instead of resolving the version tag.
func (c *ociClient) fetchFeature(ctx context.Context, ref *FeatureRef, cacheDir string, lockfile *Lockfile) (extractedPath string, manifestDigest string, err error) {
	manifestDigest, err = c.resolveManifestDigest(ctx, ref, lockfile)
	if err != nil {
		return "", "", fmt.Errorf("fetching manifest for %s: %w", ref.String(), err)
	}

	extractedPath = filepath.Join(cacheDir, ref.Registry, ref.Namespace, ref.ID, manifestDigest)

	// Cache hit: return immediately if the directory already exists.
	if info, err := os.Stat(extractedPath); err == nil && info.IsDir() {
		return extractedPath, manifestDigest, nil
	}

	manifest, err := c.fetchManifest(ctx, ref)
	if err != nil {
		return "", "", fmt.Errorf("fetching manifest for %s: %w", ref.String(), err)
	}

	if manifest.Config.MediaType != "application/vnd.devcontainers" {
		return "", "", fmt.Errorf("unexpected config mediaType %q for feature %s", manifest.Config.MediaType, ref.String())
	}

	// Find the layer with the devcontainers tarball.
	var layerDigest string
	for _, layer := range manifest.Layers {
		if layer.MediaType == "application/vnd.devcontainers.layer.v1+tar" {
			layerDigest = layer.Digest
			break
		}
	}
	if layerDigest == "" {
		return "", "", fmt.Errorf("no devcontainers layer found in manifest for %s", ref.String())
	}

	if err := c.extractBlob(ctx, ref, layerDigest, extractedPath); err != nil {
		return "", "", fmt.Errorf("extracting feature blob for %s: %w", ref.String(), err)
	}

	return extractedPath, manifestDigest, nil
}

// resolveManifestDigest returns the manifest digest for the feature, using
// the lockfile if available, otherwise fetching it from the registry.
func (c *ociClient) resolveManifestDigest(ctx context.Context, ref *FeatureRef, lockfile *Lockfile) (string, error) {
	if lockfile != nil {
		if digest := lockfile.ResolveDigest(ref.RawID); digest != "" {
			return digest, nil
		}
	}
	return c.fetchManifestDigest(ctx, ref)
}

// fetchManifestDigest performs a HEAD request for the manifest and returns
// the digest from the Docker-Content-Digest header.
func (c *ociClient) fetchManifestDigest(ctx context.Context, ref *FeatureRef) (string, error) {
	url := manifestURL(ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.oci.image.manifest.v1+json")

	resp, err := c.requestWithAuth(ctx, req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	dg := resp.Header.Get("Docker-Content-Digest")
	if dg == "" {
		return "", fmt.Errorf("missing Docker-Content-Digest header for %s", ref.String())
	}
	return dg, nil
}

// fetchManifest downloads and decodes the OCI manifest for the given reference.
func (c *ociClient) fetchManifest(ctx context.Context, ref *FeatureRef) (*ociManifest, error) {
	url := manifestURL(ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.oci.image.manifest.v1+json")

	resp, err := c.requestWithAuth(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading manifest body: %w", err)
	}

	var m ociManifest
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest JSON: %w", err)
	}
	return &m, nil
}

// extractBlob downloads the given blob digest, streams it through gzip and tar,
// and extracts it to destDir.
func (c *ociClient) extractBlob(ctx context.Context, ref *FeatureRef, blobDigest, destDir string) error {
	url := blobURL(ref, blobDigest)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := c.requestWithAuth(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Peek at the first two bytes to decide whether the layer is gzip
	// compressed. We must read from resp.Body ourselves and then wrap
	// the remaining data in a multi-reader so that a failed gzip.NewReader
	// does not consume bytes that the tar reader needs later.
	magic := make([]byte, 2)
	if _, err := io.ReadFull(resp.Body, magic); err != nil {
		return fmt.Errorf("reading blob header: %w", err)
	}
	body := io.MultiReader(bytes.NewReader(magic), resp.Body)

	if magic[0] == 0x1f && magic[1] == 0x8b {
		gzr, err := gzip.NewReader(body)
		if err != nil {
			return fmt.Errorf("creating gzip reader: %w", err)
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
			return fmt.Errorf("reading tar entry: %w", err)
		}

		if strings.Contains(hdr.Name, "..") {
			// Security: skip entries with path traversal.
			continue
		}

		target := filepath.Join(destDir, filepath.Clean(hdr.Name))
		if hdr.FileInfo().IsDir() {
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return fmt.Errorf("creating directory %s: %w", target, err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return fmt.Errorf("creating parent directory for %s: %w", target, err)
		}

		f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
		if err != nil {
			return fmt.Errorf("creating file %s: %w", target, err)
		}
		if _, err := io.Copy(f, tr); err != nil {
			_ = f.Close()
			return fmt.Errorf("writing file %s: %w", target, err)
		}
		_ = f.Close()
	}

	return nil
}

// requestWithAuth makes an HTTP request, handling 401 Bearer token challenges
// automatically. On a 401 response it parses the Www-Authenticate header,
// fetches a Bearer token, and retries the request.
func (c *ociClient) requestWithAuth(ctx context.Context, req *http.Request) (*http.Response, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusUnauthorized {
		if resp.StatusCode >= 400 {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("HTTP %d for %s %s", resp.StatusCode, req.Method, req.URL)
		}
		return resp, nil
	}

	wwwAuth := resp.Header.Get("Www-Authenticate")
	_ = resp.Body.Close()
	if wwwAuth == "" {
		return nil, fmt.Errorf("401 without Www-Authenticate header for %s", req.URL)
	}

	realm, service, scope, ok := parseWwwAuthenticate(wwwAuth)
	if !ok {
		return nil, fmt.Errorf("unparseable Www-Authenticate header: %q", wwwAuth)
	}

	token, err := c.fetchToken(ctx, realm, service, scope)
	if err != nil {
		return nil, fmt.Errorf("fetching auth token: %w", err)
	}

	// Clone the request for the retry.
	retryReq := req.Clone(ctx)
	retryReq.Header.Set("Authorization", "Bearer "+token)

	resp2, err := c.http.Do(retryReq)
	if err != nil {
		return nil, err
	}
	if resp2.StatusCode >= 400 {
		_ = resp2.Body.Close()
		return nil, fmt.Errorf("HTTP %d for %s %s (after auth)", resp2.StatusCode, retryReq.Method, retryReq.URL)
	}
	return resp2, nil
}

// fetchToken requests a Bearer token from the given realm.
func (c *ociClient) fetchToken(ctx context.Context, realm, service, scope string) (string, error) {
	u := realm + "?service=" + service
	if scope != "" {
		u += "&scope=" + scope
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("token endpoint returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading token response: %w", err)
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("parsing token JSON: %w", err)
	}
	return tr.Token, nil
}

// parseWwwAuthenticate parses a Bearer challenge from the Www-Authenticate header.
func parseWwwAuthenticate(header string) (realm, service, scope string, ok bool) {
	m := authChallengeRe.FindStringSubmatch(header)
	if len(m) >= 3 {
		realm = m[1]
		service = m[2]
		if len(m) >= 4 {
			scope = m[3]
		}
		return realm, service, scope, true
	}
	return "", "", "", false
}

// manifestURL returns the registry URL for the manifest of the given feature reference.
func manifestURL(ref *FeatureRef) string {
	repoPath := ref.Namespace + "/" + ref.ID
	return fmt.Sprintf("https://%s/v2/%s/manifests/%s", ref.Registry, repoPath, ref.Version)
}

// blobURL returns the registry URL for a blob digest.
func blobURL(ref *FeatureRef, blobDigest string) string {
	repoPath := ref.Namespace + "/" + ref.ID
	return fmt.Sprintf("https://%s/v2/%s/blobs/%s", ref.Registry, repoPath, blobDigest)
}

// validDigest verifies that the digest string is well-formed.
func validDigest(d string) error {
	_, err := digest.Parse(d)
	return err
}
