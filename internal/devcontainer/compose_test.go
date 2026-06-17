package devcontainer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kobus-v-schoor/dcx/internal/devcontainer/spec"
	"github.com/kobus-v-schoor/dcx/internal/docker"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

func TestResolveComposeFilePathsAbsolute(t *testing.T) {
	cfg := &spec.Config{
		DockerComposeFile: []string{"/abs/docker-compose.yml"},
	}
	got := resolveComposeFilePaths(cfg, "/workspace")
	if len(got) != 1 || got[0] != "/abs/docker-compose.yml" {
		t.Errorf("got %v, want [/abs/docker-compose.yml]", got)
	}
}

func TestResolveComposeFilePathsRelative(t *testing.T) {
	cfg := &spec.Config{
		DockerComposeFile: []string{"../docker-compose.yml"},
	}
	got := resolveComposeFilePaths(cfg, "/workspace")
	want := filepath.Join("/workspace", ".devcontainer", "../docker-compose.yml")
	if len(got) != 1 || got[0] != want {
		t.Errorf("got %v, want [%s]", got, want)
	}
}

func TestResolveComposeFilePathsMultiple(t *testing.T) {
	cfg := &spec.Config{
		DockerComposeFile: []string{"docker-compose.yml", "/abs/override.yml"},
	}
	got := resolveComposeFilePaths(cfg, "/workspace")
	want1 := filepath.Join("/workspace", ".devcontainer", "docker-compose.yml")
	want2 := "/abs/override.yml"
	if len(got) != 2 || got[0] != want1 || got[1] != want2 {
		t.Errorf("got %v, want [%s %s]", got, want1, want2)
	}
}

func TestResolveProjectName(t *testing.T) {
	got := resolveProjectName("/home/user/my-project")
	if got != "my-project" {
		t.Errorf("got %q, want my-project", got)
	}
}

func TestBuildComposeUpArgsNoRecreate(t *testing.T) {
	args := buildComposeUpArgs("myproj", []string{"/a.yml"}, "/override.yml", "no", "app", nil)
	if slices.Contains(args, "--no-recreate") {
		t.Error("did not expect --no-recreate for 'no' policy")
	}
	if slices.Contains(args, "--force-recreate") {
		t.Error("did not expect --force-recreate")
	}
	if slices.Contains(args, "app") {
		t.Error("did not expect service names when runServices is empty")
	}
}

func TestBuildComposeUpArgsForceRecreate(t *testing.T) {
	args := buildComposeUpArgs("myproj", []string{"/a.yml"}, "/override.yml", "force", "app", []string{"db"})
	if !slices.Contains(args, "--force-recreate") {
		t.Error("expected --force-recreate in args")
	}
	if !slices.Contains(args, "app") {
		t.Error("expected app service in args")
	}
	if !slices.Contains(args, "db") {
		t.Error("expected db service in args")
	}
	// Verify -f ordering: original first, then override.
	idxA := slices.Index(args, "/a.yml")
	idxO := slices.Index(args, "/override.yml")
	if idxA == -1 || idxO == -1 || idxA >= idxO {
		t.Error("expected original -f before override -f")
	}
}

func TestBuildComposeUpArgsProjectName(t *testing.T) {
	args := buildComposeUpArgs("myproj", []string{"/a.yml"}, "", "", "app", nil)
	idxP := slices.Index(args, "-p")
	if idxP == -1 || idxP+1 >= len(args) || args[idxP+1] != "myproj" {
		t.Error("expected -p myproj")
	}
}

func TestMountEntryToComposeVolumePlain(t *testing.T) {
	got, err := mountEntryToComposeVolume("type=bind,source=/host,target=/container")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if got.Type != "bind" {
		t.Errorf("got Type %q, want bind", got.Type)
	}
	if got.Source != "/host" {
		t.Errorf("got Source %q, want /host", got.Source)
	}
	if got.Target != "/container" {
		t.Errorf("got Target %q, want /container", got.Target)
	}
}

func TestMountEntryToComposeVolumeReadonly(t *testing.T) {
	got, err := mountEntryToComposeVolume("type=bind,source=/host,target=/container,readonly")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !got.ReadOnly {
		t.Error("expected ReadOnly to be true")
	}
}

func TestMountEntryToComposeVolumeUnsupportedOption(t *testing.T) {
	got, err := mountEntryToComposeVolume("type=bind,source=/host,target=/container,mode=777")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if got.ReadOnly || got.Consistency != "" || got.Bind != nil {
		t.Error("unsupported option should not affect volume")
	}
}

func TestBuildComposeLabelsSorted(t *testing.T) {
	cfg := &spec.Config{
		RemoteUser:      "vscode",
		WorkspaceFolder: "/workspace",
	}
	labels := buildComposeLabels(cfg, "/workspace")
	if len(labels) == 0 {
		t.Fatal("expected labels")
	}
	// Verify sorted order (lexicographically).
	for i := 1; i < len(labels); i++ {
		if labels[i] < labels[i-1] {
			t.Errorf("labels not sorted: %v", labels)
		}
	}
	found := false
	for _, l := range labels {
		if strings.HasPrefix(l, "devcontainer.local_folder=") {
			found = true
		}
	}
	if !found {
		t.Error("missing devcontainer.local_folder label")
	}
}

func TestBuildComposeMetadataJSONOverrideCommandAlwaysTrue(t *testing.T) {
	f := false
	cfg := &spec.Config{
		RemoteUser:      "vscode",
		WorkspaceFolder: "/workspace",
		OverrideCommand: &f,
	}
	jsonStr, err := buildComposeMetadataJSON(cfg)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !strings.Contains(jsonStr, `"overrideCommand":true`) {
		t.Errorf("expected overrideCommand:true, got %s", jsonStr)
	}
}

func TestWriteComposeOverride(t *testing.T) {
	cfg := &spec.Config{
		Service:         "app",
		WorkspaceFolder: "/workspace",
		ContainerEnv:    map[string]string{"FOO": "bar"},
		Mounts: []spec.MountEntry{
			spec.NewMountEntryString("type=bind,source=/host/mnt,target=/container/mnt,readonly"),
		},
	}
	path := filepath.Join(t.TempDir(), "override.yml")
	if err := writeComposeOverride(cfg, path, "/workspace", ""); err != nil {
		t.Fatalf("writeComposeOverride error: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading override: %v", err)
	}
	s := string(content)

	if !strings.Contains(s, "services:") {
		t.Error("missing services section")
	}
	if !strings.Contains(s, "app:") {
		t.Error("missing app service")
	}
	if !strings.Contains(s, "labels:") {
		t.Error("missing labels section")
	}
	if !strings.Contains(s, "environment:") {
		t.Error("missing environment section")
	}
	if !strings.Contains(s, "FOO=bar") {
		t.Error("missing FOO=bar environment")
	}
	if !strings.Contains(s, "volumes:") {
		t.Error("missing volumes section")
	}
	if !strings.Contains(s, "entrypoint:") {
		t.Error("missing entrypoint section")
	}
	if !strings.Contains(s, "/bin/sh") {
		t.Error("missing /bin/sh entrypoint")
	}
	if !strings.Contains(s, "read_only: true") {
		t.Error("missing read_only for mount")
	}
}

// composeCapture records arguments passed to the mocked composeUpRunner.
type composeCapture struct {
	called bool
	args   []string
}

func mockComposeRunner(t *testing.T) *composeCapture {
	cap := &composeCapture{}
	orig := composeUpRunner
	composeUpRunner = func(_ context.Context, args []string) error {
		cap.called = true
		cap.args = append([]string(nil), args...)
		return nil
	}
	t.Cleanup(func() { composeUpRunner = orig })
	return cap
}

func TestUpComposeReuseRunningContainer(t *testing.T) {
	cap := mockComposeRunner(t)

	cli := &mockClient{
		listResult: client.ContainerListResult{
			Items: []container.Summary{{ID: "running123", State: container.StateRunning}},
		},
	}
	cfg := &spec.Config{
		Service:           "app",
		WorkspaceFolder:   "/workspace",
		DockerComposeFile: []string{"docker-compose.yml"},
	}

	id, err := UpCompose(context.Background(), cli, cfg, "/workspace", false, false)
	if err != nil {
		t.Fatalf("UpCompose error: %v", err)
	}
	if id != "running123" {
		t.Errorf("id = %q, want running123", id)
	}
	if cap.called {
		t.Error("expected composeUpRunner NOT to be called")
	}
}

func TestUpComposeStartStoppedContainer(t *testing.T) {
	cap := mockComposeRunner(t)

	cli := &mockClient{
		listResult: client.ContainerListResult{
			Items: []container.Summary{{ID: "stopped456", State: container.StateExited}},
		},
	}
	cfg := &spec.Config{
		Service:           "app",
		WorkspaceFolder:   "/workspace",
		DockerComposeFile: []string{"docker-compose.yml"},
	}

	id, err := UpCompose(context.Background(), cli, cfg, "/workspace", false, false)
	if err != nil {
		t.Fatalf("UpCompose error: %v", err)
	}
	if id != "stopped456" {
		t.Errorf("id = %q, want stopped456", id)
	}
	if !cap.called {
		t.Fatal("expected composeUpRunner to be called")
	}
	if slices.Contains(cap.args, "--no-recreate") {
		t.Error("did not expect --no-recreate")
	}
	if slices.Contains(cap.args, "--force-recreate") {
		t.Error("did not expect --force-recreate")
	}
	// Verify the temporary override file is included in the args.
	foundOverride := false
	for i, a := range cap.args {
		if a == "-f" && i+1 < len(cap.args) && strings.Contains(cap.args[i+1], "dcx.compose.override.yml") {
			foundOverride = true
			break
		}
	}
	if !foundOverride {
		t.Error("expected override file in args")
	}
}

func TestUpComposeRebuild(t *testing.T) {
	cap := mockComposeRunner(t)
	pcap := mockPostCreateRunner(t)

	cli := &mockClient{
		listResult: client.ContainerListResult{
			Items: []container.Summary{{ID: "old789", State: container.StateRunning}},
		},
	}
	cfg := &spec.Config{
		Service:           "app",
		WorkspaceFolder:   "/workspace",
		DockerComposeFile: []string{"docker-compose.yml"},
	}

	id, err := UpCompose(context.Background(), cli, cfg, "/workspace", true, false)
	if err != nil {
		t.Fatalf("UpCompose error: %v", err)
	}
	if id != "old789" {
		t.Errorf("id = %q, want old789", id)
	}
	if !cap.called {
		t.Fatal("expected composeUpRunner to be called")
	}
	if !slices.Contains(cap.args, "--force-recreate") {
		t.Error("expected --force-recreate")
	}
	if !pcap.called {
		t.Error("expected postCreateRunner to be called for rebuild")
	}
}

func TestWriteComposeOverrideWithImage(t *testing.T) {
	cfg := &spec.Config{
		Service:         "app",
		WorkspaceFolder: "/workspace",
	}
	path := filepath.Join(t.TempDir(), "override.yml")
	if err := writeComposeOverride(cfg, path, "/workspace", "dcx-test-feat:abc123"); err != nil {
		t.Fatalf("writeComposeOverride error: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading override: %v", err)
	}
	s := string(content)
	if !strings.Contains(s, "image: dcx-test-feat:abc123") {
		t.Errorf("expected image override in compose file, got:\n%s", s)
	}
}

// mockComposeConfigRunner overrides composeConfigRunner for the duration of a test.
func mockComposeConfigRunner(t *testing.T, output []byte) {
	orig := composeConfigRunner
	composeConfigRunner = func(_ context.Context, _ string, _ []string) ([]byte, error) {
		return append([]byte(nil), output...), nil
	}
	t.Cleanup(func() { composeConfigRunner = orig })
}

// mockComposeFeatureImageBuilder overrides composeFeatureImageBuilder for tests.
func mockComposeFeatureImageBuilder(t *testing.T, result string) {
	orig := composeFeatureImageBuilder
	composeFeatureImageBuilder = func(_ context.Context, _ docker.DockerClient, _ string, _ map[string]json.RawMessage, _, _, _ string, _, _ bool) (string, error) {
		return result, nil
	}
	t.Cleanup(func() { composeFeatureImageBuilder = orig })
}

// mockComposeDockerfileBuilder overrides composeDockerfileBuilder for tests.
func mockComposeDockerfileBuilder(t *testing.T, result string) {
	orig := composeDockerfileBuilder
	composeDockerfileBuilder = func(_ context.Context, _ docker.DockerClient, _ *spec.Config, _ string, _ bool) (string, error) {
		return result, nil
	}
	t.Cleanup(func() { composeDockerfileBuilder = orig })
}

func TestUpComposeWithFeatures(t *testing.T) {
	var overrideContent []byte

	orig := composeUpRunner
	composeUpRunner = func(_ context.Context, args []string) error {
		// Capture the override file contents before UpCompose deletes the temp dir.
		for i, a := range args {
			if a == "-f" && i+1 < len(args) && strings.Contains(args[i+1], "dcx.compose.override.yml") {
				overrideContent, _ = os.ReadFile(args[i+1])
				break
			}
		}
		return nil
	}
	t.Cleanup(func() { composeUpRunner = orig })

	configJSON := []byte(`{"services":{"app":{"image":"base:local"}}}`)
	mockComposeConfigRunner(t, configJSON)
	mockComposeFeatureImageBuilder(t, "dcx-compose-feat:abc123")

	cli := &mockClient{
		listResult: client.ContainerListResult{
			Items: []container.Summary{{ID: "stopped456", State: container.StateExited}},
		},
	}
	cfg := &spec.Config{
		Service:           "app",
		WorkspaceFolder:   "/workspace",
		DockerComposeFile: []string{"docker-compose.yml"},
		Features: map[string]json.RawMessage{
			"ghcr.io/devcontainers/features/common-utils:1": {},
		},
	}

	id, err := UpCompose(context.Background(), cli, cfg, "/workspace", false, false)
	if err != nil {
		t.Fatalf("UpCompose error: %v", err)
	}
	if id != "stopped456" {
		t.Errorf("id = %q, want stopped456", id)
	}
	if overrideContent == nil {
		t.Fatal("expected override file to be captured")
	}
	if !strings.Contains(string(overrideContent), "image: dcx-compose-feat:abc123") {
		t.Errorf("expected feature image in override, got:\n%s", string(overrideContent))
	}
}

func TestResolveComposeBaseImageImageService(t *testing.T) {
	configJSON := []byte(`{"services":{"app":{"image":"alpine:latest"}}}`)
	mockComposeConfigRunner(t, configJSON)

	cli := &mockClient{}
	got, err := resolveComposeBaseImage(context.Background(), cli, "proj", []string{"/a.yml"}, "app", "/workspace", false)
	if err != nil {
		t.Fatalf("resolveComposeBaseImage error: %v", err)
	}
	if got != "alpine:latest" {
		t.Errorf("got %q, want alpine:latest", got)
	}
}

func TestResolveComposeBaseImageBuildService(t *testing.T) {
	configJSON := []byte(`{"services":{"app":{"build":{"context":"/workspace","dockerfile":"Dockerfile"}}}}`)
	mockComposeConfigRunner(t, configJSON)
	mockComposeDockerfileBuilder(t, "dcx-built:1234")

	cli := &mockClient{}
	got, err := resolveComposeBaseImage(context.Background(), cli, "proj", []string{"/a.yml"}, "app", "/workspace", false)
	if err != nil {
		t.Fatalf("resolveComposeBaseImage error: %v", err)
	}
	if got != "dcx-built:1234" {
		t.Errorf("got %q, want dcx-built:1234", got)
	}
}

func TestResolveComposeBaseImageMissingService(t *testing.T) {
	configJSON := []byte(`{"services":{"other":{"image":"alpine:latest"}}}`)
	mockComposeConfigRunner(t, configJSON)

	cli := &mockClient{}
	_, err := resolveComposeBaseImage(context.Background(), cli, "proj", []string{"/a.yml"}, "app", "/workspace", false)
	if err == nil {
		t.Fatal("expected error for missing service")
	}
}

// Test that resolveComposeBaseImage passes absolute build contexts from
// docker compose config through to the Dockerfile builder without
// duplicating the path.
func TestResolveComposeBaseImageBuildServiceAbsoluteContext(t *testing.T) {
	tmpDir := t.TempDir()
	devDir := filepath.Join(tmpDir, ".devcontainer")
	if err := os.MkdirAll(devDir, 0755); err != nil {
		t.Fatalf("creating .devcontainer dir: %v", err)
	}
	dockerfile := filepath.Join(devDir, "Dockerfile")
	if err := os.WriteFile(dockerfile, []byte("FROM alpine:3.19\n"), 0644); err != nil {
		t.Fatalf("writing Dockerfile: %v", err)
	}

	// Compose config returns an absolute build context (as docker compose
	// config does when it resolves paths).
	configJSON := []byte(fmt.Sprintf(`{"services":{"app":{"build":{"context":%q,"dockerfile":"Dockerfile"}}}}`, devDir))
	mockComposeConfigRunner(t, configJSON)

	// Restore the real buildFromDockerfile so we can verify the context path.
	origBuilder := composeDockerfileBuilder
	composeDockerfileBuilder = buildFromDockerfile
	t.Cleanup(func() { composeDockerfileBuilder = origBuilder })

	// Capture the build context directory passed to the Docker CLI builder.
	origImageBuild := imageBuildFromDirCLI
	var capturedCtx string
	imageBuildFromDirCLI = func(_ context.Context, buildCtx string, _ docker.ImageBuildOptions) (string, error) {
		capturedCtx = buildCtx
		return "sha256:built123", nil
	}
	t.Cleanup(func() { imageBuildFromDirCLI = origImageBuild })

	cli := &localMockClient{
		inspectResponses: []inspectResponse{
			{err: fmt.Errorf("not found")},
		},
	}

	_, err := resolveComposeBaseImage(context.Background(), cli, "proj", []string{"/a.yml"}, "app", tmpDir, false)
	if err != nil {
		t.Fatalf("resolveComposeBaseImage error: %v", err)
	}
	if capturedCtx != devDir {
		t.Errorf("expected build context dir=%q, got %q", devDir, capturedCtx)
	}
}
