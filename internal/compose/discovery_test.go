package compose

import (
	"context"
	"fmt"
	"testing"

	"github.com/moby/moby/api/types/container"
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

func (m *discoveryMockClient) ExecStart(_ context.Context, _ string, _ client.ExecStartOptions) (client.ExecStartResult, error) {
	return client.ExecStartResult{}, nil
}

func (m *discoveryMockClient) ExecInspect(_ context.Context, _ string, _ client.ExecInspectOptions) (client.ExecInspectResult, error) {
	return client.ExecInspectResult{ExitCode: 0}, nil
}

func (m *discoveryMockClient) Close() error {
	return nil
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
