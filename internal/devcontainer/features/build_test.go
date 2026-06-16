package features

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
)

// mockImageInspectClient records ImageInspect calls and returns pre-configured
// responses.
type mockImageInspectClient struct {
	inspectCount   int
	inspectResults []client.ImageInspectResult
	inspectErrs    []error
}

func (m *mockImageInspectClient) nextInspect(image string) (client.ImageInspectResult, error) {
	idx := m.inspectCount
	m.inspectCount++
	if idx < len(m.inspectResults) {
		return m.inspectResults[idx], m.inspectErrs[idx]
	}
	return client.ImageInspectResult{}, fmt.Errorf("unexpected inspect call %d for %s", idx, image)
}

func (m *mockImageInspectClient) Ping(_ context.Context, _ client.PingOptions) (client.PingResult, error) {
	return client.PingResult{}, nil
}
func (m *mockImageInspectClient) ContainerList(_ context.Context, _ client.ContainerListOptions) (client.ContainerListResult, error) {
	return client.ContainerListResult{}, nil
}
func (m *mockImageInspectClient) ContainerInspect(_ context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	return client.ContainerInspectResult{}, nil
}
func (m *mockImageInspectClient) ContainerStop(_ context.Context, _ string, _ client.ContainerStopOptions) (client.ContainerStopResult, error) {
	return client.ContainerStopResult{}, nil
}
func (m *mockImageInspectClient) ContainerRemove(_ context.Context, _ string, _ client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
	return client.ContainerRemoveResult{}, nil
}
func (m *mockImageInspectClient) ImageRemove(_ context.Context, _ string, _ client.ImageRemoveOptions) (client.ImageRemoveResult, error) {
	return client.ImageRemoveResult{}, nil
}
func (m *mockImageInspectClient) VolumeRemove(_ context.Context, _ string, _ client.VolumeRemoveOptions) (client.VolumeRemoveResult, error) {
	return client.VolumeRemoveResult{}, nil
}
func (m *mockImageInspectClient) CopyToContainer(_ context.Context, _ string, _ client.CopyToContainerOptions) (client.CopyToContainerResult, error) {
	return client.CopyToContainerResult{}, nil
}
func (m *mockImageInspectClient) ExecCreate(_ context.Context, _ string, _ client.ExecCreateOptions) (client.ExecCreateResult, error) {
	return client.ExecCreateResult{ID: "exec123"}, nil
}
func (m *mockImageInspectClient) ExecAttach(_ context.Context, _ string, _ client.ExecAttachOptions) (client.ExecAttachResult, error) {
	return client.ExecAttachResult{}, nil
}
func (m *mockImageInspectClient) ExecStart(_ context.Context, _ string, _ client.ExecStartOptions) (client.ExecStartResult, error) {
	return client.ExecStartResult{}, nil
}
func (m *mockImageInspectClient) ExecInspect(_ context.Context, _ string, _ client.ExecInspectOptions) (client.ExecInspectResult, error) {
	return client.ExecInspectResult{}, nil
}
func (m *mockImageInspectClient) ImagePull(_ context.Context, _ string, _ client.ImagePullOptions) (client.ImagePullResponse, error) {
	return nil, nil
}
func (m *mockImageInspectClient) ImageBuild(_ context.Context, _ io.Reader, _ client.ImageBuildOptions) (client.ImageBuildResult, error) {
	return client.ImageBuildResult{}, nil
}
func (m *mockImageInspectClient) ImageInspect(_ context.Context, image string, _ ...client.ImageInspectOption) (client.ImageInspectResult, error) {
	return m.nextInspect(image)
}
func (m *mockImageInspectClient) ImageTag(_ context.Context, _ client.ImageTagOptions) (client.ImageTagResult, error) {
	return client.ImageTagResult{}, nil
}
func (m *mockImageInspectClient) ImageList(_ context.Context, _ client.ImageListOptions) (client.ImageListResult, error) {
	return client.ImageListResult{}, nil
}
func (m *mockImageInspectClient) Close() error { return nil }

func TestStableFeatureTagDeterminism(t *testing.T) {
	cli := &mockImageInspectClient{
		inspectResults: []client.ImageInspectResult{
			{InspectResponse: image.InspectResponse{ID: "sha256:base123"}},
		},
		inspectErrs: []error{nil},
	}

	features := []ResolvedFeature{
		{
			Ref:    FeatureRef{Registry: "ghcr.io", Namespace: "devcontainers/features", ID: "github-cli", Version: "1"},
			Meta:   FeatureMeta{ID: "github-cli", Version: "1.0.0"},
			Digest: "sha256:abc",
		},
	}

	tag1, err := stableFeatureTag(cli, "base:latest", "/workspace/test", features)
	if err != nil {
		t.Fatalf("stableFeatureTag error: %v", err)
	}

	cli.inspectCount = 0
	tag2, err := stableFeatureTag(cli, "base:latest", "/workspace/test", features)
	if err != nil {
		t.Fatalf("stableFeatureTag error on second call: %v", err)
	}

	if tag1 != tag2 {
		t.Errorf("tags not deterministic: %q vs %q", tag1, tag2)
	}
	if !hasPrefix(tag1, "dcx-") {
		t.Errorf("expected tag to start with dcx-, got %q", tag1)
	}
}

func TestStableFeatureTagChangesWithFeature(t *testing.T) {
	cli := &mockImageInspectClient{
		inspectResults: []client.ImageInspectResult{
			{InspectResponse: image.InspectResponse{ID: "sha256:base123"}},
			{InspectResponse: image.InspectResponse{ID: "sha256:base123"}},
		},
		inspectErrs: []error{nil, nil},
	}

	features1 := []ResolvedFeature{
		{Ref: FeatureRef{ID: "a"}, Meta: FeatureMeta{ID: "a"}, Digest: "sha256:abc"},
	}
	features2 := []ResolvedFeature{
		{Ref: FeatureRef{ID: "a"}, Meta: FeatureMeta{ID: "a"}, Digest: "sha256:abc"},
		{Ref: FeatureRef{ID: "b"}, Meta: FeatureMeta{ID: "b"}, Digest: "sha256:def"},
	}

	tag1, _ := stableFeatureTag(cli, "base:latest", "/workspace/test", features1)
	cli.inspectCount = 0
	tag2, _ := stableFeatureTag(cli, "base:latest", "/workspace/test", features2)

	if tag1 == tag2 {
		t.Error("expected different tags when features differ")
	}
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
