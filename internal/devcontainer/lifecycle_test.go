package devcontainer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kobus-v-schoor/dcx/internal/devcontainer/spec"
)

// execCapture records the arguments passed to the mocked containerExecFunc.
type execCapture struct {
	called      bool
	containerID string
	workdir     string
	cmd         []string
	returnErr   error
}

// mockLifecycleExec overrides containerExecFunc for the duration of a test.
func mockLifecycleExec(t *testing.T) *execCapture {
	cap := &execCapture{}
	orig := containerExecFunc
	containerExecFunc = func(ctx context.Context, containerID, workdir string, cmd []string) error {
		cap.called = true
		cap.containerID = containerID
		cap.workdir = workdir
		cap.cmd = append([]string(nil), cmd...)
		return cap.returnErr
	}
	t.Cleanup(func() { containerExecFunc = orig })
	return cap
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRunLifecycleCommandString(t *testing.T) {
	cap := mockLifecycleExec(t)

	lc := spec.NewLifecycleCommandString("echo hello")
	err := runLifecycleCommand(context.Background(), "abc123", lc, "/workspace", "postCreateCommand")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cap.called {
		t.Fatal("expected exec to be called")
	}
	if cap.containerID != "abc123" {
		t.Errorf("containerID = %q, want %q", cap.containerID, "abc123")
	}
	if cap.workdir != "/workspace" {
		t.Errorf("workdir = %q, want %q", cap.workdir, "/workspace")
	}
	expected := []string{"/bin/sh", "-c", "echo hello"}
	if !sliceEqual(cap.cmd, expected) {
		t.Errorf("cmd = %v, want %v", cap.cmd, expected)
	}
}

func TestRunLifecycleCommandArray(t *testing.T) {
	cap := mockLifecycleExec(t)

	lc := spec.NewLifecycleCommandArray("echo", "hello")
	err := runLifecycleCommand(context.Background(), "abc123", lc, "/workspace", "postCreateCommand")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []string{"echo", "hello"}
	if !sliceEqual(cap.cmd, expected) {
		t.Errorf("cmd = %v, want %v", cap.cmd, expected)
	}
}

func TestRunLifecycleCommandEmpty(t *testing.T) {
	cap := mockLifecycleExec(t)

	lc := spec.LifecycleCommand{}
	err := runLifecycleCommand(context.Background(), "abc123", lc, "/workspace", "postCreateCommand")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cap.called {
		t.Fatal("expected exec NOT to be called")
	}
}

func TestRunLifecycleCommandExitError(t *testing.T) {
	cap := mockLifecycleExec(t)
	cap.returnErr = errors.New("exit status 1")

	lc := spec.NewLifecycleCommandString("false")
	err := runLifecycleCommand(context.Background(), "abc123", lc, "/workspace", "postCreateCommand")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "postCreateCommand failed") {
		t.Errorf("error = %q, want it to contain 'postCreateCommand failed'", err.Error())
	}
}

func TestRunPostCreateLenient(t *testing.T) {
	cap := mockLifecycleExec(t)
	cap.returnErr = errors.New("exit status 1")

	cfg := &spec.Config{
		PostCreateCommand: spec.NewLifecycleCommandString("false"),
		WorkspaceFolder:   "/workspace",
	}
	// RunPostCreate should not panic or return error even when command fails.
	RunPostCreate(context.Background(), "abc123", cfg)
	if !cap.called {
		t.Fatal("expected exec to be called")
	}
}
