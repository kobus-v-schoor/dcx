package runner

import (
	"fmt"
	"os"
	"os/exec"
)

const binaryName = "devcontainer"

// Find locates the devcontainer CLI on the user's PATH. It returns the absolute
// path to the binary or an error with a link to installation instructions if
// not found. Called at the start of every dcx command that delegates to the
// devcontainer CLI.
func Find() (string, error) {
	path, err := exec.LookPath(binaryName)
	if err != nil {
		return "", fmt.Errorf(
			"devcontainer CLI not found on PATH.\n" +
				"See: https://github.com/devcontainers/cli",
		)
	}
	return path, nil
}

// ExitCodeError wraps an exec.ExitError to expose the process exit code. Used by
// Run to propagate the devcontainer CLI's exit code back to the caller.
type ExitCodeError struct {
	ExitCode int
}

func (e *ExitCodeError) Error() string {
	return fmt.Sprintf("devcontainer exited with code %d", e.ExitCode)
}

// Run executes the devcontainer CLI at execPath with the given arguments. Stdin,
// stdout, and stderr are forwarded directly to the user's terminal. If the process
// exits with a non-zero code, Run returns an ExitCodeError. Called after flag
// assembly to invoke devcontainer up.
func Run(execPath string, args []string) error {
	cmd := exec.Command(execPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return &ExitCodeError{ExitCode: exitErr.ExitCode()}
		}
		return fmt.Errorf("running devcontainer: %w", err)
	}
	return nil
}
