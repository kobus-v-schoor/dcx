package ssh

import (
	"log/slog"
	"os"

	"github.com/kobus-v-schoor/dcx/internal/config"
	"github.com/kobus-v-schoor/dcx/internal/mounts"
)

// AgentResult holds the mount and environment variable produced by DetectAgent.
// Either both fields are populated (agent detected) or both are zero-valued
// (agent absent or disabled). Consumers check Mount != nil to determine
// whether forwarding is active.
type AgentResult struct {
	Mount    *mounts.ResolvedMount
	EnvName  string
	EnvValue string
}

// DetectAgent checks the host environment for an SSH agent socket. It reads
// SSH_AUTH_SOCK, verifies the socket file exists, and returns an AgentResult
// with the appropriate bind mount and env var. The container mount target is
// read from cfg.AgentSocketTarget (defaulted by the config package to
// /opt/dcx/sockets/ssh-agent.sock). If SSH_AUTH_SOCK is unset or the socket
// does not exist, a warning is logged and an empty result is returned. Called
// by the flags package during dcx up flag assembly.
func DetectAgent(cfg config.SSHConfig) AgentResult {
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

	if info.Mode()&os.ModeSocket == 0 {
		slog.Warn("SSH_AUTH_SOCK path is not a socket, skipping SSH agent forwarding", "path", socketPath)
		return AgentResult{}
	}

	return AgentResult{
		Mount: &mounts.ResolvedMount{
			Source:   socketPath,
			Target:   cfg.AgentSocketTarget,
			ReadOnly: false,
		},
		EnvName:  "SSH_AUTH_SOCK",
		EnvValue: cfg.AgentSocketTarget,
	}
}
