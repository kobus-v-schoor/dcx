package ssh

import (
	"log/slog"
	"os"

	"github.com/kobus-v-schoor/dcx/internal/colima"
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
// SSH_AUTH_SOCK, verifies the socket file exists on the host filesystem, and
// returns an AgentResult with the appropriate bind mount and env var. The
// container mount target is read from cfg.AgentSocketTarget (defaulted by the
// config package to /opt/dcx/sockets/ssh-agent.sock). If SSH_AUTH_SOCK is
// unset or the socket does not exist, a warning is logged and an empty result
// is returned.
func DetectAgent(cfg config.SSHConfig) AgentResult {
	socketPath := os.Getenv("SSH_AUTH_SOCK")
	if socketPath == "" {
		slog.Warn("SSH_AUTH_SOCK not set, skipping SSH agent forwarding")
		return AgentResult{}
	}
	return resolveAgent(cfg, socketPath, true)
}

// ResolveAgent detects the SSH agent socket, accounting for container
// runtimes that run inside a VM (e.g. Colima on macOS). When Colima is
// the active Docker runtime, it reads SSH_AUTH_SOCK from inside the Colima
// VM (where the Docker daemon lives) so that the bind-mount source path is
// valid for the Docker daemon rather than the macOS host. If SSH agent
// forwarding is not enabled in the VM, it may prompt the user to restart
// Colima with --ssh-agent when stdin is a terminal. Falls back to
// DetectAgent when Colima is not in use.
func ResolveAgent(cfg config.SSHConfig) AgentResult {
	if colima.IsActive() {
		profile := colima.Profile()
		vmSocket, err := colima.EnsureSSHAgent(profile)
		if err != nil {
			slog.Warn("colima SSH agent resolution failed, skipping forwarding", "error", err)
			return AgentResult{}
		}
		return resolveAgent(cfg, vmSocket, false)
	}
	return DetectAgent(cfg)
}

// resolveAgent builds an AgentResult for the given socket path. When
// validateHost is true, it checks that the socket exists and is a Unix socket
// on the local filesystem. When validateHost is false (e.g. the path refers
// to a file inside a VM), the stat check is skipped because the path is not
// resolvable from the host.
func resolveAgent(cfg config.SSHConfig, socketPath string, validateHost bool) AgentResult {
	if validateHost {
		info, err := os.Stat(socketPath)
		if err != nil {
			slog.Warn("SSH_AUTH_SOCK path does not exist, skipping SSH agent forwarding", "path", socketPath)
			return AgentResult{}
		}

		if info.Mode()&os.ModeSocket == 0 {
			slog.Warn("SSH_AUTH_SOCK path is not a socket, skipping SSH agent forwarding", "path", socketPath)
			return AgentResult{}
		}
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
