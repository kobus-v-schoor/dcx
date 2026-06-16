package features

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/go-extract"
)

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

// ociClient fetches OCI artifacts from a registry using go-containerregistry.
type ociClient struct{}

// fetchFeature downloads the feature tarball for an OCI reference.
// It returns the path to the extracted directory and the manifest digest.
// Cache hits are served from disk without network. If lockfile is non-nil
// and contains an entry for the feature, the digest from the lockfile is
// used instead of resolving the version tag.
func (c *ociClient) fetchFeature(ctx context.Context, ref *FeatureRef, cacheDir string, lockfile *Lockfile) (extractedPath string, manifestDigest string, err error) {
	manifestDigest, err = c.resolveManifestDigest(ctx, ref, lockfile)
	if err != nil {
		return "", "", fmt.Errorf("resolving manifest digest for %s: %w", ref.String(), err)
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

// fetchManifestDigest fetches the manifest digest for the given reference
// from the registry using go-containerregistry.
func (c *ociClient) fetchManifestDigest(ctx context.Context, ref *FeatureRef) (string, error) {
	r, err := name.ParseReference(ref.String())
	if err != nil {
		return "", fmt.Errorf("parsing reference %s: %w", ref.String(), err)
	}
	desc, err := remote.Head(r, remote.WithContext(ctx))
	if err != nil {
		return "", fmt.Errorf("HEAD manifest for %s: %w", ref.String(), err)
	}
	return desc.Digest.String(), nil
}

// fetchManifest downloads and decodes the OCI manifest for the given reference.
func (c *ociClient) fetchManifest(ctx context.Context, ref *FeatureRef) (*ociManifest, error) {
	r, err := name.ParseReference(ref.String())
	if err != nil {
		return nil, fmt.Errorf("parsing reference %s: %w", ref.String(), err)
	}
	desc, err := remote.Get(r, remote.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("GET manifest for %s: %w", ref.String(), err)
	}
	var m ociManifest
	if err := json.Unmarshal(desc.Manifest, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest JSON for %s: %w", ref.String(), err)
	}
	return &m, nil
}

// extractBlob downloads the given blob digest and extracts it to destDir.
// It uses go-extract to handle decompression and archive extraction.
func (c *ociClient) extractBlob(ctx context.Context, ref *FeatureRef, blobDigest, destDir string) error {
	r, err := name.ParseReference(ref.String())
	if err != nil {
		return fmt.Errorf("parsing reference %s: %w", ref.String(), err)
	}
	digestRef := r.Context().Digest(blobDigest)
	l, err := remote.Layer(digestRef, remote.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("fetching layer %s for %s: %w", blobDigest, ref.String(), err)
	}
	rc, err := l.Compressed()
	if err != nil {
		return fmt.Errorf("reading layer %s for %s: %w", blobDigest, ref.String(), err)
	}
	defer rc.Close()

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("creating destination directory %s: %w", destDir, err)
	}

	cfg := extract.NewConfig(extract.WithCreateDestination(true))
	if err := extract.Unpack(ctx, destDir, rc, cfg); err != nil {
		return fmt.Errorf("extracting blob %s for %s: %w", blobDigest, ref.String(), err)
	}
	return nil
}
