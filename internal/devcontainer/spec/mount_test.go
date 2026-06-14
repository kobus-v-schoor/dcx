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
	// Passing an empty string to Abs returns the working directory on all
	// supported platforms; this test simply verifies the function does not
	// panic and returns a non-empty path.
	cfg := &Config{}
	got := ResolveWorkspaceFolder(cfg, "")
	if got == "" {
		t.Error("expected non-empty path from ResolveWorkspaceFolder with empty input")
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
	if got.Type != "bind" {
		t.Errorf("Type = %q, want bind", got.Type)
	}
	if got.Source != "/host/data" {
		t.Errorf("Source = %q, want /host/data", got.Source)
	}
	if got.Target != "/workspace" {
		t.Errorf("Target = %q, want /workspace", got.Target)
	}
	if len(got.Options) != 1 || got.Options[0] != "readonly" {
		t.Errorf("Options = %v, want [readonly]", got.Options)
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
	if got.Type != "bind" {
		t.Errorf("Type = %q, want bind", got.Type)
	}
	if got.Source != absHost {
		t.Errorf("Source = %q, want %q", got.Source, absHost)
	}
	if got.Target != absHost {
		t.Errorf("Target = %q, want %q", got.Target, absHost)
	}

	if runtime.GOOS == "darwin" {
		if len(got.Options) != 1 || got.Options[0] != "consistency=cached" {
			t.Errorf("Options = %v, want [consistency=cached] on macOS", got.Options)
		}
	} else {
		if len(got.Options) != 0 {
			t.Errorf("Options = %v, want empty on %s", got.Options, runtime.GOOS)
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
	if got.Type != "bind" {
		t.Errorf("Type = %q, want bind", got.Type)
	}
	if got.Source != absHost {
		t.Errorf("Source = %q, want %q", got.Source, absHost)
	}
	if got.Target != "/workspace" {
		t.Errorf("Target = %q, want /workspace", got.Target)
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
	if len(got.Options) != 1 || got.Options[0] != "consistency=cached" {
		t.Errorf("Options = %v, want [consistency=cached] when goos=darwin", got.Options)
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
	if len(got.Options) != 0 {
		t.Errorf("Options = %v, want empty when goos=linux", got.Options)
	}
}

func TestParseMountStringAliases(t *testing.T) {
	mount := "type=bind,src=/host/path,dst=/container/path"
	wm, err := ParseMountString(mount)
	if err != nil {
		t.Fatalf("ParseMountString() error: %v", err)
	}
	if wm.Type != "bind" {
		t.Errorf("Type = %q, want bind", wm.Type)
	}
	if wm.Source != "/host/path" {
		t.Errorf("Source = %q, want /host/path", wm.Source)
	}
	if wm.Target != "/container/path" {
		t.Errorf("Target = %q, want /container/path", wm.Target)
	}
}

func TestParseMountStringDestinationAlias(t *testing.T) {
	mount := "type=bind,source=/host,destination=/dst"
	wm, err := ParseMountString(mount)
	if err != nil {
		t.Fatalf("ParseMountString() error: %v", err)
	}
	if wm.Source != "/host" {
		t.Errorf("Source = %q, want /host", wm.Source)
	}
	if wm.Target != "/dst" {
		t.Errorf("Target = %q, want /dst", wm.Target)
	}
}
