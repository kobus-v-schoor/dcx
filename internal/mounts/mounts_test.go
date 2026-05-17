package mounts

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kobus-v-schoor/dcx/internal/config"
)

func TestExpandHome(t *testing.T) {
	home := "/home/testuser"
	t.Setenv("HOME", home)

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "tilde slash expanded",
			path: "~/projects/myapp",
			want: filepath.Join(home, "projects/myapp"),
		},
		{
			name: "bare tilde not expanded",
			path: "~",
			want: "~",
		},
		{
			name: "no tilde unchanged",
			path: "/absolute/path",
			want: "/absolute/path",
		},
		{
			name: "relative path unchanged",
			path: "relative/path",
			want: "relative/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandHome(tt.path)
			if got != tt.want {
				t.Errorf("expandHome(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestExpandEnvVars(t *testing.T) {
	t.Setenv("MY_PROJECT_DIR", "/projects/myapp")

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "single var expanded",
			path: "${MY_PROJECT_DIR}/data",
			want: "/projects/myapp/data",
		},
		{
			name: "entire path is var",
			path: "${MY_PROJECT_DIR}",
			want: "/projects/myapp",
		},
		{
			name: "unset var left unchanged",
			path: "${UNSET_VAR_12345}/data",
			want: "${UNSET_VAR_12345}/data",
		},
		{
			name: "no vars unchanged",
			path: "/plain/path",
			want: "/plain/path",
		},
		{
			name: "multiple vars expanded",
			path: "${MY_PROJECT_DIR}:${MY_PROJECT_DIR}",
			want: "/projects/myapp:/projects/myapp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandEnvVars(tt.path)
			if got != tt.want {
				t.Errorf("expandEnvVars(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

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
	want := "type=bind,source=/host/path,target=/container/path,readonly"
	if got != want {
		t.Errorf("Format() = %q, want %q", got, want)
	}
}

func TestFormatReadOnlyFalse(t *testing.T) {
	m := ResolvedMount{Source: "/host/path", Target: "/container/path", ReadOnly: false}
	got := Format(m)
	want := "type=bind,source=/host/path,target=/container/path"
	if got != want {
		t.Errorf("Format() = %q, want %q", got, want)
	}
}

func TestFormatWithSpacesInSource(t *testing.T) {
	m := ResolvedMount{Source: "/host/my path", Target: "/container/path", ReadOnly: false}
	got := Format(m)
	want := `type=bind,source="/host/my path",target=/container/path`
	if got != want {
		t.Errorf("Format() = %q, want %q", got, want)
	}
}

func TestFormatWithSpacesInTarget(t *testing.T) {
	m := ResolvedMount{Source: "/host/path", Target: "/container/my path", ReadOnly: true}
	got := Format(m)
	want := `type=bind,source=/host/path,target="/container/my path",readonly`
	if got != want {
		t.Errorf("Format() = %q, want %q", got, want)
	}
}

func TestQuoteMountValue(t *testing.T) {
	tests := []struct {
		name string
		val  string
		want string
	}{
		{name: "no special chars", val: "/plain/path", want: "/plain/path"},
		{name: "contains space", val: "/path with spaces", want: `"/path with spaces"`},
		{name: "contains comma", val: "/path,with,commas", want: `"/path,with,commas"`},
		{name: "contains equals", val: "/path=equals", want: `"/path=equals"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := quoteMountValue(tt.val)
			if got != tt.want {
				t.Errorf("quoteMountValue(%q) = %q, want %q", tt.val, got, tt.want)
			}
		})
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
	expected := "type=bind,source=" + dir + ",target=/container/data,readonly"
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

func TestBuildFlagsDuplicateTargetWarning(t *testing.T) {
	dir := t.TempDir()

	cfgMounts := []config.Mount{
		{Source: dir, Target: "/container/same", ReadOnly: false},
		{Source: dir, Target: "/container/same", ReadOnly: true},
	}

	flags := BuildFlags(cfgMounts)

	// Both mounts should still be present despite duplicate target.
	if len(flags) != 4 {
		t.Fatalf("expected 4 flag elements (2 mounts), got %d: %v", len(flags), flags)
	}
}
