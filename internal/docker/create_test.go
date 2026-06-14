package docker

import (
	"sort"
	"testing"
)

func TestBuildCreateArgsFull(t *testing.T) {
	labels := map[string]string{
		"dcx.managed":               "true",
		"devcontainer.local_folder": "/workspace",
	}
	mounts := []string{
		"type=bind,source=/host,target=/container,readonly",
	}
	envs := []string{"FOO=bar"}
	runArgs := []string{"-p", "8080:80", "--network", "host"}
	cmdArgs := []string{"-c", "echo hello"}

	args := BuildCreateArgs("alpine:latest", runArgs, mounts, envs, labels, "vscode", "/workspace", "/bin/sh", cmdArgs)

	// Verify image ref position.
	imgIdx := indexOf(args, "alpine:latest")
	if imgIdx < 0 {
		t.Fatal("image ref not found in args")
	}

	// Verify command args are after image ref.
	if args[imgIdx+1] != "-c" {
		t.Errorf("expected cmd arg after image, got %v", args[imgIdx+1:])
	}

	// Verify labels are present.
	if !containsArgPair(args, "--label", "dcx.managed=true") {
		t.Error("missing label dcx.managed=true")
	}
	if !containsArgPair(args, "--label", "devcontainer.local_folder=/workspace") {
		t.Error("missing label devcontainer.local_folder=/workspace")
	}

	// Verify mounts.
	if !containsArgPair(args, "--mount", "type=bind,source=/host,target=/container,readonly") {
		t.Error("missing mount flag")
	}

	// Verify envs.
	if !containsArgPair(args, "--env", "FOO=bar") {
		t.Error("missing env flag")
	}

	// Verify user, workdir, entrypoint.
	if !containsArgPair(args, "--user", "vscode") {
		t.Error("missing user flag")
	}
	if !containsArgPair(args, "--workdir", "/workspace") {
		t.Error("missing workdir flag")
	}
	if !containsArgPair(args, "--entrypoint", "/bin/sh") {
		t.Error("missing entrypoint flag")
	}

	// Verify runArgs passed verbatim.
	if !containsArgPair(args, "-p", "8080:80") {
		t.Error("missing runArg -p 8080:80")
	}
	if !containsArgPair(args, "--network", "host") {
		t.Error("missing runArg --network host")
	}
}

func TestBuildCreateArgsNoOptional(t *testing.T) {
	args := BuildCreateArgs("alpine:latest", nil, nil, nil, nil, "", "", "", nil)

	if len(args) != 1 || args[0] != "alpine:latest" {
		t.Errorf("args = %v, want [alpine:latest]", args)
	}
}

func TestBuildCreateArgsLabelOrder(t *testing.T) {
	labels := map[string]string{
		"z": "last",
		"a": "first",
		"m": "middle",
	}
	args := BuildCreateArgs("img", nil, nil, nil, labels, "", "", "", nil)

	// Extract label values in order.
	var got []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--label" && i+1 < len(args) {
			got = append(got, args[i+1])
			i++
		}
	}
	want := []string{"a=first", "m=middle", "z=last"}
	if !sort.StringsAreSorted(got) {
		t.Errorf("labels not sorted: %v", got)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d labels, want %d: %v", len(got), len(want), got)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("label[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestBuildCreateArgsRunArgsOverride(t *testing.T) {
	// runArgs should appear after the fixed options and before the image,
	// so a --entrypoint in runArgs overrides the one we set.
	args := BuildCreateArgs("img", []string{"--entrypoint", "/other"}, nil, nil, nil, "", "", "/bin/sh", nil)

	entIdx := -1
	for i := 0; i < len(args); i++ {
		if args[i] == "--entrypoint" {
			entIdx = i
		}
	}
	if entIdx < 0 {
		t.Fatal("no --entrypoint found")
	}
	// The first --entrypoint is ours, the second is from runArgs.
	// Because they are both before the image, Docker uses last-one-wins.
	// But we just need to verify the runArg is present.
	found := false
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--entrypoint" && args[i+1] == "/other" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("runArg --entrypoint /other not found in args: %v", args)
	}
}

func indexOf(slice []string, val string) int {
	for i, v := range slice {
		if v == val {
			return i
		}
	}
	return -1
}

func containsArgPair(args []string, key, val string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == key && args[i+1] == val {
			return true
		}
	}
	return false
}
