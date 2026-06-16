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
// Only ImageInspect, ImagePull, and ImageBuild are stateful; the rest return
// zero values.
type localMockClient struct {
	inspectCallCount  int
	inspectResponses  []inspectResponse
	capturedBuildOpts *client.ImageBuildOptions
	imagePullResult   client.ImagePullResponse
	imagePullErr      error
	imageBuildResult  client.ImageBuildResult
	imageBuildErr     error
	imageTagErr       error
	imageListResult   client.ImageListResult
	imageListErr      error
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

func (m *localMockClient) ContainerCreate(_ context.Context, _ client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
	return client.ContainerCreateResult{}, nil
}

func (m *localMockClient) ContainerStart(_ context.Context, _ string, _ client.ContainerStartOptions) (client.ContainerStartResult, error) {
	return client.ContainerStartResult{}, nil
}

func (m *localMockClient) Close() error {
	return nil
}

func (m *localMockClient) ImagePull(_ context.Context, _ string, _ client.ImagePullOptions) (client.ImagePullResponse, error) {
	return m.imagePullResult, m.imagePullErr
}

func (m *localMockClient) ImageBuild(_ context.Context, _ io.Reader, opts client.ImageBuildOptions) (client.ImageBuildResult, error) {
	m.capturedBuildOpts = &opts
	return m.imageBuildResult, m.imageBuildErr
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

	cli := &localMockClient{
		inspectResponses: []inspectResponse{
			{err: fmt.Errorf("not found")}, // cache miss
			{result: client.ImageInspectResult{InspectResponse: image.InspectResponse{ID: "sha256:built123"}}},
		},
		imageBuildResult: client.ImageBuildResult{
			Body: io.NopCloser(strings.NewReader("")),
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
	if cli.capturedBuildOpts == nil {
		t.Fatal("expected ImageBuild to be called")
	}
	if string(cli.capturedBuildOpts.Version) != "1" {
		t.Errorf("expected v1 builder version, got %q", cli.capturedBuildOpts.Version)
	}
	if len(cli.capturedBuildOpts.Tags) == 0 || !strings.HasPrefix(cli.capturedBuildOpts.Tags[0], "dcx-") {
		t.Errorf("expected tags to include dcx- prefix, got %v", cli.capturedBuildOpts.Tags)
	}
	if cli.capturedBuildOpts.Labels[docker.DevcontainerLabel] != tmpDir {
		t.Errorf("expected label devcontainer.local_folder=%s, got %s", tmpDir, cli.capturedBuildOpts.Labels[docker.DevcontainerLabel])
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
	if cli.capturedBuildOpts != nil {
		t.Error("expected ImageBuild to NOT be called when cached image exists")
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

	cli := &localMockClient{
		inspectResponses: []inspectResponse{
			{result: client.ImageInspectResult{InspectResponse: image.InspectResponse{ID: "sha256:cached456"}}},
			{result: client.ImageInspectResult{InspectResponse: image.InspectResponse{ID: "sha256:built123"}}},
		},
		imageBuildResult: client.ImageBuildResult{
			Body: io.NopCloser(strings.NewReader("")),
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
	if cli.capturedBuildOpts == nil {
		t.Error("expected ImageBuild to be called when forceRebuild=true")
	}
}

// Test that build arguments are correctly passed to the Docker client.
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

	cli := &localMockClient{
		inspectResponses: []inspectResponse{
			{err: fmt.Errorf("not found")},
			{result: client.ImageInspectResult{InspectResponse: image.InspectResponse{ID: "sha256:built123"}}},
		},
		imageBuildResult: client.ImageBuildResult{
			Body: io.NopCloser(strings.NewReader("")),
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
	if cli.capturedBuildOpts == nil {
		t.Fatal("expected ImageBuild to be called")
	}
	if len(cli.capturedBuildOpts.BuildArgs) != 2 {
		t.Fatalf("expected 2 build args, got %d", len(cli.capturedBuildOpts.BuildArgs))
	}
	if cli.capturedBuildOpts.BuildArgs["MYARG"] == nil || *cli.capturedBuildOpts.BuildArgs["MYARG"] != "MYVALUE" {
		t.Errorf("expected MYARG=MYVALUE")
	}
	if cli.capturedBuildOpts.BuildArgs["EMPTY"] == nil || *cli.capturedBuildOpts.BuildArgs["EMPTY"] != "" {
		t.Errorf("expected EMPTY=\"\"")
	}
}

// Test that build.context is resolved relative to .devcontainer/ and that
// the Dockerfile path inside the tar is correct.
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

	cli := &localMockClient{
		inspectResponses: []inspectResponse{
			{err: fmt.Errorf("not found")},
			{result: client.ImageInspectResult{InspectResponse: image.InspectResponse{ID: "sha256:built123"}}},
		},
		imageBuildResult: client.ImageBuildResult{
			Body: io.NopCloser(strings.NewReader("")),
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
	if cli.capturedBuildOpts == nil {
		t.Fatal("expected ImageBuild to be called")
	}
	wantDockerfile := ".devcontainer/Dockerfile"
	if cli.capturedBuildOpts.Dockerfile != wantDockerfile {
		t.Errorf("expected Dockerfile=%q, got %q", wantDockerfile, cli.capturedBuildOpts.Dockerfile)
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
