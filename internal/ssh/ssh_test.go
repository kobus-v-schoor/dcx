package ssh

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/kobus-v-schoor/dcx/internal/config"
)

func TestDetectAgentSocketPresent(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "agent.sock")

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Skipf("cannot create unix socket: %v", err)
	}
	defer func() { _ = listener.Close() }()

	t.Setenv("SSH_AUTH_SOCK", socketPath)

	cfg := config.SSHConfig{
		ForwardAgent:      true,
		AgentSocketTarget: "/opt/dcx/sockets/ssh-agent.sock",
	}
	result := DetectAgent(cfg)

	if result.Mount == nil {
		t.Fatal("expected Mount to be non-nil when socket exists")
	}
	if result.Mount.Source != socketPath {
		t.Errorf("Mount.Source = %q, want %q", result.Mount.Source, socketPath)
	}
	if result.Mount.Target != "/opt/dcx/sockets/ssh-agent.sock" {
		t.Errorf("Mount.Target = %q, want /opt/dcx/sockets/ssh-agent.sock", result.Mount.Target)
	}
	if result.Mount.ReadOnly {
		t.Error("Mount.ReadOnly should be false for SSH agent socket")
	}
	if result.EnvName != "SSH_AUTH_SOCK" {
		t.Errorf("EnvName = %q, want SSH_AUTH_SOCK", result.EnvName)
	}
	if result.EnvValue != "/opt/dcx/sockets/ssh-agent.sock" {
		t.Errorf("EnvValue = %q, want /opt/dcx/sockets/ssh-agent.sock", result.EnvValue)
	}
}

func TestDetectAgentCustomTarget(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "agent.sock")

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Skipf("cannot create unix socket: %v", err)
	}
	defer func() { _ = listener.Close() }()

	t.Setenv("SSH_AUTH_SOCK", socketPath)

	cfg := config.SSHConfig{
		ForwardAgent:      true,
		AgentSocketTarget: "/custom/path/agent.sock",
	}
	result := DetectAgent(cfg)

	if result.Mount == nil {
		t.Fatal("expected Mount to be non-nil")
	}
	if result.Mount.Target != "/custom/path/agent.sock" {
		t.Errorf("Mount.Target = %q, want /custom/path/agent.sock", result.Mount.Target)
	}
	if result.EnvValue != "/custom/path/agent.sock" {
		t.Errorf("EnvValue = %q, want /custom/path/agent.sock", result.EnvValue)
	}
}

func TestDetectAgentEnvNotSet(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")

	cfg := config.SSHConfig{ForwardAgent: true}
	result := DetectAgent(cfg)

	if result.Mount != nil {
		t.Error("expected Mount to be nil when SSH_AUTH_SOCK is unset")
	}
	if result.EnvName != "" {
		t.Errorf("EnvName = %q, want empty", result.EnvName)
	}
}

func TestDetectAgentSocketMissing(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "/nonexistent/path/agent.sock")

	cfg := config.SSHConfig{ForwardAgent: true}
	result := DetectAgent(cfg)

	if result.Mount != nil {
		t.Error("expected Mount to be nil when socket path does not exist")
	}
}

func TestDetectAgentPathIsNotSocket(t *testing.T) {
	dir := t.TempDir()
	regularFile := filepath.Join(dir, "not-a-socket")
	if err := os.WriteFile(regularFile, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SSH_AUTH_SOCK", regularFile)

	cfg := config.SSHConfig{ForwardAgent: true}
	result := DetectAgent(cfg)

	if result.Mount != nil {
		t.Error("expected Mount to be nil when path is not a socket")
	}
}
