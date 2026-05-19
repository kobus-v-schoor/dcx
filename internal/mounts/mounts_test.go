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
	want := `type=bind,source="/host/path",target="/container/path",readonly`
	if got != want {
		t.Errorf("Format() = %q, want %q", got, want)
	}
}

func TestFormatReadOnlyFalse(t *testing.T) {
	m := ResolvedMount{Source: "/host/path", Target: "/container/path", ReadOnly: false}
	got := Format(m)
	want := `type=bind,source="/host/path",target="/container/path"`
	if got != want {
		t.Errorf("Format() = %q, want %q", got, want)
	}
}

func TestBuildFlagsEmpty(t *testing.T) {
	got := BuildFlags(nil)
	if got != nil {
		t.Errorf("BuildFlags(nil) = %v, want nil", got)
	}

	got = BuildFlags([]config.Mount{})
	if got != nil {
		t.Errorf("BuildFlags([]) = %v, want nil", got)
	}
}

func TestBuildFlagsSingleMount(t *testing.T) {
	dir := t.TempDir()

	cfgMounts := []config.Mount{
		{Source: dir, Target: "/container/data", ReadOnly: true},
	}

	flags := BuildFlags(cfgMounts)

	if len(flags) != 2 {
		t.Fatalf("expected 2 flag elements, got %d: %v", len(flags), flags)
	}
	if flags[0] != "--mount" {
		t.Errorf("flag = %q, want --mount", flags[0])
	}
	expected := fmt.Sprintf(`type=bind,source="%s",target="/container/data",readonly`, dir)
	if flags[1] != expected {
		t.Errorf("flag value = %q, want %q", flags[1], expected)
	}
}

func TestBuildFlagsMultipleMounts(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	cfgMounts := []config.Mount{
		{Source: dir1, Target: "/container/a", ReadOnly: false},
		{Source: dir2, Target: "/container/b", ReadOnly: true},
	}

	flags := BuildFlags(cfgMounts)

	if len(flags) != 4 {
		t.Fatalf("expected 4 flag elements, got %d: %v", len(flags), flags)
	}
	if flags[0] != "--mount" {
		t.Errorf("flag[0] = %q, want --mount", flags[0])
	}
	if flags[2] != "--mount" {
		t.Errorf("flag[2] = %q, want --mount", flags[2])
	}
}

func TestBuildFlagsSkipsMissingSource(t *testing.T) {
	dir := t.TempDir()

	cfgMounts := []config.Mount{
		{Source: dir, Target: "/container/exists", ReadOnly: false},
		{Source: "/nonexistent/path", Target: "/container/missing", ReadOnly: false},
	}

	flags := BuildFlags(cfgMounts)

	// Only one mount should survive; missing source is skipped.
	if len(flags) != 2 {
		t.Fatalf("expected 2 flag elements (1 mount), got %d: %v", len(flags), flags)
	}
	if flags[0] != "--mount" {
		t.Errorf("flag = %q, want --mount", flags[0])
	}
}

func TestBuildFlagsAllSkippedReturnsNil(t *testing.T) {
	cfgMounts := []config.Mount{
		{Source: "/nonexistent/a", Target: "/container/a", ReadOnly: false},
		{Source: "/nonexistent/b", Target: "/container/b", ReadOnly: false},
	}

	flags := BuildFlags(cfgMounts)

	if flags != nil {
		t.Errorf("expected nil when all mounts are skipped, got %v", flags)
	}
}
