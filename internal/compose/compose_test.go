package compose

import (
	"context"
	"fmt"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

// mockDockerClient is a minimal test double that satisfies docker.DockerClient.
type mockDockerClient struct {
	containers       client.ContainerListResult
	containerListErr error
	stopErr          error
	removeErr        error
	volumeRemoveErr  error
	stoppedIDs       []string
	removedIDs       []string
	removedVolumes   []string
}

func (m *mockDockerClient) Ping(_ context.Context, _ client.PingOptions) (client.PingResult, error) {
	return client.PingResult{}, nil
}

func (m *mockDockerClient) ContainerList(_ context.Context, _ client.ContainerListOptions) (client.ContainerListResult, error) {
	return m.containers, m.containerListErr
}

func (m *mockDockerClient) ContainerInspect(_ context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	return client.ContainerInspectResult{}, nil
}

func (m *mockDockerClient) ContainerStop(_ context.Context, containerID string, _ client.ContainerStopOptions) (client.ContainerStopResult, error) {
	m.stoppedIDs = append(m.stoppedIDs, containerID)
	return client.ContainerStopResult{}, m.stopErr
}

func (m *mockDockerClient) ContainerRemove(_ context.Context, containerID string, _ client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
	m.removedIDs = append(m.removedIDs, containerID)
	return client.ContainerRemoveResult{}, m.removeErr
}

func (m *mockDockerClient) ImageRemove(_ context.Context, _ string, _ client.ImageRemoveOptions) (client.ImageRemoveResult, error) {
	return client.ImageRemoveResult{}, nil
}

func (m *mockDockerClient) VolumeRemove(_ context.Context, volumeID string, _ client.VolumeRemoveOptions) (client.VolumeRemoveResult, error) {
	m.removedVolumes = append(m.removedVolumes, volumeID)
	return client.VolumeRemoveResult{}, m.volumeRemoveErr
}

func (m *mockDockerClient) CopyToContainer(_ context.Context, _ string, _ client.CopyToContainerOptions) (client.CopyToContainerResult, error) {
	return client.CopyToContainerResult{}, nil
}

func (m *mockDockerClient) ExecCreate(_ context.Context, _ string, _ client.ExecCreateOptions) (client.ExecCreateResult, error) {
	return client.ExecCreateResult{ID: "exec123"}, nil
}

func (m *mockDockerClient) ExecAttach(_ context.Context, _ string, _ client.ExecAttachOptions) (client.ExecAttachResult, error) {
	return client.ExecAttachResult{}, nil
}

func (m *mockDockerClient) ExecStart(_ context.Context, _ string, _ client.ExecStartOptions) (client.ExecStartResult, error) {
	return client.ExecStartResult{}, nil
}

func (m *mockDockerClient) ExecInspect(_ context.Context, _ string, _ client.ExecInspectOptions) (client.ExecInspectResult, error) {
	return client.ExecInspectResult{ExitCode: 0}, nil
}

func (m *mockDockerClient) ImagePull(_ context.Context, _ string, _ client.ImagePullOptions) (client.ImagePullResponse, error) {
	return nil, nil
}

func (m *mockDockerClient) ImageInspect(_ context.Context, _ string, _ ...client.ImageInspectOption) (client.ImageInspectResult, error) {
	return client.ImageInspectResult{InspectResponse: image.InspectResponse{}}, nil
}

func (m *mockDockerClient) ImageTag(_ context.Context, _ client.ImageTagOptions) (client.ImageTagResult, error) {
	return client.ImageTagResult{}, nil
}

func (m *mockDockerClient) ImageList(_ context.Context, _ client.ImageListOptions) (client.ImageListResult, error) {
	return client.ImageListResult{}, nil
}

func (m *mockDockerClient) Close() error {
	return nil
}

func TestExtractProject(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   *Project
	}{
		{
			name:   "no labels",
			labels: map[string]string{},
			want:   nil,
		},
		{
			name:   "compose project label",
			labels: map[string]string{"com.docker.compose.project": "myproject"},
			want:   &Project{Name: "myproject"},
		},
		{
			name:   "empty project name",
			labels: map[string]string{"com.docker.compose.project": ""},
			want:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractProject(tt.labels)
			if (got == nil) != (tt.want == nil) {
				t.Fatalf("ExtractProject() = %v, want %v", got, tt.want)
			}
			if got != nil && got.Name != tt.want.Name {
				t.Errorf("ExtractProject().Name = %q, want %q", got.Name, tt.want.Name)
			}
		})
	}
}

func TestStop(t *testing.T) {
	cli := &mockDockerClient{
		containers: client.ContainerListResult{
			Items: []container.Summary{
				{
					ID:     "abc123",
					Image:  "img1",
					Labels: map[string]string{"com.docker.compose.project": "proj", "com.docker.compose.service": "svc1"},
					State:  container.StateRunning,
				},
				{
					ID:     "def456",
					Image:  "img2",
					Labels: map[string]string{"com.docker.compose.project": "proj", "com.docker.compose.service": "svc2"},
					State:  container.StateExited,
				},
			},
		},
	}

	project := &Project{Name: "proj"}
	if err := Stop(context.Background(), cli, project); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cli.stoppedIDs) != 1 {
		t.Fatalf("expected 1 stopped container, got %d", len(cli.stoppedIDs))
	}
	if cli.stoppedIDs[0] != "abc123" {
		t.Errorf("stopped container = %q, want abc123", cli.stoppedIDs[0])
	}
}

func TestStopNoContainers(t *testing.T) {
	cli := &mockDockerClient{
		containers: client.ContainerListResult{Items: []container.Summary{}},
	}

	project := &Project{Name: "proj"}
	if err := Stop(context.Background(), cli, project); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cli.stoppedIDs) != 0 {
		t.Errorf("expected 0 stopped containers, got %d", len(cli.stoppedIDs))
	}
}

func TestStopError(t *testing.T) {
	cli := &mockDockerClient{
		containers: client.ContainerListResult{
			Items: []container.Summary{
				{
					ID:     "abc123",
					Image:  "img1",
					Labels: map[string]string{"com.docker.compose.project": "proj"},
					State:  container.StateRunning,
				},
			},
		},
		stopErr: fmt.Errorf("stop failed"),
	}

	project := &Project{Name: "proj"}
	err := Stop(context.Background(), cli, project)
	if err == nil {
		t.Fatal("expected error when stop fails")
	}
	if cli.stopErr.Error() != err.Error() {
		// Error wraps the original.
		if !contains(err.Error(), "stopping compose container") {
			t.Errorf("error should mention stopping compose container, got: %s", err.Error())
		}
	}
}

func TestDown(t *testing.T) {
	cli := &mockDockerClient{
		containers: client.ContainerListResult{
			Items: []container.Summary{
				{
					ID:     "abc123",
					Image:  "img1",
					Labels: map[string]string{"com.docker.compose.project": "proj"},
					State:  container.StateRunning,
				},
				{
					ID:     "def456",
					Image:  "img2",
					Labels: map[string]string{"com.docker.compose.project": "proj"},
					State:  container.StateExited,
				},
			},
		},
	}

	project := &Project{Name: "proj"}
	if err := Down(context.Background(), cli, project, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cli.stoppedIDs) != 1 {
		t.Errorf("expected 1 stopped container, got %d", len(cli.stoppedIDs))
	}
	if len(cli.removedIDs) != 2 {
		t.Errorf("expected 2 removed containers, got %d", len(cli.removedIDs))
	}
}

func TestDownWithVolumes(t *testing.T) {
	cli := &mockDockerClient{
		containers: client.ContainerListResult{
			Items: []container.Summary{
				{
					ID:     "abc123",
					Image:  "img1",
					Labels: map[string]string{"com.docker.compose.project": "proj"},
					State:  container.StateRunning,
					Mounts: []container.MountPoint{
						{Type: mount.TypeVolume, Name: "vol1"},
						{Type: mount.TypeBind, Source: "/host", Destination: "/container"},
					},
				},
				{
					ID:     "def456",
					Image:  "img2",
					Labels: map[string]string{"com.docker.compose.project": "proj"},
					State:  container.StateExited,
					Mounts: []container.MountPoint{
						{Type: mount.TypeVolume, Name: "vol1"}, // duplicate
						{Type: mount.TypeVolume, Name: "vol2"},
					},
				},
			},
		},
	}

	project := &Project{Name: "proj"}
	if err := Down(context.Background(), cli, project, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cli.removedVolumes) != 2 {
		t.Fatalf("expected 2 removed volumes, got %d (%v)", len(cli.removedVolumes), cli.removedVolumes)
	}
	volMap := make(map[string]bool)
	for _, v := range cli.removedVolumes {
		volMap[v] = true
	}
	if !volMap["vol1"] || !volMap["vol2"] {
		t.Errorf("expected vol1 and vol2 removed, got %v", cli.removedVolumes)
	}
}

func TestDownNoContainers(t *testing.T) {
	cli := &mockDockerClient{
		containers: client.ContainerListResult{Items: []container.Summary{}},
	}

	project := &Project{Name: "proj"}
	if err := Down(context.Background(), cli, project, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cli.removedIDs) != 0 {
		t.Errorf("expected 0 removed containers, got %d", len(cli.removedIDs))
	}
}

func TestDownRemoveError(t *testing.T) {
	cli := &mockDockerClient{
		containers: client.ContainerListResult{
			Items: []container.Summary{
				{
					ID:     "abc123",
					Image:  "img1",
					Labels: map[string]string{"com.docker.compose.project": "proj"},
					State:  container.StateExited,
				},
			},
		},
		removeErr: fmt.Errorf("remove failed"),
	}

	project := &Project{Name: "proj"}
	err := Down(context.Background(), cli, project, false)
	if err == nil {
		t.Fatal("expected error when remove fails")
	}
	if !contains(err.Error(), "removing compose container") {
		t.Errorf("error should mention removing compose container, got: %s", err.Error())
	}
}

func TestDownSkipsAlreadyRemoved(t *testing.T) {
	cli := &mockDockerClient{
		containers: client.ContainerListResult{
			Items: []container.Summary{
				{
					ID:     "abc123",
					Image:  "img1",
					Labels: map[string]string{"com.docker.compose.project": "proj"},
					State:  container.StateExited,
				},
			},
		},
		removeErr: fmt.Errorf("No such container: abc123"),
	}

	project := &Project{Name: "proj"}
	if err := Down(context.Background(), cli, project, false); err != nil {
		t.Fatalf("unexpected error for already-removed container: %v", err)
	}
}

func TestFindContainersError(t *testing.T) {
	cli := &mockDockerClient{
		containerListErr: fmt.Errorf("list failed"),
	}

	project := &Project{Name: "proj"}
	_, err := FindContainers(context.Background(), cli, project)
	if err == nil {
		t.Fatal("expected error when list fails")
	}
	if !contains(err.Error(), "listing compose containers") {
		t.Errorf("error should mention listing compose containers, got: %s", err.Error())
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
