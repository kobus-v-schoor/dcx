package colima

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/term"
)

// dockerConfigFile returns the path to the Docker CLI config.json. It respects
// the DOCKER_CONFIG environment variable, falling back to ~/.docker/config.json.
func dockerConfigFile() string {
	if p := os.Getenv("DOCKER_CONFIG"); p != "" {
		return filepath.Join(p, "config.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".docker", "config.json")
}

type dockerConfig struct {
	CurrentContext string `json:"currentContext"`
}

// currentContext reads the current Docker context name from the Docker CLI
// config file. Returns empty string when the config file is missing or unreadable.
func currentContext() string {
	data, err := os.ReadFile(dockerConfigFile())
	if err != nil {
		return ""
	}
	var cfg dockerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ""
	}
	return cfg.CurrentContext
}

// IsActive reports whether the active Docker context points to a Colima
// instance. It checks the current context name from ~/.docker/config.json
// (or $DOCKER_CONFIG/config.json) and falls back to inspecting DOCKER_HOST.
func IsActive() bool {
	ctx := currentContext()
	if strings.HasPrefix(ctx, "colima") {
		return true
	}
	if strings.Contains(os.Getenv("DOCKER_HOST"), ".colima") {
		return true
	}
	return false
}

// Profile returns the Colima profile name inferred from the current Docker
// context. The default profile (context name "colima") returns "default".
// Named profiles (context name "colima-<name>") return the name suffix. Returns
// empty string when Colima is not the active runtime.
func Profile() string {
	ctx := currentContext()
	if ctx == "colima" {
		return "default"
	}
	if strings.HasPrefix(ctx, "colima-") {
		return strings.TrimPrefix(ctx, "colima-")
	}
	if strings.Contains(os.Getenv("DOCKER_HOST"), ".colima") {
		return "default"
	}
	return ""
}

// SSHAuthSock returns the SSH_AUTH_SOCK path inside the Colima VM. It runs
// `colima ssh` to read the environment variable and verifies that the socket
// file actually exists in the VM. Returns an error if the agent socket is not
// forwarded or the VM is unreachable.
func SSHAuthSock(profile string) (string, error) {
	out, err := colimaSSH(profile, "/bin/sh", "-c", `echo "$SSH_AUTH_SOCK"`)
	if err != nil {
		return "", fmt.Errorf("checking SSH_AUTH_SOCK in colima VM: %w", err)
	}

	socket := strings.TrimSpace(string(out))
	if socket == "" {
		return "", fmt.Errorf("SSH agent forwarding not enabled in colima VM")
	}

	// Verify the socket actually exists in the VM.
	_, err = colimaSSH(profile, "/bin/sh", "-c", `test -S "$SSH_AUTH_SOCK"`)
	if err != nil {
		return "", fmt.Errorf("SSH agent socket %q does not exist in colima VM", socket)
	}

	return socket, nil
}

// EnsureSSHAgent checks whether SSH agent forwarding is enabled in the Colima
// VM. If it is not, and stdin is connected to a terminal, it prompts the user
// to restart Colima with --ssh-agent and performs the restart when confirmed.
// Returns the VM-resident SSH_AUTH_SOCK path.
func EnsureSSHAgent(profile string) (string, error) {
	socket, err := SSHAuthSock(profile)
	if err == nil && socket != "" {
		slog.Info("colima SSH agent socket resolved", "socket", socket, "profile", profile)
		return socket, nil
	}

	slog.Warn("SSH agent forwarding not enabled in colima VM", "profile", profile)

	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", fmt.Errorf("SSH agent forwarding is not enabled in colima; run 'colima stop && colima start --ssh-agent' to enable it")
	}

	fmt.Fprintf(os.Stderr, "SSH agent forwarding is not enabled in colima. Restart colima with --ssh-agent? [y/N] ")
	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read prompt response: %w", err)
	}

	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer != "y" && answer != "yes" {
		return "", fmt.Errorf("SSH agent forwarding not enabled; user declined restart")
	}

	slog.Info("restarting colima with SSH agent forwarding", "profile", profile)
	if err := restartWithSSHAgent(profile); err != nil {
		return "", fmt.Errorf("restarting colima with --ssh-agent: %w", err)
	}

	socket, err = SSHAuthSock(profile)
	if err != nil {
		return "", fmt.Errorf("SSH agent forwarding still not available after restart: %w", err)
	}
	return socket, nil
}

// colimaSSH runs a command in the Colima VM via `colima ssh`.
func colimaSSH(profile string, args ...string) ([]byte, error) {
	cmdArgs := []string{"ssh"}
	if profile != "" && profile != "default" {
		cmdArgs = append(cmdArgs, "--profile", profile)
	}
	cmdArgs = append(cmdArgs, "--")
	cmdArgs = append(cmdArgs, args...)
	return exec.Command("colima", cmdArgs...).Output()
}

// restartWithSSHAgent stops and starts the Colima instance with the
// --ssh-agent flag.
func restartWithSSHAgent(profile string) error {
	stopArgs := []string{"stop"}
	startArgs := []string{"start", "--ssh-agent"}
	if profile != "" && profile != "default" {
		stopArgs = append(stopArgs, "--profile", profile)
		startArgs = append(startArgs, "--profile", profile)
	}

	slog.Info("stopping colima", "profile", profile)
	out, err := exec.Command("colima", stopArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("colima stop failed: %w\n%s", err, string(out))
	}

	slog.Info("starting colima with --ssh-agent", "profile", profile)
	out, err = exec.Command("colima", startArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("colima start --ssh-agent failed: %w\n%s", err, string(out))
	}

	return nil
}
