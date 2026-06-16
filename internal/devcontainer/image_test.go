package devcontainer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kobus-v-schoor/dcx/internal/devcontainer/features"
	"github.com/kobus-v-schoor/dcx/internal/devcontainer/spec"
	"github.com/kobus-v-schoor/dcx/internal/docker"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/api/types/jsonstream"
	"github.com/moby/moby/client"
)

// mockImagePullResponse implements client.ImagePullResponse for testing.
type mockImagePullResponse struct {
	io.ReadCloser
	waitErr error
}

func (m *mockImagePullResponse) JSONMessages(_ context.Context) iter.Seq2[jsonstream.Message, error] {
	return func(yield func(jsonstream.Message, error) bool) {}
}

func (m *mockImagePullResponse) Wait(_ context.Context) error {
	return m.waitErr
}

// inspectResponse pairs a result with an error for ordered mock responses.
type inspectResponse struct {
	result client.ImageInspectResult
	err    error
}

// localMockClient satisfies docker.DockerClient for testing BuildImage.
// Only ImageInspect and ImagePull are stateful; the rest return zero values.
type localMockClient struct {
	inspectCallCount int
	inspectResponses []inspectResponse
	imagePullResult  client.ImagePullResponse
	imagePullErr     error
	imageTagErr      error
	imageListResult  client.ImageListResult
	imageListErr     error
}

func (m *localMockClient) Ping(_ context.Context, _ client.PingOptions) (client.PingResult, error) {
	return client.PingResult{}, nil
}

func (m *localMockClient) ContainerList(_ context.Context, _ client.ContainerListOptions) (client.ContainerListResult, error) {
	return client.ContainerListResult{}, nil
}

func (m *localMockClient) ContainerInspect(_ context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	return client.ContainerInspectResult{}, nil
}

func (m *localMockClient) ContainerStop(_ context.Context, _ string, _ client.ContainerStopOptions) (client.ContainerStopResult, error) {
	return client.ContainerStopResult{}, nil
}

func (m *localMockClient) ContainerRemove(_ context.Context, _ string, _ client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
	return client.ContainerRemoveResult{}, nil
}

func (m *localMockClient) ImageRemove(_ context.Context, _ string, _ client.ImageRemoveOptions) (client.ImageRemoveResult, error) {
	return client.ImageRemoveResult{}, nil
}

func (m *localMockClient) VolumeRemove(_ context.Context, _ string, _ client.VolumeRemoveOptions) (client.VolumeRemoveResult, error) {
	return client.VolumeRemoveResult{}, nil
}

func (m *localMockClient) CopyToContainer(_ context.Context, _ string, _ client.CopyToContainerOptions) (client.CopyToContainerResult, error) {
	return client.CopyToContainerResult{}, nil
}

func (m *localMockClient) ExecCreate(_ context.Context, _ string, _ client.ExecCreateOptions) (client.ExecCreateResult, error) {
	return client.ExecCreateResult{ID: "exec123"}, nil
}

func (m *localMockClient) ExecAttach(_ context.Context, _ string, _ client.ExecAttachOptions) (client.ExecAttachResult, error) {
	return client.ExecAttachResult{}, nil
}

func (m *localMockClient) ExecStart(_ context.Context, _ string, _ client.ExecStartOptions) (client.ExecStartResult, error) {
	return client.ExecStartResult{}, nil
}

func (m *localMockClient) ExecInspect(_ context.Context, _ string, _ client.ExecInspectOptions) (client.ExecInspectResult, error) {
	return client.ExecInspectResult{}, nil
}

func (m *localMockClient) Close() error {
	return nil
}

func (m *localMockClient) ImagePull(_ context.Context, _ string, _ client.ImagePullOptions) (client.ImagePullResponse, error) {
	return m.imagePullResult, m.imagePullErr
}

func (m *localMockClient) ImageInspect(_ context.Context, _ string, _ ...client.ImageInspectOption) (client.ImageInspectResult, error) {
	idx := m.inspectCallCount
	m.inspectCallCount++
	if idx >= len(m.inspectResponses) {
		return client.ImageInspectResult{}, fmt.Errorf("unexpected ImageInspect call %d", idx)
	}
	r := m.inspectResponses[idx]
	return r.result, r.err
}

func (m *localMockClient) ImageTag(_ context.Context, _ client.ImageTagOptions) (client.ImageTagResult, error) {
	return client.ImageTagResult{}, m.imageTagErr
}

func (m *localMockClient) ImageList(_ context.Context, _ client.ImageListOptions) (client.ImageListResult, error) {
	return m.imageListResult, m.imageListErr
}

// captureImageBuild overrides imageBuildFromDirCLI for the duration of the
// test and records the options passed to it. The returned function returns
// the captured options (or nil if the builder was not called).
func captureImageBuild(t *testing.T) func() *docker.ImageBuildOptions {
	orig := imageBuildFromDirCLI
	var captured *docker.ImageBuildOptions
	imageBuildFromDirCLI = func(_ context.Context, _ string, opts docker.ImageBuildOptions) (string, error) {
		captured = &opts
		return "sha256:built123", nil
	}
	t.Cleanup(func() { imageBuildFromDirCLI = orig })
	return func() *docker.ImageBuildOptions { return captured }
}

// Test that BuildImage returns the image name when an image-based config
// is used and the image is already present locally.
func TestBuildImageAlreadyPresent(t *testing.T) {
	cli := &localMockClient{
		inspectResponses: []inspectResponse{
			{result: client.ImageInspectResult{InspectResponse: image.InspectResponse{ID: "sha256:abc123"}}},
		},
	}
	cfg := &spec.Config{Image: "alpine:3.19"}
	ref, err := BuildImage(context.Background(), cli, cfg, "/tmp/test", false, false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if ref != "alpine:3.19" {
		t.Errorf("expected ref alpine:3.19, got %s", ref)
	}
	if cli.inspectCallCount != 1 {
		t.Errorf("expected 1 ImageInspect call, got %d", cli.inspectCallCount)
	}
}

// Test that BuildImage pulls the image when it is not present locally.
func TestBuildImagePullsWhenMissing(t *testing.T) {
	cli := &localMockClient{
		inspectResponses: []inspectResponse{
			{err: fmt.Errorf("not found")},
		},
		imagePullResult: &mockImagePullResponse{
			ReadCloser: io.NopCloser(strings.NewReader("")),
			waitErr:    nil,
		},
	}
	cfg := &spec.Config{Image: "alpine:3.19"}
	ref, err := BuildImage(context.Background(), cli, cfg, "/tmp/test", false, false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if ref != "alpine:3.19" {
		t.Errorf("expected ref alpine:3.19, got %s", ref)
	}
}

// Test that BuildImage returns an error when neither image nor build is set.
func TestBuildImageNoImageOrBuild(t *testing.T) {
	cli := &localMockClient{}
	cfg := &spec.Config{}
	_, err := BuildImage(context.Background(), cli, cfg, "/tmp/test", false, false)
	if err == nil {
		t.Fatal("expected error when neither image or build is configured")
	}
	if !strings.Contains(err.Error(), "does not specify image or build") {
		t.Errorf("expected error about missing image or build, got: %s", err.Error())
	}
}

// Test that BuildImage builds and tags a new image when the Dockerfile-based
// config has no existing cached image.
func TestBuildImageBuildsWhenNotCached(t *testing.T) {
	tmpDir := t.TempDir()
	devDir := filepath.Join(tmpDir, ".devcontainer")
	if err := os.MkdirAll(devDir, 0755); err != nil {
		t.Fatalf("creating .devcontainer dir: %v", err)
	}
	dockerfile := filepath.Join(devDir, "Dockerfile")
	if err := os.WriteFile(dockerfile, []byte("FROM alpine:3.19\n"), 0644); err != nil {
		t.Fatalf("writing Dockerfile: %v", err)
	}

	getOpts := captureImageBuild(t)

	cli := &localMockClient{
		inspectResponses: []inspectResponse{
			{err: fmt.Errorf("not found")}, // cache miss
		},
	}

	cfg := &spec.Config{Build: &spec.Build{Dockerfile: "Dockerfile"}}
	ref, err := BuildImage(context.Background(), cli, cfg, tmpDir, false, false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.HasPrefix(ref, "dcx-") {
		t.Errorf("expected ref to start with dcx-, got %s", ref)
	}
	captured := getOpts()
	if captured == nil {
		t.Fatal("expected imageBuildFromDirCLI to be called")
	}
	if len(captured.Tags) == 0 || !strings.HasPrefix(captured.Tags[0], "dcx-") {
		t.Errorf("expected tags to include dcx- prefix, got %v", captured.Tags)
	}
	if captured.Labels[docker.DevcontainerLabel] != tmpDir {
		t.Errorf("expected label devcontainer.local_folder=%s, got %s", tmpDir, captured.Labels[docker.DevcontainerLabel])
	}
}

// Test that BuildImage returns the existing stable tag when a cached image
// is already present.
func TestBuildImageReusesCache(t *testing.T) {
	tmpDir := t.TempDir()
	devDir := filepath.Join(tmpDir, ".devcontainer")
	if err := os.MkdirAll(devDir, 0755); err != nil {
		t.Fatalf("creating .devcontainer dir: %v", err)
	}
	dockerfile := filepath.Join(devDir, "Dockerfile")
	if err := os.WriteFile(dockerfile, []byte("FROM alpine:3.19\n"), 0644); err != nil {
		t.Fatalf("writing Dockerfile: %v", err)
	}

	getOpts := captureImageBuild(t)

	cli := &localMockClient{
		inspectResponses: []inspectResponse{
			{result: client.ImageInspectResult{InspectResponse: image.InspectResponse{ID: "sha256:cached456"}}},
		},
	}

	cfg := &spec.Config{Build: &spec.Build{Dockerfile: "Dockerfile"}}
	ref, err := BuildImage(context.Background(), cli, cfg, tmpDir, false, false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.HasPrefix(ref, "dcx-") {
		t.Errorf("expected ref to start with dcx-, got %s", ref)
	}
	if getOpts() != nil {
		t.Error("expected imageBuildFromDirCLI to NOT be called when cached image exists")
	}
}

// Test that BuildImage rebuilds the image even when a cached image exists
// if forceRebuild is true.
func TestBuildImageForceRebuild(t *testing.T) {
	tmpDir := t.TempDir()
	devDir := filepath.Join(tmpDir, ".devcontainer")
	if err := os.MkdirAll(devDir, 0755); err != nil {
		t.Fatalf("creating .devcontainer dir: %v", err)
	}
	dockerfile := filepath.Join(devDir, "Dockerfile")
	if err := os.WriteFile(dockerfile, []byte("FROM alpine:3.19\n"), 0644); err != nil {
		t.Fatalf("writing Dockerfile: %v", err)
	}

	getOpts := captureImageBuild(t)

	cli := &localMockClient{
		inspectResponses: []inspectResponse{
			{result: client.ImageInspectResult{InspectResponse: image.InspectResponse{ID: "sha256:cached456"}}},
		},
	}

	cfg := &spec.Config{Build: &spec.Build{Dockerfile: "Dockerfile"}}
	ref, err := BuildImage(context.Background(), cli, cfg, tmpDir, true, false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.HasPrefix(ref, "dcx-") {
		t.Errorf("expected ref to start with dcx-, got %s", ref)
	}
	if getOpts() == nil {
		t.Error("expected imageBuildFromDirCLI to be called when forceRebuild=true")
	}
}

// Test that build arguments are correctly passed to the Docker CLI builder.
func TestBuildImageBuildArgs(t *testing.T) {
	tmpDir := t.TempDir()
	devDir := filepath.Join(tmpDir, ".devcontainer")
	if err := os.MkdirAll(devDir, 0755); err != nil {
		t.Fatalf("creating .devcontainer dir: %v", err)
	}
	dockerfile := filepath.Join(devDir, "Dockerfile")
	if err := os.WriteFile(dockerfile, []byte("FROM alpine:3.19\n"), 0644); err != nil {
		t.Fatalf("writing Dockerfile: %v", err)
	}

	getOpts := captureImageBuild(t)

	cli := &localMockClient{
		inspectResponses: []inspectResponse{
			{err: fmt.Errorf("not found")},
		},
	}

	cfg := &spec.Config{
		Build: &spec.Build{
			Dockerfile: "Dockerfile",
			Args:       map[string]string{"MYARG": "MYVALUE", "EMPTY": ""},
		},
	}
	_, err := BuildImage(context.Background(), cli, cfg, tmpDir, false, false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	captured := getOpts()
	if captured == nil {
		t.Fatal("expected imageBuildFromDirCLI to be called")
	}
	if len(captured.BuildArgs) != 2 {
		t.Fatalf("expected 2 build args, got %d", len(captured.BuildArgs))
	}
	if captured.BuildArgs["MYARG"] != "MYVALUE" {
		t.Errorf("expected MYARG=MYVALUE, got %q", captured.BuildArgs["MYARG"])
	}
	if captured.BuildArgs["EMPTY"] != "" {
		t.Errorf("expected EMPTY=\"\", got %q", captured.BuildArgs["EMPTY"])
	}
}

// Test that build.context is resolved relative to .devcontainer/ and that
// the Dockerfile path is passed correctly to the CLI builder.
func TestBuildImageContextPath(t *testing.T) {
	tmpDir := t.TempDir()
	devDir := filepath.Join(tmpDir, ".devcontainer")
	if err := os.MkdirAll(devDir, 0755); err != nil {
		t.Fatalf("creating .devcontainer dir: %v", err)
	}
	dockerfile := filepath.Join(devDir, "Dockerfile")
	if err := os.WriteFile(dockerfile, []byte("FROM alpine:3.19\n"), 0644); err != nil {
		t.Fatalf("writing Dockerfile: %v", err)
	}

	getOpts := captureImageBuild(t)

	cli := &localMockClient{
		inspectResponses: []inspectResponse{
			{err: fmt.Errorf("not found")},
		},
	}

	cfg := &spec.Config{
		Build: &spec.Build{
			Dockerfile: "Dockerfile",
			Context:    "..",
		},
	}
	_, err := BuildImage(context.Background(), cli, cfg, tmpDir, false, false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	captured := getOpts()
	if captured == nil {
		t.Fatal("expected imageBuildFromDirCLI to be called")
	}
	wantDockerfile := ".devcontainer/Dockerfile"
	if captured.Dockerfile != wantDockerfile {
		t.Errorf("expected Dockerfile=%q, got %q", wantDockerfile, captured.Dockerfile)
	}
}

// Test that the stable tag format is correct and deterministic.
func TestBuildImageStableTag(t *testing.T) {
	tmpDir := t.TempDir()
	devDir := filepath.Join(tmpDir, ".devcontainer")
	if err := os.MkdirAll(devDir, 0755); err != nil {
		t.Fatalf("creating .devcontainer dir: %v", err)
	}
	dockerfile := filepath.Join(devDir, "Dockerfile")
	if err := os.WriteFile(dockerfile, []byte("FROM alpine:3.19\n"), 0644); err != nil {
		t.Fatalf("writing Dockerfile: %v", err)
	}

	// Override the builder so no actual Docker call is made.
	orig := imageBuildFromDirCLI
	imageBuildFromDirCLI = func(_ context.Context, _ string, _ docker.ImageBuildOptions) (string, error) {
		return "sha256:built123", nil
	}
	defer func() { imageBuildFromDirCLI = orig }()

	cli := &localMockClient{
		inspectResponses: []inspectResponse{
			{result: client.ImageInspectResult{InspectResponse: image.InspectResponse{ID: "sha256:cached789"}}},
		},
	}

	cfg := &spec.Config{Build: &spec.Build{Dockerfile: "Dockerfile"}}
	ref1, err := BuildImage(context.Background(), cli, cfg, tmpDir, false, false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Reset counter and re-run; should get the exact same tag because the
	// Dockerfile and config haven't changed.
	cli.inspectCallCount = 0
	ref2, err := BuildImage(context.Background(), cli, cfg, tmpDir, false, false)
	if err != nil {
		t.Fatalf("expected no error on second call, got %v", err)
	}
	if ref1 != ref2 {
		t.Errorf("expected deterministic tag, got %q and %q", ref1, ref2)
	}

	parts := strings.Split(ref1, ":")
	if len(parts) != 2 {
		t.Fatalf("expected tag in format name:hash, got %q", ref1)
	}
	if !strings.HasPrefix(parts[0], "dcx-") {
		t.Errorf("expected repo to start with dcx-, got %q", parts[0])
	}
	if len(parts[1]) != 16 {
		t.Errorf("expected 16-char hash, got %q (len=%d)", parts[1], len(parts[1]))
	}
}

// Test that BuildImage delegates to the feature builder when features are
// configured. The base image is resolved first, then featureImageBuilder is
// invoked with the base reference and feature list.
func TestBuildImageWithFeatures_CallsFeatureBuilder(t *testing.T) {
	cli := &localMockClient{
		inspectResponses: []inspectResponse{
			{result: client.ImageInspectResult{InspectResponse: image.InspectResponse{ID: "sha256:baseimg"}}},
		},
	}

	called := false
	featureImageBuilder = func(_ context.Context, _ docker.DockerClient, baseImage string, features map[string]json.RawMessage, _, _, _ string, _, _ bool) (string, error) {
		called = true
		if baseImage != "alpine:3.19" {
			t.Errorf("expected baseImage alpine:3.19, got %s", baseImage)
		}
		if len(features) != 1 {
			t.Errorf("expected 1 feature, got %d", len(features))
		}
		return "dcx-test-feat:1234", nil
	}
	defer func() { featureImageBuilder = features.BuildFeatureImage }()

	cfg := &spec.Config{
		Image: "alpine:3.19",
		Features: map[string]json.RawMessage{
			"ghcr.io/devcontainers/features/github-cli:1": {},
		},
	}
	ref, err := BuildImage(context.Background(), cli, cfg, "/tmp/test", false, false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if ref != "dcx-test-feat:1234" {
		t.Errorf("expected ref dcx-test-feat:1234, got %s", ref)
	}
	if !called {
		t.Error("expected featureImageBuilder to be called")
	}
}
