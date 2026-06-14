package docker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// ContainerExecCLI executes a command inside a running container using the
// Docker CLI. It streams stdout and stderr directly to the host's stdout
// and stderr so the user sees lifecycle logs in real time. Returns an
// *ExitCodeError if the command exits with a non-zero code.
func ContainerExecCLI(ctx context.Context, containerID, workdir string, cmd []string) error {
	args := []string{"exec"}
	if workdir != "" {
		args = append(args, "--workdir", workdir)
	}
	args = append(args, containerID)
	args = append(args, cmd...)

	c := exec.CommandContext(ctx, "docker", args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return &ExitCodeError{ExitCode: exitErr.ExitCode()}
		}
		return fmt.Errorf("docker exec failed: %w", err)
	}
	return nil
}
