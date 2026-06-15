package docker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// ImageBuildCLI builds a Docker image using the docker CLI. It constructs
// a `docker build` command from the build context directory, Dockerfile path,
// tags, and build arguments. stdout/stderr are streamed to the user's
// terminal so build progress is visible. Returns an error if the command
// exits non-zero.
func ImageBuildCLI(ctx context.Context, contextDir, dockerfile string, tags []string, buildArgs map[string]string) error {
	var args []string
	args = append(args, "build")
	if dockerfile != "" && dockerfile != "Dockerfile" {
		args = append(args, "-f", dockerfile)
	}
	for _, tag := range tags {
		args = append(args, "-t", tag)
	}
	for k, v := range buildArgs {
		args = append(args, "--build-arg", k+"="+v)
	}
	args = append(args, contextDir)

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker build failed: %w", err)
	}
	return nil
}
