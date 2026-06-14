package docker

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// ContainerCreateCLI creates a container using the Docker CLI and returns the
// container ID. It constructs a `docker create` command with the provided
// options, appending runArgs verbatim before the image reference and command
// args after it. On failure the stderr from Docker is included in the error.
func ContainerCreateCLI(ctx context.Context, imageRef string, runArgs, mounts, envs []string, labels map[string]string, user, workdir, entrypoint string, cmdArgs []string) (string, error) {
	args := BuildCreateArgs(imageRef, runArgs, mounts, envs, labels, user, workdir, entrypoint, cmdArgs)

	cmd := exec.CommandContext(ctx, "docker", append([]string{"create"}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("docker create failed: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("docker create: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ContainerStartCLI starts a container using the Docker CLI.
func ContainerStartCLI(ctx context.Context, containerID string) error {
	cmd := exec.CommandContext(ctx, "docker", "start", containerID)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker start failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// BuildCreateArgs constructs the argument slice for `docker create`.
// All options appear before the image reference, and any command args appear
// after it, matching the Docker CLI syntax:
//
//	docker create [OPTIONS] IMAGE [COMMAND] [ARG...]
func BuildCreateArgs(imageRef string, runArgs, mounts, envs []string, labels map[string]string, user, workdir, entrypoint string, cmdArgs []string) []string {
	var args []string

	// Labels in deterministic order so tests are stable.
	var labelKeys []string
	for k := range labels {
		labelKeys = append(labelKeys, k)
	}
	sort.Strings(labelKeys)
	for _, k := range labelKeys {
		args = append(args, "--label", k+"="+labels[k])
	}

	for _, m := range mounts {
		args = append(args, "--mount", m)
	}

	for _, e := range envs {
		args = append(args, "--env", e)
	}

	if user != "" {
		args = append(args, "--user", user)
	}

	if workdir != "" {
		args = append(args, "--workdir", workdir)
	}

	if entrypoint != "" {
		args = append(args, "--entrypoint", entrypoint)
	}

	args = append(args, runArgs...)

	args = append(args, imageRef)

	args = append(args, cmdArgs...)

	return args
}
