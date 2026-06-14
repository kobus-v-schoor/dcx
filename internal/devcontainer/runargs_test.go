package devcontainer

import (
	"strings"
	"testing"

	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
)

func TestParseRunArgsPublish(t *testing.T) {
	r, err := ParseRunArgs([]string{"--publish", "8080:80"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.PortBindings) != 1 {
		t.Fatalf("expected 1 port binding, got %d", len(r.PortBindings))
	}
	_, ok := r.PortBindings[network.MustParsePort("80/tcp")]
	if !ok {
		t.Fatalf("expected binding for 80/tcp")
	}
}

func TestParseRunArgsPublishShorthand(t *testing.T) {
	r, err := ParseRunArgs([]string{"-p8080:80"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.PortBindings) != 1 {
		t.Fatalf("expected 1 port binding, got %d", len(r.PortBindings))
	}
}

func TestParseRunArgsPublishSpaceSeparatedShort(t *testing.T) {
	r, err := ParseRunArgs([]string{"-p", "8080:80"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.PortBindings) != 1 {
		t.Fatalf("expected 1 port binding, got %d", len(r.PortBindings))
	}
}

func TestParseRunArgsNetwork(t *testing.T) {
	r, err := ParseRunArgs([]string{"--network", "host"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.NetworkMode != "host" {
		t.Errorf("NetworkMode = %q, want %q", r.NetworkMode, "host")
	}
}

func TestParseRunArgsCapAddPrivileged(t *testing.T) {
	r, err := ParseRunArgs([]string{"--cap-add", "SYS_PTRACE", "--privileged"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.CapAdd) != 1 || r.CapAdd[0] != "SYS_PTRACE" {
		t.Errorf("CapAdd = %v, want [SYS_PTRACE]", r.CapAdd)
	}
	if !r.Privileged {
		t.Error("expected Privileged to be true")
	}
}

func TestParseRunArgsUnsupportedFlag(t *testing.T) {
	_, err := ParseRunArgs([]string{"--gpus", "all"})
	if err == nil {
		t.Fatal("expected error for unsupported flag")
	}
	if !strings.Contains(err.Error(), "unsupported runArg") {
		t.Errorf("error should mention unsupported runArg, got: %s", err.Error())
	}
}

func TestParseRunArgsEnv(t *testing.T) {
	r, err := ParseRunArgs([]string{"--env", "FOO=bar"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Env["FOO"] != "bar" {
		t.Errorf("Env[FOO] = %q, want %q", r.Env["FOO"], "bar")
	}
}

func TestParseRunArgsMount(t *testing.T) {
	r, err := ParseRunArgs([]string{"--mount", "type=bind,source=/host,target=/container,readonly"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.Mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(r.Mounts))
	}
	m := r.Mounts[0]
	if m.Type != mount.TypeBind {
		t.Errorf("mount.Type = %q, want %q", m.Type, mount.TypeBind)
	}
	if m.Source != "/host" {
		t.Errorf("mount.Source = %q, want %q", m.Source, "/host")
	}
	if m.Target != "/container" {
		t.Errorf("mount.Target = %q, want %q", m.Target, "/container")
	}
	if !m.ReadOnly {
		t.Error("expected mount.ReadOnly to be true")
	}
}

func TestParseRunArgsVolume(t *testing.T) {
	r, err := ParseRunArgs([]string{"-v", "/host:/container:ro"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.Binds) != 1 || r.Binds[0] != "/host:/container:ro" {
		t.Errorf("Binds = %v, want [/host:/container:ro]", r.Binds)
	}
}

func TestParseRunArgsMemory(t *testing.T) {
	r, err := ParseRunArgs([]string{"-m", "512m"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Memory != 512*1024*1024 {
		t.Errorf("Memory = %d, want %d", r.Memory, 512*1024*1024)
	}
}

func TestParseRunArgsCPUs(t *testing.T) {
	r, err := ParseRunArgs([]string{"--cpus", "1.5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.NanoCPUs != 1500000000 {
		t.Errorf("NanoCPUs = %d, want %d", r.NanoCPUs, 1500000000)
	}
}

func TestParseRunArgsInit(t *testing.T) {
	r, err := ParseRunArgs([]string{"--init"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Init == nil || !*r.Init {
		t.Error("expected Init to be true")
	}
}

func TestParseRunArgsGroupAdd(t *testing.T) {
	r, err := ParseRunArgs([]string{"--group-add", "1000"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.GroupAdd) != 1 || r.GroupAdd[0] != "1000" {
		t.Errorf("GroupAdd = %v, want [1000]", r.GroupAdd)
	}
}

func TestParseRunArgsEntrypoint(t *testing.T) {
	r, err := ParseRunArgs([]string{"--entrypoint", "/bin/sh"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.Entrypoint) != 1 || r.Entrypoint[0] != "/bin/sh" {
		t.Errorf("Entrypoint = %v, want [/bin/sh]", r.Entrypoint)
	}
}

func TestParseRunArgsHostnameWorkdir(t *testing.T) {
	r, err := ParseRunArgs([]string{"--hostname", "myhost", "-w", "/app"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Hostname != "myhost" {
		t.Errorf("Hostname = %q, want %q", r.Hostname, "myhost")
	}
	if r.WorkingDir != "/app" {
		t.Errorf("WorkingDir = %q, want %q", r.WorkingDir, "/app")
	}
}

func TestParseRunArgsReadOnly(t *testing.T) {
	r, err := ParseRunArgs([]string{"--read-only"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.ReadonlyRootfs {
		t.Error("expected ReadonlyRootfs to be true")
	}
}

func TestParseRunArgsTmpfs(t *testing.T) {
	r, err := ParseRunArgs([]string{"--tmpfs", "/tmp:size=100m"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Tmpfs["/tmp"] != "size=100m" {
		t.Errorf("Tmpfs = %v", r.Tmpfs)
	}
}

func TestParseMemoryFlag(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"512", 512},
		{"512b", 512},
		{"512k", 512 * 1024},
		{"512m", 512 * 1024 * 1024},
		{"1g", 1024 * 1024 * 1024},
		{"1.5g", int64(1.5 * 1024 * 1024 * 1024)},
	}
	for _, tt := range tests {
		got, err := parseMemoryFlag(tt.input)
		if err != nil {
			t.Fatalf("parseMemoryFlag(%q) error: %v", tt.input, err)
		}
		if got != tt.want {
			t.Errorf("parseMemoryFlag(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
