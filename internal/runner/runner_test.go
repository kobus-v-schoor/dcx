package runner

import (
	"os"
	"os/exec"
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
	if !contains(msg, "npm install") {
		t.Errorf("error message should mention npm install, got: %s", msg)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(len(s) > 0 && findSubstr(s, sub)))
}

func findSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestMain(m *testing.M) {
	if os.Getenv("GO_TEST_PROCESS") == "1" {
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func init() {
	execErr := exec.Command("sh", "-c", "echo test").Run()
	_ = execErr
}
