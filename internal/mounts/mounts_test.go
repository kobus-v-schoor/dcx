package mounts

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/kobus-v-schoor/dcx/internal/config"
)

func TestResolveWithTildeExpansion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, "mydata")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	m := config.Mount{Source: "~/mydata", Target: "/container/mydata"}
	resolved := Resolve(m)

	if resolved == nil {
		t.Fatal("expected non-nil resolved mount")
	}
	if resolved.Source != dir {
		t.Errorf("Source = %q, want %q", resolved.Source, dir)
	}
}

func TestResolveWithEnvVarExpansion(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DCX_TEST_DATA", dir)

	m := config.Mount{Source: "${DCX_TEST_DATA}", Target: "/container/data"}
	resolved := Resolve(m)

	if resolved == nil {
		t.Fatal("expected non-nil resolved mount")
	}
	if resolved.Source != dir {
		t.Errorf("Source = %q, want %q", resolved.Source, dir)
	}
}

func TestResolveMissingSourceSkipped(t *testing.T) {
	m := config.Mount{Source: "/nonexistent/path/that/does/not/exist", Target: "/container/data"}
	resolved := Resolve(m)

	if resolved != nil {
		t.Error("expected nil for missing source path")
	}
}

func TestResolvePreservesTargetAndReadOnly(t *testing.T) {
	dir := t.TempDir()

	m := config.Mount{Source: dir, Target: "/container/data", ReadOnly: true}
	resolved := Resolve(m)

	if resolved == nil {
		t.Fatal("expected non-nil resolved mount")
	}
	if resolved.Target != "/container/data" {
		t.Errorf("Target = %q, want %q", resolved.Target, "/container/data")
	}
	if !resolved.ReadOnly {
		t.Error("ReadOnly should be true")
	}
}

func TestFormatReadOnlyTrue(t *testing.T) {
	m := ResolvedMount{Source: "/host/path", Target: "/container/path", ReadOnly: true}
	got := Format(m)
	want := `type=bind,source=/host/path,target=/container/path,readonly`
	if got != want {
		t.Errorf("Format() = %q, want %q", got, want)
	}
}

func TestFormatReadOnlyFalse(t *testing.T) {
	m := ResolvedMount{Source: "/host/path", Target: "/container/path", ReadOnly: false}
	got := Format(m)
	want := `type=bind,source=/host/path,target=/container/path`
	if got != want {
		t.Errorf("Format() = %q, want %q", got, want)
	}
}

func TestBuildStringsEmpty(t *testing.T) {
	got := BuildStrings(nil, "")
	if got != nil {
		t.Errorf("BuildStrings(nil) = %v, want nil", got)
	}

	got = BuildStrings([]config.Mount{}, "")
	if got != nil {
		t.Errorf("BuildStrings([]) = %v, want nil", got)
	}
}

func TestBuildStringsSingleMount(t *testing.T) {
	dir := t.TempDir()

	cfgMounts := []config.Mount{
		{Source: dir, Target: "/container/data", ReadOnly: true},
	}

	result := BuildStrings(cfgMounts, "")

	if len(result) != 1 {
		t.Fatalf("expected 1 mount string, got %d: %v", len(result), result)
	}
	expected := fmt.Sprintf(`type=bind,source=%s,target=/container/data,readonly`, dir)
	if result[0] != expected {
		t.Errorf("mount string = %q, want %q", result[0], expected)
	}
}

func TestBuildStringsMultipleMounts(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	cfgMounts := []config.Mount{
		{Source: dir1, Target: "/container/a", ReadOnly: false},
		{Source: dir2, Target: "/container/b", ReadOnly: true},
	}

	result := BuildStrings(cfgMounts, "")

	if len(result) != 2 {
		t.Fatalf("expected 2 mount strings, got %d: %v", len(result), result)
	}
}

func TestBuildStringsSkipsMissingSource(t *testing.T) {
	dir := t.TempDir()

	cfgMounts := []config.Mount{
		{Source: dir, Target: "/container/exists", ReadOnly: false},
		{Source: "/nonexistent/path", Target: "/container/missing", ReadOnly: false},
	}

	result := BuildStrings(cfgMounts, "")

	// Only one mount should survive; missing source is skipped.
	if len(result) != 1 {
		t.Fatalf("expected 1 mount string, got %d: %v", len(result), result)
	}
}

func TestBuildStringsAllSkippedReturnsNil(t *testing.T) {
	cfgMounts := []config.Mount{
		{Source: "/nonexistent/a", Target: "/container/a", ReadOnly: false},
		{Source: "/nonexistent/b", Target: "/container/b", ReadOnly: false},
	}

	result := BuildStrings(cfgMounts, "")

	if result != nil {
		t.Errorf("expected nil when all mounts are skipped, got %v", result)
	}
}

func TestExpandContainerHome(t *testing.T) {
	tests := []struct {
		path     string
		homeDir  string
		expected string
	}{
		{"~/.ssh/config", "/home/vscode", "/home/vscode/.ssh/config"},
		{"~/data", "/root", "/root/data"},
		{"/absolute/path", "/home/vscode", "/absolute/path"},
		{"~/.ssh/config", "", "~/.ssh/config"},
		{"no/tilde/here", "/home/vscode", "no/tilde/here"},
		{"~", "/home/vscode", "~"}, // only ~/ prefix expands, not bare ~
	}

	for _, tt := range tests {
		got := ExpandContainerHome(tt.path, tt.homeDir)
		if got != tt.expected {
			t.Errorf("ExpandContainerHome(%q, %q) = %q, want %q", tt.path, tt.homeDir, got, tt.expected)
		}
	}
}

func TestBuildStringsExpandsTargetTilde(t *testing.T) {
	dir := t.TempDir()

	cfgMounts := []config.Mount{
		{Source: dir, Target: "~/.ssh/config", ReadOnly: true},
	}

	result := BuildStrings(cfgMounts, "/home/vscode")

	if len(result) != 1 {
		t.Fatalf("expected 1 mount string, got %d: %v", len(result), result)
	}
	expected := fmt.Sprintf(`type=bind,source=%s,target=/home/vscode/.ssh/config,readonly`, dir)
	if result[0] != expected {
		t.Errorf("mount string = %q, want %q", result[0], expected)
	}
}

func TestBuildStringsSkipsTargetTildeWhenHomeUnknown(t *testing.T) {
	dir := t.TempDir()

	cfgMounts := []config.Mount{
		{Source: dir, Target: "~/.ssh/config", ReadOnly: true},
	}

	result := BuildStrings(cfgMounts, "")

	// Mount should be skipped because container home is unknown.
	if result != nil {
		t.Errorf("expected nil when ~ target cannot expand, got %v", result)
	}
}
