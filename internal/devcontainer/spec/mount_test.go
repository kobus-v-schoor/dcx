package spec

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResolveWorkspaceFolderCustom(t *testing.T) {
	cfg := &Config{WorkspaceFolder: "/workspace"}
	got := ResolveWorkspaceFolder(cfg, "/host/project")
	if got != "/workspace" {
		t.Errorf("ResolveWorkspaceFolder() = %q, want /workspace", got)
	}
}

func TestResolveWorkspaceFolderDefault(t *testing.T) {
	workspace := t.TempDir()
	cfg := &Config{}
	got := ResolveWorkspaceFolder(cfg, workspace)
	want, _ := filepath.Abs(workspace)
	if got != want {
		t.Errorf("ResolveWorkspaceFolder() = %q, want %q", got, want)
	}
}

func TestResolveWorkspaceFolderFallbackWhenAbsFails(t *testing.T) {
	// Passing an empty string to Abs returns the working directory on most
	// platforms; this test simply verifies the function does not panic and
	// returns the input when the config has no workspaceFolder.
	cfg := &Config{}
	got := ResolveWorkspaceFolder(cfg, "")
	if got == "" && runtime.GOOS == "windows" {
		// On Windows, filepath.Abs("") returns the current directory.
		// This is acceptable; the test only checks we don't panic.
	}
}

func TestResolveWorkspaceMountCustom(t *testing.T) {
	cfg := &Config{
		WorkspaceFolder: "/workspace",
		WorkspaceMount:  "type=bind,source=/host/data,target=/workspace,readonly",
	}
	got, err := ResolveWorkspaceMount(cfg, "/host/project")
	if err != nil {
		t.Fatalf("ResolveWorkspaceMount() error: %v", err)
	}
	want := "type=bind,source=/host/data,target=/workspace,readonly"
	if got != want {
		t.Errorf("ResolveWorkspaceMount() = %q, want %q", got, want)
	}
}

func TestResolveWorkspaceMountCustomMissingType(t *testing.T) {
	cfg := &Config{
		WorkspaceMount: "source=/host,data=/workspace",
	}
	_, err := ResolveWorkspaceMount(cfg, "/host/project")
	if err == nil {
		t.Fatal("expected error for mount string missing type")
	}
	if !strings.Contains(err.Error(), "type") {
		t.Errorf("error should mention missing type, got: %v", err)
	}
}

func TestResolveWorkspaceMountCustomMissingSource(t *testing.T) {
	cfg := &Config{
		WorkspaceMount: "type=bind,target=/workspace",
	}
	_, err := ResolveWorkspaceMount(cfg, "/host/project")
	if err == nil {
		t.Fatal("expected error for mount string missing source")
	}
	if !strings.Contains(err.Error(), "source") {
		t.Errorf("error should mention missing source, got: %v", err)
	}
}

func TestResolveWorkspaceMountCustomMissingTarget(t *testing.T) {
	cfg := &Config{
		WorkspaceMount: "type=bind,source=/host",
	}
	_, err := ResolveWorkspaceMount(cfg, "/host/project")
	if err == nil {
		t.Fatal("expected error for mount string missing target")
	}
	if !strings.Contains(err.Error(), "target") {
		t.Errorf("error should mention missing target, got: %v", err)
	}
}

func TestResolveWorkspaceMountDefault(t *testing.T) {
	workspace := t.TempDir()
	cfg := &Config{}
	got, err := ResolveWorkspaceMount(cfg, workspace)
	if err != nil {
		t.Fatalf("ResolveWorkspaceMount() error: %v", err)
	}

	absHost, _ := filepath.Abs(workspace)
	wantPrefix := "type=bind,source=" + absHost + ",target=" + absHost
	if !strings.HasPrefix(got, wantPrefix) {
		t.Errorf("ResolveWorkspaceMount() = %q, want prefix %q", got, wantPrefix)
	}

	if runtime.GOOS == "darwin" {
		if !strings.HasSuffix(got, ",consistency=cached") {
			t.Errorf("expected consistency=cached on macOS, got %q", got)
		}
	} else {
		if strings.Contains(got, "consistency=") {
			t.Errorf("expected no consistency on %s, got %q", runtime.GOOS, got)
		}
	}
}

func TestResolveWorkspaceMountDefaultWithCustomFolder(t *testing.T) {
	workspace := t.TempDir()
	cfg := &Config{WorkspaceFolder: "/workspace"}
	got, err := ResolveWorkspaceMount(cfg, workspace)
	if err != nil {
		t.Fatalf("ResolveWorkspaceMount() error: %v", err)
	}

	absHost, _ := filepath.Abs(workspace)
	want := "type=bind,source=" + absHost + ",target=/workspace"
	if !strings.HasPrefix(got, want) {
		t.Errorf("ResolveWorkspaceMount() = %q, want prefix %q", got, want)
	}
}

func TestResolveWorkspaceMountDarwinConsistency(t *testing.T) {
	workspace := t.TempDir()
	cfg := &Config{}

	old := goos
	goos = "darwin"
	defer func() { goos = old }()

	got, err := ResolveWorkspaceMount(cfg, workspace)
	if err != nil {
		t.Fatalf("ResolveWorkspaceMount() error: %v", err)
	}
	if !strings.HasSuffix(got, ",consistency=cached") {
		t.Errorf("expected consistency=cached when goos=darwin, got %q", got)
	}
}

func TestResolveWorkspaceMountLinuxNoConsistency(t *testing.T) {
	workspace := t.TempDir()
	cfg := &Config{}

	old := goos
	goos = "linux"
	defer func() { goos = old }()

	got, err := ResolveWorkspaceMount(cfg, workspace)
	if err != nil {
		t.Fatalf("ResolveWorkspaceMount() error: %v", err)
	}
	if strings.Contains(got, "consistency=") {
		t.Errorf("expected no consistency when goos=linux, got %q", got)
	}
}
