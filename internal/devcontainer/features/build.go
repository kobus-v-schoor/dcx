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
	"sort"
	"strings"

	"github.com/kobus-v-schoor/dcx/internal/docker"
	"github.com/moby/moby/api/types/build"
	"github.com/moby/moby/client"
)

// BuildFeatureImage resolves, orders, and builds all features on top of
// the given base image. It returns a stable image tag. If the stable
// tag already exists locally and forceRebuild is false, it returns
// immediately without rebuilding.
//
// When upgradeLockfile is true the lockfile is not consulted during
// resolution and a new devcontainer-lock.json is written after features are
// resolved so that subsequent builds are pinned to the exact digests.
func BuildFeatureImage(ctx context.Context, cli docker.DockerClient, baseImageRef string, cfgFeatures map[string]json.RawMessage, containerUser, remoteUser, workspaceFolder string, forceRebuild, upgradeLockfile bool) (string, error) {
	// Parse feature references.
	var refs []*FeatureRef
	for id, opts := range cfgFeatures {
		ref, err := Parse(id, opts)
		if err != nil {
			return "", fmt.Errorf("parsing feature %s: %w", id, err)
		}
		refs = append(refs, ref)
	}

	// Load lockfile if present to pin features to specific digests.
	// When upgrading the lockfile, ignore the existing one so that the
	// latest digests are fetched.
	lockfile, _ := LoadLockfile(workspaceFolder)
	if upgradeLockfile {
		lockfile = nil
	}

	// Resolve each feature (download + parse metadata).
	var resolved []ResolvedFeature
	for _, ref := range refs {
		rf, err := Resolve(ctx, ref, workspaceFolder, lockfile)
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

	// Write the lockfile if upgrading so that future builds are pinned.
	if upgradeLockfile {
		lf := &Lockfile{Features: make(map[string]LockfileFeature)}
		for _, rf := range resolved {
			if rf.Ref.Source != SourceOCI || rf.Digest == "" {
				continue
			}
			resolvedStr := fmt.Sprintf("%s/%s/%s@%s", rf.Ref.Registry, rf.Ref.Namespace, rf.Ref.ID, rf.Digest)
			lf.Features[strings.ToLower(rf.Ref.RawID)] = LockfileFeature{
				Version:   rf.Meta.Version,
				Resolved:  resolvedStr,
				Integrity: rf.Digest,
				DependsOn: dependsOnKeys(rf.Meta.DependsOn),
			}
		}
		if err := SaveLockfile(workspaceFolder, lf); err != nil {
			return "", fmt.Errorf("saving lockfile: %w", err)
		}
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
	if contextDir != "" {
		defer func() { _ = os.RemoveAll(contextDir) }()
	}
	if err != nil {
		return "", fmt.Errorf("generating feature build context: %w", err)
	}

	slog.Info("building feature image", "tag", stableTag, "features", len(ordered))

	// Build via Docker SDK ImageBuild API using the v1 builder.
	opts := client.ImageBuildOptions{
		Tags:        []string{stableTag},
		Dockerfile:  "Dockerfile",
		Version:     build.BuilderV1,
		Remove:      true,
		ForceRemove: true,
	}
	if _, err := docker.ImageBuildFromDir(ctx, cli, contextDir, opts); err != nil {
		return "", fmt.Errorf("building feature image %s: %w", stableTag, err)
	}

	slog.Info("feature image built", "tag", stableTag)
	return stableTag, nil
}

// stableFeatureTag computes a deterministic image tag for the given base image
// and ordered features.
func stableFeatureTag(cli docker.DockerClient, baseImageRef, workspaceFolder string, features []ResolvedFeature) (string, error) {
	slug := docker.WorkspaceNameSlug(filepath.Base(workspaceFolder))

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

// dependsOnKeys extracts the keys of a dependsOn map into a sorted slice.
// It returns nil when the map is empty so that omitempty skips the field
// in the lockfile JSON.
func dependsOnKeys(m map[string]interface{}) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
