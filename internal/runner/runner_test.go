package runner

import (
	"os"
	"strings"
	"testing"
)

func TestFindMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err := Find()
	if err == nil {
		t.Fatal("expected error when devcontainer not on PATH")
	}
}

func TestFindPresent(t *testing.T) {
	tmp := t.TempDir()

	script := `#!/bin/sh
echo "devcontainer"
`
	if err := os.WriteFile(tmp+"/devcontainer", []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", tmp)

	path, err := Find()
	if err != nil {
		t.Fatalf("Find() error: %v", err)
	}
	if path != tmp+"/devcontainer" {
		t.Errorf("path = %q, want %q", path, tmp+"/devcontainer")
	}
}

func TestRunSuccess(t *testing.T) {
	tmp := t.TempDir()

	script := `#!/bin/sh
exit 0
`
	path := tmp + "/fake-cli"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := Run(path, nil); err != nil {
		t.Fatalf("Run() error: %v", err)
	}
}

func TestRunNonZeroExit(t *testing.T) {
	tmp := t.TempDir()

	script := `#!/bin/sh
exit 42
`
	path := tmp + "/fake-cli"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	err := Run(path, nil)
	if err == nil {
		t.Fatal("expected error for non-zero exit")
	}

	var exitErr *ExitCodeError
	if !isExitCodeError(err, &exitErr) {
		t.Fatalf("expected ExitCodeError, got %T: %v", err, err)
	}
	if exitErr.ExitCode != 42 {
		t.Errorf("ExitCode = %d, want 42", exitErr.ExitCode)
	}
}

func TestRunCommandNotFound(t *testing.T) {
	err := Run("/nonexistent/binary", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent binary")
	}

	var exitErr *ExitCodeError
	if isExitCodeError(err, &exitErr) {
		t.Fatalf("should not be ExitCodeError for missing binary, got %v", exitErr)
	}
}

func isExitCodeError(err error, target **ExitCodeError) bool {
	if e, ok := err.(*ExitCodeError); ok {
		*target = e
		return true
	}
	return false
}

func TestFindErrorMessage(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err := Find()
	if err == nil {
		t.Fatal("expected error")
	}

	msg := err.Error()
	if !strings.Contains(msg, "https://github.com/devcontainers/cli") {
		t.Errorf("error message should contain the install link, got: %s", msg)
	}
}
