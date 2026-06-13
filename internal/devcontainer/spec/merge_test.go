package spec

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestMergeNilBaseNilOverride(t *testing.T) {
	got := Merge(nil, nil)
	if got == nil {
		t.Fatal("expected non-nil Config")
	}
	if got.Image != "" {
		t.Error("expected empty merged config")
	}
}

func TestMergeNilBaseNonNilOverride(t *testing.T) {
	override := &Config{Image: "override:latest"}
	got := Merge(nil, override)
	if got.Image != "override:latest" {
		t.Errorf("Image = %q, want override:latest", got.Image)
	}
}

func TestMergeNonNilBaseNilOverride(t *testing.T) {
	base := &Config{Image: "base:latest"}
	got := Merge(base, nil)
	if got.Image != "base:latest" {
		t.Errorf("Image = %q, want base:latest", got.Image)
	}
}

func TestMergeScalarFields(t *testing.T) {
	base := &Config{
		Name:             "base",
		Image:            "base:latest",
		Service:          "base-svc",
		WorkspaceFolder:  "/base",
		WorkspaceMount:   "base-mount",
		RemoteUser:       "base-user",
		ContainerUser:    "root",
		ShutdownAction:   "none",
		LegacyDockerfile: "Dockerfile.base",
	}
	override := &Config{
		Name:             "override",
		Image:            "override:latest",
		Service:          "override-svc",
		WorkspaceFolder:  "/override",
		WorkspaceMount:   "override-mount",
		RemoteUser:       "override-user",
		ContainerUser:    "vscode",
		ShutdownAction:   "stopContainer",
		LegacyDockerfile: "Dockerfile.override",
	}
	got := Merge(base, override)

	if got.Name != "override" {
		t.Errorf("Name = %q, want override", got.Name)
	}
	if got.Image != "override:latest" {
		t.Errorf("Image = %q, want override:latest", got.Image)
	}
	if got.Service != "override-svc" {
		t.Errorf("Service = %q, want override-svc", got.Service)
	}
	if got.WorkspaceFolder != "/override" {
		t.Errorf("WorkspaceFolder = %q, want /override", got.WorkspaceFolder)
	}
	if got.WorkspaceMount != "override-mount" {
		t.Errorf("WorkspaceMount = %q, want override-mount", got.WorkspaceMount)
	}
	if got.RemoteUser != "override-user" {
		t.Errorf("RemoteUser = %q, want override-user", got.RemoteUser)
	}
	if got.ContainerUser != "vscode" {
		t.Errorf("ContainerUser = %q, want vscode", got.ContainerUser)
	}
	if got.ShutdownAction != "stopContainer" {
		t.Errorf("ShutdownAction = %q, want stopContainer", got.ShutdownAction)
	}
	if got.LegacyDockerfile != "Dockerfile.override" {
		t.Errorf("LegacyDockerfile = %q, want Dockerfile.override", got.LegacyDockerfile)
	}
}

func TestMergeScalarEmptyOverrideDoesNotReplace(t *testing.T) {
	base := &Config{Image: "base:latest", Name: "base-name"}
	override := &Config{Image: ""}
	got := Merge(base, override)
	if got.Image != "base:latest" {
		t.Errorf("Image = %q, want base:latest", got.Image)
	}
	if got.Name != "base-name" {
		t.Errorf("Name = %q, want base-name", got.Name)
	}
}

func TestMergeBuild(t *testing.T) {
	base := &Config{Build: &Build{Dockerfile: "Dockerfile.base", Context: "."}}
	override := &Config{Build: &Build{Dockerfile: "Dockerfile.override"}}
	got := Merge(base, override)

	if got.Build == nil {
		t.Fatal("expected Build to be non-nil")
	}
	if got.Build.Dockerfile != "Dockerfile.override" {
		t.Errorf("Build.Dockerfile = %q, want Dockerfile.override", got.Build.Dockerfile)
	}
	if got.Build.Context != "" {
		t.Errorf("Build.Context = %q, want empty (replaced entirely)", got.Build.Context)
	}
}

func TestMergeDockerComposeFile(t *testing.T) {
	base := &Config{DockerComposeFile: []string{"a.yml"}}
	override := &Config{DockerComposeFile: []string{"b.yml", "c.yml"}}
	got := Merge(base, override)
	want := []string{"b.yml", "c.yml"}
	if !reflect.DeepEqual(got.DockerComposeFile, want) {
		t.Errorf("DockerComposeFile = %v, want %v", got.DockerComposeFile, want)
	}
}

func TestMergeRunServices(t *testing.T) {
	base := &Config{RunServices: []string{"app"}}
	override := &Config{RunServices: []string{"app", "db"}}
	got := Merge(base, override)
	want := []string{"app", "db"}
	if !reflect.DeepEqual(got.RunServices, want) {
		t.Errorf("RunServices = %v, want %v", got.RunServices, want)
	}
}

func TestMergeContainerEnv(t *testing.T) {
	base := &Config{ContainerEnv: map[string]string{"A": "a", "B": "b"}}
	override := &Config{ContainerEnv: map[string]string{"B": "override-b", "C": "c"}}
	got := Merge(base, override)

	if got.ContainerEnv == nil || len(got.ContainerEnv) != 3 {
		t.Fatalf("expected 3 env vars, got %v", got.ContainerEnv)
	}
	if got.ContainerEnv["A"] != "a" {
		t.Errorf("ContainerEnv[A] = %q, want a", got.ContainerEnv["A"])
	}
	if got.ContainerEnv["B"] != "override-b" {
		t.Errorf("ContainerEnv[B] = %q, want override-b", got.ContainerEnv["B"])
	}
	if got.ContainerEnv["C"] != "c" {
		t.Errorf("ContainerEnv[C] = %q, want c", got.ContainerEnv["C"])
	}
}

func TestMergeRemoteEnv(t *testing.T) {
	base := &Config{RemoteEnv: map[string]string{"X": "x"}}
	override := &Config{RemoteEnv: map[string]string{"Y": "y"}}
	got := Merge(base, override)
	if got.RemoteEnv == nil || len(got.RemoteEnv) != 2 {
		t.Fatalf("expected 2 remote env vars, got %v", got.RemoteEnv)
	}
}

func TestMergeFeatures(t *testing.T) {
	base := &Config{Features: map[string]json.RawMessage{
		"feat-a": json.RawMessage(`{}`),
		"feat-b": json.RawMessage(`{"version":"1"}`),
	}}
	override := &Config{Features: map[string]json.RawMessage{
		"feat-b": json.RawMessage(`{"version":"2"}`),
		"feat-c": json.RawMessage(`{}`),
	}}
	got := Merge(base, override)

	if len(got.Features) != 3 {
		t.Fatalf("expected 3 features, got %d", len(got.Features))
	}
	if string(got.Features["feat-b"]) != `{"version":"2"}` {
		t.Errorf("feat-b = %s, want {\"version\":\"2\"}", string(got.Features["feat-b"]))
	}
	if string(got.Features["feat-c"]) != `{}` {
		t.Errorf("feat-c = %s", string(got.Features["feat-c"]))
	}
}

func TestMergePortsAttributes(t *testing.T) {
	base := &Config{PortsAttributes: map[string]json.RawMessage{
		"8080": json.RawMessage(`{"label":"app"}`),
	}}
	override := &Config{PortsAttributes: map[string]json.RawMessage{
		"8080": json.RawMessage(`{"label":"web"}`),
	}}
	got := Merge(base, override)
	if string(got.PortsAttributes["8080"]) != `{"label":"web"}` {
		t.Errorf("portsAttributes[8080] = %s", string(got.PortsAttributes["8080"]))
	}
}

func TestMergeMountsConcatenated(t *testing.T) {
	base := &Config{Mounts: []MountEntry{NewMountEntryString("type=bind,source=/a,target=/a")}}
	override := &Config{Mounts: []MountEntry{NewMountEntryString("type=bind,source=/b,target=/b")}}
	got := Merge(base, override)
	if len(got.Mounts) != 2 {
		t.Fatalf("expected 2 mounts, got %d", len(got.Mounts))
	}
	if s, _ := got.Mounts[0].AsString(); s != "type=bind,source=/a,target=/a" {
		t.Errorf("Mounts[0] = %q", s)
	}
	if s, _ := got.Mounts[1].AsString(); s != "type=bind,source=/b,target=/b" {
		t.Errorf("Mounts[1] = %q", s)
	}
}

func TestMergeMountsNilBase(t *testing.T) {
	override := &Config{Mounts: []MountEntry{NewMountEntryString("type=bind,source=/b,target=/b")}}
	got := Merge(nil, override)
	if len(got.Mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(got.Mounts))
	}
}

func TestMergeMountsNilOverride(t *testing.T) {
	base := &Config{Mounts: []MountEntry{NewMountEntryString("type=bind,source=/a,target=/a")}}
	got := Merge(base, nil)
	if len(got.Mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(got.Mounts))
	}
}

func TestMergeLifecycleCommands(t *testing.T) {
	base := &Config{
		PostCreateCommand: NewLifecycleCommandString("base-create"),
		PostStartCommand:  NewLifecycleCommandString("base-start"),
	}
	override := &Config{
		PostCreateCommand: NewLifecycleCommandString("override-create"),
		PostAttachCommand: NewLifecycleCommandString("override-attach"),
	}
	got := Merge(base, override)

	if s, _ := got.PostCreateCommand.AsString(); s != "override-create" {
		t.Errorf("PostCreateCommand = %q, want override-create", s)
	}
	if s, _ := got.PostStartCommand.AsString(); s != "base-start" {
		t.Errorf("PostStartCommand = %q, want base-start", s)
	}
	if s, _ := got.PostAttachCommand.AsString(); s != "override-attach" {
		t.Errorf("PostAttachCommand = %q, want override-attach", s)
	}
	if !got.InitializeCommand.IsEmpty() {
		t.Errorf("InitializeCommand should be empty")
	}
}

func TestMergeRunArgs(t *testing.T) {
	base := &Config{RunArgs: []string{"--cap-add=SYS_PTRACE"}}
	override := &Config{RunArgs: []string{"--privileged"}}
	got := Merge(base, override)
	want := []string{"--privileged"}
	if !reflect.DeepEqual(got.RunArgs, want) {
		t.Errorf("RunArgs = %v, want %v", got.RunArgs, want)
	}
}

func TestMergeForwardPorts(t *testing.T) {
	base := &Config{ForwardPorts: []ForwardPort{NewForwardPortInt(8080)}}
	override := &Config{ForwardPorts: []ForwardPort{NewForwardPortInt(3000), NewForwardPortString("db:5432")}}
	got := Merge(base, override)
	want := []ForwardPort{NewForwardPortInt(3000), NewForwardPortString("db:5432")}
	if len(got.ForwardPorts) != len(want) {
		t.Fatalf("ForwardPorts length = %d, want %d", len(got.ForwardPorts), len(want))
	}
	for i := range want {
		if got.ForwardPorts[i].String() != want[i].String() {
			t.Errorf("ForwardPorts[%d] = %v, want %v", i, got.ForwardPorts[i], want[i])
		}
	}
}

func TestMergePointerFields(t *testing.T) {
	trueVal := true
	falseVal := false

	base := &Config{OverrideCommand: &trueVal, UpdateRemoteUserUID: &trueVal}
	override := &Config{OverrideCommand: &falseVal}
	got := Merge(base, override)

	if got.OverrideCommand == nil || *got.OverrideCommand != false {
		t.Error("OverrideCommand should be false from override")
	}
	if got.UpdateRemoteUserUID == nil || *got.UpdateRemoteUserUID != true {
		t.Error("UpdateRemoteUserUID should remain true from base")
	}
}

func TestMergeDoesNotMutateInput(t *testing.T) {
	base := &Config{
		Image:        "base:latest",
		ContainerEnv: map[string]string{"KEY": "base"},
		Mounts:       []MountEntry{NewMountEntryString("mount-a")},
	}
	baseSnapshot := deepCopy(base)

	override := &Config{
		Image:        "override:latest",
		ContainerEnv: map[string]string{"KEY": "override"},
		Mounts:       []MountEntry{NewMountEntryString("mount-b")},
	}
	overrideSnapshot := deepCopy(override)

	_ = Merge(base, override)

	if !reflect.DeepEqual(base, baseSnapshot) {
		t.Error("Merge mutated base config")
	}
	if !reflect.DeepEqual(override, overrideSnapshot) {
		t.Error("Merge mutated override config")
	}
}

func TestMergeEmptyMaps(t *testing.T) {
	base := &Config{Image: "base:latest"}
	override := &Config{Image: "override:latest"}
	got := Merge(base, override)
	if got.ContainerEnv != nil {
		t.Error("expected nil ContainerEnv when both sides are empty")
	}
	if got.Features != nil {
		t.Error("expected nil Features when both sides are empty")
	}
	if got.PortsAttributes != nil {
		t.Error("expected nil PortsAttributes when both sides are empty")
	}
}
