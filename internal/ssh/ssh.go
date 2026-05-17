package ssh

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/kobus-v-schoor/dcx/internal/mounts"
)

const (
	// agentMountTarget is the container path where the SSH agent socket is
	// bind-mounted. Placed under /opt/dcx/ to avoid conflicts with
	// container-installed software, per the project's mount namespace convention.
	agentMountTarget = "/opt/dcx/sockets/ssh-agent.sock"

	// agentEnvValue is the value assigned to SSH_AUTH_SOCK inside the container,
	// pointing to the bind-mounted socket.
	agentEnvValue = agentMountTarget
)

// AgentResult holds the mount and environment variable produced by DetectAgent.
// Either both fields are populated (agent detected) or both are zero-valued
// (agent absent or disabled). Consumers check Mount != nil to determine
// whether forwarding is active.
type AgentResult struct {
	Mount   *mounts.ResolvedMount
	EnvName  string
	EnvValue string
}

// DetectAgent checks the host environment for an SSH agent socket. It reads
// SSH_AUTH_SOCK, verifies the socket file exists, and returns an AgentResult
// with the appropriate bind mount and env var. If SSH_AUTH_SOCK is unset or
// the socket does not exist, a warning is logged and an empty result is
// returned. Called by the flags package during dcx up flag assembly.
func DetectAgent() AgentResult {
	socketPath := os.Getenv("SSH_AUTH_SOCK")
	if socketPath == "" {
		slog.Warn("SSH_AUTH_SOCK not set, skipping SSH agent forwarding")
		return AgentResult{}
	}

	info, err := os.Stat(socketPath)
	if err != nil {
		slog.Warn("SSH_AUTH_SOCK path does not exist, skipping SSH agent forwarding", "path", socketPath)
		return AgentResult{}
	}

	// Verify it is a socket (mode & socket type bit). Some alternative agents
	// like Secretive may use regular files, but the standard OpenSSH agent
	// always creates a Unix domain socket.
	if info.Mode()&os.ModeSocket == 0 {
		slog.Warn("SSH_AUTH_SOCK path is not a socket, skipping SSH agent forwarding", "path", socketPath)
		return AgentResult{}
	}

	return AgentResult{
		Mount: &mounts.ResolvedMount{
			Source:   socketPath,
			Target:   agentMountTarget,
			ReadOnly: false,
		},
		EnvName:  "SSH_AUTH_SOCK",
		EnvValue: agentEnvValue,
	}
}

// FormatAgentEnv formats the --remote-env flag value for SSH agent forwarding.
// Returns the string in NAME=VALUE format suitable for the devcontainer CLI.
func FormatAgentEnv(result AgentResult) string {
	return fmt.Sprintf("%s=%s", result.EnvName, result.EnvValue)
}
