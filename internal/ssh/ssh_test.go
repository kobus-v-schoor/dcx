package ssh

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectAgentSocketPresent(t *testing.T) {
	// Create a temporary Unix domain socket to simulate SSH_AUTH_SOCK.
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "agent.sock")

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Skipf("cannot create unix socket: %v", err)
	}
	defer func() { _ = listener.Close() }()

	t.Setenv("SSH_AUTH_SOCK", socketPath)

	result := DetectAgent()

	if result.Mount == nil {
		t.Fatal("expected Mount to be non-nil when socket exists")
	}
	if result.Mount.Source != socketPath {
		t.Errorf("Mount.Source = %q, want %q", result.Mount.Source, socketPath)
	}
	if result.Mount.Target != agentMountTarget {
		t.Errorf("Mount.Target = %q, want %q", result.Mount.Target, agentMountTarget)
	}
	if result.Mount.ReadOnly {
		t.Error("Mount.ReadOnly should be false for SSH agent socket")
	}
	if result.EnvName != "SSH_AUTH_SOCK" {
		t.Errorf("EnvName = %q, want SSH_AUTH_SOCK", result.EnvName)
	}
	if result.EnvValue != agentEnvValue {
		t.Errorf("EnvValue = %q, want %q", result.EnvValue, agentEnvValue)
	}
}

func TestDetectAgentEnvNotSet(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")

	result := DetectAgent()

	if result.Mount != nil {
		t.Error("expected Mount to be nil when SSH_AUTH_SOCK is unset")
	}
	if result.EnvName != "" {
		t.Errorf("EnvName = %q, want empty", result.EnvName)
	}
}

func TestDetectAgentSocketMissing(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "/nonexistent/path/agent.sock")

	result := DetectAgent()

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

	result := DetectAgent()

	if result.Mount != nil {
		t.Error("expected Mount to be nil when path is not a socket")
	}
}

func TestFormatAgentEnv(t *testing.T) {
	result := AgentResult{
		EnvName:  "SSH_AUTH_SOCK",
		EnvValue: "/opt/dcx/sockets/ssh-agent.sock",
	}

	got := FormatAgentEnv(result)
	want := "SSH_AUTH_SOCK=/opt/dcx/sockets/ssh-agent.sock"
	if got != want {
		t.Errorf("FormatAgentEnv() = %q, want %q", got, want)
	}
}

func TestFormatAgentEnvEmpty(t *testing.T) {
	result := AgentResult{}

	got := FormatAgentEnv(result)
	want := "="
	if got != want {
		t.Errorf("FormatAgentEnv() with empty result = %q, want %q", got, want)
	}
}
