package cli

import (
	"testing"

	"github.com/kobus-v-schoor/dcx/internal/config"
)

func TestBuildExecArgsDefaultShell(t *testing.T) {
	cfg := &config.Config{DefaultShell: "zsh"}
	args := buildExecArgs(cfg, "abc123", nil, nil, "")
	if args[len(args)-1] != "zsh" {
		t.Errorf("expected shell zsh, got %q", args[len(args)-1])
	}
}

func TestBuildExecArgsBashFallback(t *testing.T) {
	args := buildExecArgs(nil, "abc123", nil, nil, "")
	if args[len(args)-1] != "bash" {
		t.Errorf("expected shell bash, got %q", args[len(args)-1])
	}
}

func TestBuildExecArgsUserOverridesDefaultShell(t *testing.T) {
	cfg := &config.Config{DefaultShell: "zsh"}
	args := buildExecArgs(cfg, "abc123", nil, []string{"make", "test"}, "")
	expected := []string{"make", "test"}
	got := args[len(args)-len(expected):]
	for i, want := range expected {
		if got[i] != want {
			t.Errorf("user arg[%d] = %q, want %q", i, got[i], want)
		}
	}
}
