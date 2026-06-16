package compose

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

type discoveryMockClient struct {
	containers       client.ContainerListResult
	containerListErr error
}

func (m *discoveryMockClient) Ping(_ context.Context, _ client.PingOptions) (client.PingResult, error) {
	return client.PingResult{}, nil
}

func (m *discoveryMockClient) ContainerList(_ context.Context, _ client.ContainerListOptions) (client.ContainerListResult, error) {
	return m.containers, m.containerListErr
}

func (m *discoveryMockClient) ContainerInspect(_ context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	return client.ContainerInspectResult{}, nil
}

func (m *discoveryMockClient) ContainerStop(_ context.Context, _ string, _ client.ContainerStopOptions) (client.ContainerStopResult, error) {
	return client.ContainerStopResult{}, nil
}

func (m *discoveryMockClient) ContainerRemove(_ context.Context, _ string, _ client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
	return client.ContainerRemoveResult{}, nil
}

func (m *discoveryMockClient) ImageRemove(_ context.Context, _ string, _ client.ImageRemoveOptions) (client.ImageRemoveResult, error) {
	return client.ImageRemoveResult{}, nil
}

func (m *discoveryMockClient) VolumeRemove(_ context.Context, _ string, _ client.VolumeRemoveOptions) (client.VolumeRemoveResult, error) {
	return client.VolumeRemoveResult{}, nil
}

func (m *discoveryMockClient) CopyToContainer(_ context.Context, _ string, _ client.CopyToContainerOptions) (client.CopyToContainerResult, error) {
	return client.CopyToContainerResult{}, nil
}

func (m *discoveryMockClient) ExecCreate(_ context.Context, _ string, _ client.ExecCreateOptions) (client.ExecCreateResult, error) {
	return client.ExecCreateResult{ID: "exec123"}, nil
}

func (m *discoveryMockClient) ExecAttach(_ context.Context, _ string, _ client.ExecAttachOptions) (client.ExecAttachResult, error) {
	return client.ExecAttachResult{}, nil
}

func (m *discoveryMockClient) ExecStart(_ context.Context, _ string, _ client.ExecStartOptions) (client.ExecStartResult, error) {
	return client.ExecStartResult{}, nil
}

func (m *discoveryMockClient) ExecInspect(_ context.Context, _ string, _ client.ExecInspectOptions) (client.ExecInspectResult, error) {
	return client.ExecInspectResult{ExitCode: 0}, nil
}

func (m *discoveryMockClient) ImagePull(_ context.Context, _ string, _ client.ImagePullOptions) (client.ImagePullResponse, error) {
	return nil, nil
}

func (m *discoveryMockClient) ImageInspect(_ context.Context, _ string, _ ...client.ImageInspectOption) (client.ImageInspectResult, error) {
	return client.ImageInspectResult{InspectResponse: image.InspectResponse{}}, nil
}

func (m *discoveryMockClient) ImageTag(_ context.Context, _ client.ImageTagOptions) (client.ImageTagResult, error) {
	return client.ImageTagResult{}, nil
}

func (m *discoveryMockClient) ImageList(_ context.Context, _ client.ImageListOptions) (client.ImageListResult, error) {
	return client.ImageListResult{}, nil
}

func (m *discoveryMockClient) Close() error {
	return nil
}

// projectContainersMockClient is a filter-aware test double that returns
// different results for devcontainer and compose container list queries.
type projectContainersMockClient struct {
	devcontainers     client.ContainerListResult
	devcontainerErr   error
	composeContainers client.ContainerListResult
	composeListErr    error
}

func (m *projectContainersMockClient) Ping(_ context.Context, _ client.PingOptions) (client.PingResult, error) {
	return client.PingResult{}, nil
}

func (m *projectContainersMockClient) ContainerList(_ context.Context, opts client.ContainerListOptions) (client.ContainerListResult, error) {
	labels, ok := opts.Filters["label"]
	if !ok {
		return client.ContainerListResult{}, nil
	}
	for k := range labels {
		if strings.Contains(k, "devcontainer.local_folder") {
			return m.devcontainers, m.devcontainerErr
		}
		if strings.Contains(k, "com.docker.compose.project") {
			return m.composeContainers, m.composeListErr
		}
	}
	return client.ContainerListResult{}, nil
}

func (m *projectContainersMockClient) ContainerInspect(_ context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	return client.ContainerInspectResult{}, nil
}

func (m *projectContainersMockClient) ContainerStop(_ context.Context, _ string, _ client.ContainerStopOptions) (client.ContainerStopResult, error) {
	return client.ContainerStopResult{}, nil
}

func (m *projectContainersMockClient) ContainerRemove(_ context.Context, _ string, _ client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
	return client.ContainerRemoveResult{}, nil
}

func (m *projectContainersMockClient) ImageRemove(_ context.Context, _ string, _ client.ImageRemoveOptions) (client.ImageRemoveResult, error) {
	return client.ImageRemoveResult{}, nil
}

func (m *projectContainersMockClient) VolumeRemove(_ context.Context, _ string, _ client.VolumeRemoveOptions) (client.VolumeRemoveResult, error) {
	return client.VolumeRemoveResult{}, nil
}

func (m *projectContainersMockClient) CopyToContainer(_ context.Context, _ string, _ client.CopyToContainerOptions) (client.CopyToContainerResult, error) {
	return client.CopyToContainerResult{}, nil
}

func (m *projectContainersMockClient) ExecCreate(_ context.Context, _ string, _ client.ExecCreateOptions) (client.ExecCreateResult, error) {
	return client.ExecCreateResult{ID: "exec123"}, nil
}

func (m *projectContainersMockClient) ExecAttach(_ context.Context, _ string, _ client.ExecAttachOptions) (client.ExecAttachResult, error) {
	return client.ExecAttachResult{}, nil
}

func (m *projectContainersMockClient) ExecStart(_ context.Context, _ string, _ client.ExecStartOptions) (client.ExecStartResult, error) {
	return client.ExecStartResult{}, nil
}

func (m *projectContainersMockClient) ExecInspect(_ context.Context, _ string, _ client.ExecInspectOptions) (client.ExecInspectResult, error) {
	return client.ExecInspectResult{ExitCode: 0}, nil
}

func (m *projectContainersMockClient) ImagePull(_ context.Context, _ string, _ client.ImagePullOptions) (client.ImagePullResponse, error) {
	return nil, nil
}

func (m *projectContainersMockClient) ImageInspect(_ context.Context, _ string, _ ...client.ImageInspectOption) (client.ImageInspectResult, error) {
	return client.ImageInspectResult{InspectResponse: image.InspectResponse{}}, nil
}

func (m *projectContainersMockClient) ImageTag(_ context.Context, _ client.ImageTagOptions) (client.ImageTagResult, error) {
	return client.ImageTagResult{}, nil
}

func (m *projectContainersMockClient) ImageList(_ context.Context, _ client.ImageListOptions) (client.ImageListResult, error) {
	return client.ImageListResult{}, nil
}

func (m *projectContainersMockClient) Close() error {
	return nil
}

func TestFindProjectContainersDevcontainerOnly(t *testing.T) {
	cli := &projectContainersMockClient{
		devcontainers: client.ContainerListResult{
			Items: []container.Summary{
				{
					ID:    "abc123def4567890123456789012",
					Names: []string{"/vsc-project"},
					Image: "myimage",
					State: container.StateRunning,
				},
			},
		},
	}

	containers, err := FindProjectContainers(context.Background(), cli, "/foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}
	if containers[0].ID != "abc123def4567890123456789012" {
		t.Errorf("container ID = %q, want abc123...", containers[0].ID)
	}
}

func TestFindProjectContainersWithCompose(t *testing.T) {
	cli := &projectContainersMockClient{
		devcontainers: client.ContainerListResult{
			Items: []container.Summary{
				{
					ID:     "dev123def4567890123456789012",
					Names:  []string{"/vsc-project"},
					Image:  "myimage",
					State:  container.StateRunning,
					Labels: map[string]string{"devcontainer.local_folder": "/foo", "com.docker.compose.project": "proj"},
				},
			},
		},
		composeContainers: client.ContainerListResult{
			Items: []container.Summary{
				{
					ID:     "svc123def4567890123456789012",
					Names:  []string{"/proj_web_1"},
					Image:  "nginx",
					State:  container.StateRunning,
					Labels: map[string]string{"com.docker.compose.project": "proj", "com.docker.compose.service": "web"},
				},
			},
		},
	}

	containers, err := FindProjectContainers(context.Background(), cli, "/foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(containers) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(containers))
	}

	// Verify both containers are present (order is unstable since
	// FindProjectContainers does not sort).
	ids := make(map[string]bool)
	for _, c := range containers {
		ids[c.ID] = true
	}
	if !ids["dev123def4567890123456789012"] {
		t.Errorf("expected devcontainer ID in results")
	}
	if !ids["svc123def4567890123456789012"] {
		t.Errorf("expected compose container ID in results")
	}
}

func TestFindProjectContainersDedup(t *testing.T) {
	// If the devcontainer is also returned by compose list, it should not
	// appear twice.
	cli := &projectContainersMockClient{
		devcontainers: client.ContainerListResult{
			Items: []container.Summary{
				{
					ID:     "same123def456789012345678901",
					Names:  []string{"/vsc-project"},
					Image:  "myimage",
					State:  container.StateRunning,
					Labels: map[string]string{"devcontainer.local_folder": "/foo", "com.docker.compose.project": "proj"},
				},
			},
		},
		composeContainers: client.ContainerListResult{
			Items: []container.Summary{
				{
					ID:     "same123def456789012345678901",
					Names:  []string{"/vsc-project"},
					Image:  "myimage",
					State:  container.StateRunning,
					Labels: map[string]string{"com.docker.compose.project": "proj", "com.docker.compose.service": "dev"},
				},
			},
		},
	}

	containers, err := FindProjectContainers(context.Background(), cli, "/foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}
}

func TestFindProjectContainersListError(t *testing.T) {
	cli := &projectContainersMockClient{
		devcontainerErr: fmt.Errorf("list failed"),
	}

	_, err := FindProjectContainers(context.Background(), cli, "/foo")
	if err == nil {
		t.Fatal("expected error when container list fails")
	}
}

func TestFindProjectsAndVolumesNoCompose(t *testing.T) {
	cli := &discoveryMockClient{
		containers: client.ContainerListResult{
			Items: []container.Summary{
				{
					ID:     "abc123",
					Image:  "img1",
					Labels: map[string]string{"devcontainer.local_folder": "/foo"},
				},
			},
		},
	}

	projects, volumes, err := FindProjectsAndVolumes(context.Background(), cli, "/foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 0 {
		t.Fatalf("expected 0 projects, got %d", len(projects))
	}
	if len(volumes) != 0 {
		t.Fatalf("expected 0 volumes, got %d", len(volumes))
	}
}

func TestFindProjectsAndVolumesWithCompose(t *testing.T) {
	cli := &discoveryMockClient{
		containers: client.ContainerListResult{
			Items: []container.Summary{
				{
					ID:     "abc123",
					Image:  "img1",
					Labels: map[string]string{"devcontainer.local_folder": "/foo", "com.docker.compose.project": "proj"},
					Mounts: []container.MountPoint{
						{Type: mount.TypeVolume, Name: "vol1"},
					},
				},
			},
		},
	}

	projects, volumes, err := FindProjectsAndVolumes(context.Background(), cli, "/foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0].Name != "proj" {
		t.Errorf("project.Name = %q, want proj", projects[0].Name)
	}
	if len(volumes) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(volumes))
	}
	if volumes[0] != "vol1" {
		t.Errorf("volume = %q, want vol1", volumes[0])
	}
}

func TestFindProjectsAndVolumesMultipleContainersSameProject(t *testing.T) {
	cli := &discoveryMockClient{
		containers: client.ContainerListResult{
			Items: []container.Summary{
				{
					ID:     "abc123",
					Labels: map[string]string{"devcontainer.local_folder": "/foo", "com.docker.compose.project": "proj"},
				},
				{
					ID:     "def456",
					Labels: map[string]string{"devcontainer.local_folder": "/foo", "com.docker.compose.project": "proj"},
				},
			},
		},
	}

	projects, volumes, err := FindProjectsAndVolumes(context.Background(), cli, "/foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 unique project, got %d", len(projects))
	}
	if len(volumes) != 0 {
		t.Fatalf("expected 0 volumes, got %d", len(volumes))
	}
}

func TestFindProjectsAndVolumesListError(t *testing.T) {
	cli := &discoveryMockClient{
		containerListErr: fmt.Errorf("list failed"),
	}

	_, _, err := FindProjectsAndVolumes(context.Background(), cli, "/foo")
	if err == nil {
		t.Fatal("expected error when container list fails")
	}
}

func TestFindProjectsAndVolumesDedupVolumes(t *testing.T) {
	cli := &discoveryMockClient{
		containers: client.ContainerListResult{
			Items: []container.Summary{
				{
					ID:     "abc123",
					Labels: map[string]string{"devcontainer.local_folder": "/foo", "com.docker.compose.project": "proj"},
					Mounts: []container.MountPoint{
						{Type: mount.TypeVolume, Name: "vol1"},
					},
				},
				{
					ID:     "def456",
					Labels: map[string]string{"devcontainer.local_folder": "/foo", "com.docker.compose.project": "proj"},
					Mounts: []container.MountPoint{
						{Type: mount.TypeVolume, Name: "vol1"},
						{Type: mount.TypeVolume, Name: "vol2"},
					},
				},
			},
		},
	}

	_, volumes, err := FindProjectsAndVolumes(context.Background(), cli, "/foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(volumes) != 2 {
		t.Fatalf("expected 2 unique volumes, got %d", len(volumes))
	}
	volMap := make(map[string]bool)
	for _, v := range volumes {
		volMap[v] = true
	}
	if !volMap["vol1"] || !volMap["vol2"] {
		t.Errorf("expected vol1 and vol2, got %v", volumes)
	}
}
