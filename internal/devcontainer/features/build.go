package features

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/kobus-v-schoor/dcx/internal/docker"
)

// BuildFeatureImage resolves, orders, and builds all features on top of
// the given base image. It returns a stable image tag. If the stable
// tag already exists locally and forceRebuild is false, it returns
// immediately without rebuilding.
func BuildFeatureImage(ctx context.Context, cli docker.DockerClient, baseImageRef string, cfgFeatures map[string]json.RawMessage, containerUser, remoteUser, workspaceFolder string, forceRebuild bool) (string, error) {
	// Parse feature references.
	var refs []*FeatureRef
	for id, opts := range cfgFeatures {
		ref, err := Parse(id, opts)
		if err != nil {
			return "", fmt.Errorf("parsing feature %s: %w", id, err)
		}
		refs = append(refs, ref)
	}

	// Resolve each feature (download + parse metadata).
	var resolved []ResolvedFeature
	for _, ref := range refs {
		rf, err := Resolve(ctx, ref, workspaceFolder)
		if err != nil {
			return "", fmt.Errorf("resolving feature %s: %w", ref.String(), err)
		}
		resolved = append(resolved, *rf)
	}

	// Determine installation order.
	ordered, err := Ordered(resolved, nil)
	if err != nil {
		return "", fmt.Errorf("ordering features: %w", err)
	}

	// Compute stable tag.
	stableTag, err := stableFeatureTag(cli, baseImageRef, workspaceFolder, ordered)
	if err != nil {
		return "", fmt.Errorf("computing stable feature tag: %w", err)
	}

	// Fast path: cached image exists.
	if !forceRebuild {
		if _, err := cli.ImageInspect(ctx, stableTag); err == nil {
			slog.Debug("reusing cached feature image", "tag", stableTag)
			return stableTag, nil
		}
	}

	// Generate build context.
	contextDir, _, err := BuildContext(baseImageRef, ordered, containerUser, remoteUser)
	if err != nil {
		return "", fmt.Errorf("generating feature build context: %w", err)
	}
	defer os.RemoveAll(contextDir)

	slog.Info("building feature image", "tag", stableTag, "features", len(ordered))

	// Build via Docker CLI.
	if err := docker.ImageBuildCLI(ctx, contextDir, "", []string{stableTag}, nil); err != nil {
		return "", fmt.Errorf("building feature image %s: %w", stableTag, err)
	}

	slog.Info("feature image built", "tag", stableTag)
	return stableTag, nil
}

var slugRegex = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// workspaceNameSlug sanitises a workspace folder name so it can be used as
// part of a Docker image tag. Non-alphanumeric characters are replaced
// with hyphens, the result is lowercased, and truncated to 20 characters.
func workspaceNameSlug(name string) string {
	slug := slugRegex.ReplaceAllString(name, "-")
	slug = strings.ToLower(strings.Trim(slug, "-"))
	if slug == "" {
		slug = "workspace"
	}
	if len(slug) > 20 {
		slug = slug[:20]
	}
	return slug
}

// stableFeatureTag computes a deterministic image tag for the given base image
// and ordered features.
func stableFeatureTag(cli docker.DockerClient, baseImageRef, workspaceFolder string, features []ResolvedFeature) (string, error) {
	slug := workspaceNameSlug(filepath.Base(workspaceFolder))

	// Inspect base image to get its digest for a truly stable hash.
	baseDigest := baseImageRef
	inspect, err := cli.ImageInspect(context.Background(), baseImageRef)
	if err == nil && inspect.ID != "" {
		baseDigest = inspect.ID
	}

	h := sha256.New()
	h.Write([]byte(baseDigest))

	// Sort features by canonical ID so the hash is order-independent at this stage.
	keys := make([]string, len(features))
	for i, f := range features {
		keys[i] = f.Ref.String()
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, f := range features {
			if f.Ref.String() == k {
				h.Write([]byte(f.Ref.String()))
				h.Write([]byte(f.Meta.Version))
				opts, _ := json.Marshal(f.Ref.Options)
				h.Write(opts)
				h.Write([]byte(f.Digest))
				break
			}
		}
	}

	hash := hex.EncodeToString(h.Sum(nil))[:16]
	return fmt.Sprintf("dcx-%s-feat:%s", slug, hash), nil
}
