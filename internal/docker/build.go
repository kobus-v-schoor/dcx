package docker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// ImageBuildOptions holds the parameters for a Docker image build.
type ImageBuildOptions struct {
	Tags       []string
	Dockerfile string
	Target     string
	BuildArgs  map[string]string
	Labels     map[string]string
}

// ImageBuildFromDirCLI builds a Docker image from a Dockerfile located inside
// buildContextDir by invoking the Docker CLI (`docker build`). Stdout and
// stderr are wired directly to os.Stdout and os.Stderr so build progress is
// visible in real time. No manual tar archive creation or JSON stream parsing
// is required — the CLI handles the build context, .dockerignore, and
// BuildKit automatically.
//
// The image is tagged with the provided tags (from opts.Tags). Returns the
// image ID of the built image by inspecting the first requested tag after
// the build completes.
func ImageBuildFromDirCLI(ctx context.Context, buildContextDir string, opts ImageBuildOptions) (string, error) {
	args := []string{"build"}

	dockerfile := opts.Dockerfile
	if dockerfile != "" && !filepath.IsAbs(dockerfile) {
		dockerfile = filepath.Join(buildContextDir, dockerfile)
	}
	if dockerfile != "" {
		args = append(args, "--file", dockerfile)
	}

	if opts.Target != "" {
		args = append(args, "--target", opts.Target)
	}

	for k, v := range opts.BuildArgs {
		args = append(args, "--build-arg", k+"="+v)
	}

	for k, v := range opts.Labels {
		args = append(args, "--label", k+"="+v)
	}

	for _, tag := range opts.Tags {
		args = append(args, "--tag", tag)
	}

	// Ensure BuildKit is enabled (the default for modern Docker, but
	// setting the env var explicitly avoids issues on older daemons).
	args = append(args, buildContextDir)

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "DOCKER_BUILDKIT=1")

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker build failed: %w", err)
	}

	if len(opts.Tags) == 0 {
		return "", fmt.Errorf("no tags specified for built image")
	}

	// Need a DockerClient to inspect the built image. Create a temporary
	// client for the inspect call.
	cli, err := NewClient(ctx)
	if err != nil {
		return "", fmt.Errorf("creating Docker client to inspect built image: %w", err)
	}
	defer func() { _ = cli.Close() }()

	tag := opts.Tags[0]
	inspect, err := cli.ImageInspect(ctx, tag)
	if err != nil {
		return "", fmt.Errorf("docker build completed but could not inspect image %s: %w", tag, err)
	}

	return inspect.ID, nil
}
