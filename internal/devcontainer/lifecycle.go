package devcontainer

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/kobus-v-schoor/dcx/internal/devcontainer/spec"
	"github.com/kobus-v-schoor/dcx/internal/docker"
)

// containerExecFunc is the function used by runLifecycleCommand to execute
// commands inside a running container. In production it is
// docker.ContainerExecCLI; tests may override it to capture constructed
// arguments without invoking Docker.
var containerExecFunc = docker.ContainerExecCLI

// runLifecycleCommand executes a single lifecycle command inside the given
// container. String commands are run via /bin/sh -c; array commands are
// executed directly without a shell. Object-form commands are logged as
// unsupported and skipped. The command runs in workdir if non-empty.
// Output is streamed to the host terminal. Returns an error if the
// command exits with a non-zero code or if docker exec itself fails.
func runLifecycleCommand(ctx context.Context, containerID string, lc spec.LifecycleCommand, workdir string, label string) error {
	if lc.IsEmpty() {
		return nil
	}

	var containerCmd []string
	if s, ok := lc.AsString(); ok {
		containerCmd = []string{"/bin/sh", "-c", s}
	} else if arr, ok := lc.AsArray(); ok {
		containerCmd = arr
	} else {
		slog.Warn("unsupported lifecycle command form (object), skipping", "label", label)
		return nil
	}

	slog.Info("running lifecycle command", "label", label, "command", strings.Join(containerCmd, " "))
	if err := containerExecFunc(ctx, containerID, workdir, containerCmd); err != nil {
		return fmt.Errorf("lifecycle command %s failed: %w", label, err)
	}

	slog.Info("lifecycle command completed", "label", label)
	return nil
}

// RunPostCreate executes the container's postCreateCommand if one is defined.
// It is called after a new devcontainer has been created and started. A
// failing post-create command is logged as a warning but does not abort the
// up flow, matching the devcontainer CLI's lenient default behaviour.
func RunPostCreate(ctx context.Context, containerID string, cfg *spec.Config) {
	if err := runLifecycleCommand(ctx, containerID, cfg.PostCreateCommand, cfg.WorkspaceFolder, "postCreateCommand"); err != nil {
		slog.Warn("postCreateCommand failed", "error", err)
	}
}

// RunPostStart executes the container's postStartCommand if one is defined.
// It is called each time the container is started. Returns an error if the
// command exits with a non-zero code.
func RunPostStart(ctx context.Context, containerID string, cfg *spec.Config) error {
	return runLifecycleCommand(ctx, containerID, cfg.PostStartCommand, cfg.WorkspaceFolder, "postStartCommand")
}

// RunPostAttach executes the container's postAttachCommand if one is defined.
// It is called each time a tool attaches to the container. Returns an error
// if the command exits with a non-zero code.
func RunPostAttach(ctx context.Context, containerID string, cfg *spec.Config) error {
	return runLifecycleCommand(ctx, containerID, cfg.PostAttachCommand, cfg.WorkspaceFolder, "postAttachCommand")
}
