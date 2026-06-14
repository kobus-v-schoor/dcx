package devcontainer

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kobus-v-schoor/dcx/internal/devcontainer/spec"
)

func TestSubstituteAllBasic(t *testing.T) {
	tmp := t.TempDir()
	cfg := &spec.Config{
		WorkspaceFolder: "${localWorkspaceFolder}",
		WorkspaceMount:  "source=${localWorkspaceFolder},target=/workspace,type=bind",
		ContainerEnv: map[string]string{
			"HOST_PATH": "${localWorkspaceFolder}",
		},
		Mounts: []spec.MountEntry{
			spec.NewMountEntryString("source=${localWorkspaceFolder}/sub,target=/sub,type=bind"),
		},
		RunArgs: []string{"--env", "PATH=${localWorkspaceFolder}/bin"},
	}

	if err := SubstituteAll(cfg, tmp); err != nil {
		t.Fatalf("SubstituteAll error: %v", err)
	}

	abs, _ := filepath.Abs(tmp)
	if cfg.WorkspaceFolder != abs {
		t.Errorf("WorkspaceFolder = %q, want %q", cfg.WorkspaceFolder, abs)
	}
	wantMount := "source=" + abs + ",target=/workspace,type=bind"
	if cfg.WorkspaceMount != wantMount {
		t.Errorf("WorkspaceMount = %q, want %q", cfg.WorkspaceMount, wantMount)
	}
	if cfg.ContainerEnv["HOST_PATH"] != abs {
		t.Errorf("ContainerEnv[HOST_PATH] = %q, want %q", cfg.ContainerEnv["HOST_PATH"], abs)
	}
	if s, _ := cfg.Mounts[0].AsString(); !strings.Contains(s, abs) {
		t.Errorf("Mount did not contain abs path: %q", s)
	}
	want := "PATH=" + abs + "/bin"
	if cfg.RunArgs[1] != want {
		t.Errorf("RunArgs[1] = %q, want %q", cfg.RunArgs[1], want)
	}
}

func TestSubstituteBasename(t *testing.T) {
	tmp := t.TempDir()
	cfg := &spec.Config{
		WorkspaceFolder: "/workspace",
		ContainerEnv: map[string]string{
			"LOCAL_BASE":     "${localWorkspaceFolderBasename}",
			"CONTAINER_BASE": "${containerWorkspaceFolderBasename}",
		},
	}

	if err := SubstituteAll(cfg, tmp); err != nil {
		t.Fatalf("SubstituteAll error: %v", err)
	}

	wantLocalBase := filepath.Base(tmp)
	if cfg.ContainerEnv["LOCAL_BASE"] != wantLocalBase {
		t.Errorf("LOCAL_BASE = %q, want %q", cfg.ContainerEnv["LOCAL_BASE"], wantLocalBase)
	}
	// containerFolder is /workspace, whose basename is "workspace"
	if cfg.ContainerEnv["CONTAINER_BASE"] != "workspace" {
		t.Errorf("CONTAINER_BASE = %q, want %q", cfg.ContainerEnv["CONTAINER_BASE"], wantLocalBase)
	}
}

func TestSubstituteLocalEnv(t *testing.T) {
	t.Setenv("DCX_TEST_VAR", "hello")

	cfg := &spec.Config{
		ContainerEnv: map[string]string{
			"FROM_ENV":         "${localEnv:DCX_TEST_VAR}",
			"WITH_DEFAULT_SET": "${localEnv:DCX_TEST_VAR:unused}",
			"MISSING":          "${localEnv:DCX_MISSING_VAR}",
			"MISSING_DEFAULT":  "${localEnv:DCX_MISSING_VAR:fallback}",
		},
	}

	if err := SubstituteAll(cfg, "/tmp/workspace"); err != nil {
		t.Fatalf("SubstituteAll error: %v", err)
	}

	if cfg.ContainerEnv["FROM_ENV"] != "hello" {
		t.Errorf("FROM_ENV = %q, want %q", cfg.ContainerEnv["FROM_ENV"], "hello")
	}
	if cfg.ContainerEnv["WITH_DEFAULT_SET"] != "hello" {
		t.Errorf("WITH_DEFAULT_SET = %q, want %q", cfg.ContainerEnv["WITH_DEFAULT_SET"], "hello")
	}
	if cfg.ContainerEnv["MISSING"] != "" {
		t.Errorf("MISSING = %q, want empty", cfg.ContainerEnv["MISSING"])
	}
	if cfg.ContainerEnv["MISSING_DEFAULT"] != "fallback" {
		t.Errorf("MISSING_DEFAULT = %q, want %q", cfg.ContainerEnv["MISSING_DEFAULT"], "fallback")
	}
}

func TestSubstituteDevcontainerId(t *testing.T) {
	cfg := &spec.Config{
		ContainerEnv: map[string]string{
			"ID": "${devcontainerId}",
		},
	}

	if err := SubstituteAll(cfg, "/tmp/workspace"); err != nil {
		t.Fatalf("SubstituteAll error: %v", err)
	}

	id := cfg.ContainerEnv["ID"]
	if len(id) != 32 {
		t.Errorf("devcontainerId length = %d, want 32", len(id))
	}
}

func TestSubstituteUnknownPattern(t *testing.T) {
	cfg := &spec.Config{
		ContainerEnv: map[string]string{
			"UNKNOWN": "${unknownPattern}",
		},
	}

	if err := SubstituteAll(cfg, "/tmp/workspace"); err != nil {
		t.Fatalf("SubstituteAll error: %v", err)
	}

	if cfg.ContainerEnv["UNKNOWN"] != "${unknownPattern}" {
		t.Errorf("UNKNOWN = %q, want unchanged", cfg.ContainerEnv["UNKNOWN"])
	}
}

func TestSubstituteAllNoFields(t *testing.T) {
	cfg := &spec.Config{}
	if err := SubstituteAll(cfg, "/tmp/workspace"); err != nil {
		t.Fatalf("SubstituteAll error: %v", err)
	}
}
