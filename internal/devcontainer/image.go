package devcontainer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/kobus-v-schoor/dcx/internal/devcontainer/spec"
	"github.com/kobus-v-schoor/dcx/internal/docker"
	"github.com/moby/moby/api/types/build"
	"github.com/moby/moby/client"
)

var slugRegex = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// BuildImage resolves the image reference that should be used for creating
// a devcontainer based on the parsed devcontainer.json spec. It handles
// three cases:
//
//  1. "image" is set → the image is pulled if not already present locally.
//     The reference is returned as-is. If forceRebuild is true, the image
//     is re-pulled even when already present.
//
//  2. "build" or legacy "dockerFile" is set → a Dockerfile is built using
//     the Docker SDK ImageBuild API. The resulting image is tagged with a
//     stable, deterministic name so that identical configs reuse the same
//     image without rebuilding. The stable tag is returned. If forceRebuild
//     is true, the image is rebuilt even when a cached tag already exists.
//
//  3. Neither image nor build is configured → returns an error.
//
// This function is used by the native dcx up path (Part 5). It is safe to
// call repeatedly: image-based references are idempotent, and build-based
// references leverage Docker layer caching.
func BuildImage(ctx context.Context, cli docker.DockerClient, cfg *spec.Config, workspaceFolder string, forceRebuild bool) (string, error) {
	if cfg.Image != "" {
		if err := docker.ImagePullIfMissing(ctx, cli, cfg.Image, forceRebuild); err != nil {
			return "", err
		}
		return cfg.Image, nil
	}

	if cfg.Build != nil || cfg.LegacyDockerfile != "" {
		return buildFromDockerfile(ctx, cli, cfg, workspaceFolder, forceRebuild)
	}

	return "", fmt.Errorf("devcontainer.json does not specify image or build")
}

// buildFromDockerfile builds a devcontainer image from a Dockerfile
// configuration and returns the image reference (stable tag) to use.
// When forceRebuild is true, the image is rebuilt even if a previously
// built stable tag already exists locally.
func buildFromDockerfile(ctx context.Context, cli docker.DockerClient, cfg *spec.Config, workspaceFolder string, forceRebuild bool) (string, error) {
	devcontainerDir := filepath.Join(workspaceFolder, ".devcontainer")

	dockerfileRel := cfg.EffectiveDockerfile()
	if dockerfileRel == "" && cfg.Build != nil {
		dockerfileRel = "Dockerfile"
	}
	if dockerfileRel == "" {
		return "", fmt.Errorf("devcontainer.json build configuration is missing dockerfile")
	}
	dockerfilePath := filepath.Join(devcontainerDir, dockerfileRel)

	// Verify Dockerfile exists.
	if _, err := os.Stat(dockerfilePath); err != nil {
		return "", fmt.Errorf("dockerfile not found at %s: %w", dockerfilePath, err)
	}

	var buildContextDir string
	if cfg.Build != nil && cfg.Build.Context != "" {
		buildContextDir = filepath.Join(devcontainerDir, cfg.Build.Context)
	} else {
		buildContextDir = devcontainerDir
	}

	// Compute stable tag.
	slug := workspaceNameSlug(filepath.Base(workspaceFolder))
	hash, err := computeBuildHash(dockerfilePath, cfg.Build)
	if err != nil {
		return "", fmt.Errorf("computing build hash: %w", err)
	}
	stableTag := fmt.Sprintf("dcx-%s:%s", slug, hash)

	// Fast path: image already built and tagged.
	if !forceRebuild {
		if _, err := cli.ImageInspect(ctx, stableTag); err == nil {
			slog.Debug("reusing cached devcontainer image", "tag", stableTag)
			return stableTag, nil
		}
	}

	slog.Info("building devcontainer image", "tag", stableTag, "context", buildContextDir, "dockerfile", dockerfileRel)

	// Dockerfile path relative to the build context.
	dockerfileInContext, err := filepath.Rel(buildContextDir, dockerfilePath)
	if err != nil {
		return "", fmt.Errorf("resolving dockerfile relative to build context: %w", err)
	}

	opts := client.ImageBuildOptions{
		Tags:       []string{stableTag},
		Dockerfile: filepath.ToSlash(dockerfileInContext),
		Target:     cfg.Build.Target,
		// Use the v1 builder rather than BuildKit. BuildKit via the Docker SDK
		// fails with "no active sessions" when the base image is not already
		// present locally because the SDK does not set up the BuildKit session
		// that the Docker CLI creates automatically. The v1 builder handles
		// pulling base images during the build and is sufficient for the simple
		// Dockerfiles used by devcontainer projects.
		Version: build.BuilderV1,
		Labels: map[string]string{
			docker.DevcontainerLabel: workspaceFolder,
		},
	}

	if cfg.Build != nil && len(cfg.Build.Args) > 0 {
		opts.BuildArgs = make(map[string]*string, len(cfg.Build.Args))
		for k, v := range cfg.Build.Args {
			// Take a copy for the pointer so loop variable reuse doesn't
			// cause all map entries to point to the last value.
			val := v
			opts.BuildArgs[k] = &val
		}
	}

	_, err = docker.ImageBuildFromDir(ctx, cli, buildContextDir, opts)
	if err != nil {
		return "", fmt.Errorf("building devcontainer image: %w", err)
	}

	return stableTag, nil
}

// workspaceNameSlug sanitises a workspace folder name so it can be used as
// part of a Docker image tag. Non-alphanumeric characters are replaced
// with hyphens, the result is lowercased, and truncated to 20 characters
// to keep tag lengths reasonable.
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

// computeBuildHash calculates a deterministic SHA256 hash of the build
// recipe (Dockerfile content, target stage, and sorted build args). The
// hash uniquely identifies the image for a given workspace Dockerfile
// configuration. Docker's own layer caching handles changes to other
// context files.
func computeBuildHash(dockerfilePath string, buildCfg *spec.Build) (string, error) {
	h := sha256.New()

	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		return "", err
	}
	h.Write(content)

	if buildCfg != nil {
		h.Write([]byte(buildCfg.Target))
		args := make([]string, 0, len(buildCfg.Args))
		for k, v := range buildCfg.Args {
			args = append(args, k+"="+v)
		}
		sort.Strings(args)
		for _, a := range args {
			h.Write([]byte(a))
		}
	}

	return hex.EncodeToString(h.Sum(nil))[:16], nil
}
