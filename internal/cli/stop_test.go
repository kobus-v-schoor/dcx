package cli

import (
	"context"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// testDockerClient is a minimal mock for docker.DockerClient used in CLI tests.
type testDockerClient struct {
	containers       client.ContainerListResult
	containerListErr error
	closed           bool
}

func (m *testDockerClient) Ping(_ context.Context, _ client.PingOptions) (client.PingResult, error) {
	return client.PingResult{}, nil
}

func (m *testDockerClient) ContainerList(_ context.Context, _ client.ContainerListOptions) (client.ContainerListResult, error) {
	return m.containers, m.containerListErr
}

func (m *testDockerClient) ContainerInspect(_ context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	return client.ContainerInspectResult{}, nil
}

func (m *testDockerClient) ContainerStop(_ context.Context, _ string, _ client.ContainerStopOptions) (client.ContainerStopResult, error) {
	return client.ContainerStopResult{}, nil
}

func (m *testDockerClient) ContainerRemove(_ context.Context, _ string, _ client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
	return client.ContainerRemoveResult{}, nil
}

func (m *testDockerClient) ImageRemove(_ context.Context, _ string, _ client.ImageRemoveOptions) (client.ImageRemoveResult, error) {
	return client.ImageRemoveResult{}, nil
}

func (m *testDockerClient) VolumeRemove(_ context.Context, _ string, _ client.VolumeRemoveOptions) (client.VolumeRemoveResult, error) {
	return client.VolumeRemoveResult{}, nil
}

func (m *testDockerClient) CopyToContainer(_ context.Context, _ string, _ client.CopyToContainerOptions) (client.CopyToContainerResult, error) {
	return client.CopyToContainerResult{}, nil
}

func (m *testDockerClient) ExecCreate(_ context.Context, _ string, _ client.ExecCreateOptions) (client.ExecCreateResult, error) {
	return client.ExecCreateResult{ID: "exec123"}, nil
}

func (m *testDockerClient) ExecStart(_ context.Context, _ string, _ client.ExecStartOptions) (client.ExecStartResult, error) {
	return client.ExecStartResult{}, nil
}

func (m *testDockerClient) ExecInspect(_ context.Context, _ string, _ client.ExecInspectOptions) (client.ExecInspectResult, error) {
	return client.ExecInspectResult{ExitCode: 0}, nil
}

func (m *testDockerClient) Close() error {
	m.closed = true
	return nil
}

func TestFindComposeProjectsNoCompose(t *testing.T) {
	cli := &testDockerClient{
		containers: client.ContainerListResult{
			Items: []container.Summary{
				{
					ID:     "abc123",
					Image:  "img1",
					Labels: map[string]string{"devcontainer.local_folder": "/foo"},
					State:  container.StateRunning,
				},
			},
		},
	}

	projects, err := findComposeProjects(context.Background(), cli, "/foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 0 {
		t.Fatalf("expected 0 projects, got %d", len(projects))
	}
}

func TestFindComposeProjectsWithCompose(t *testing.T) {
	cli := &testDockerClient{
		containers: client.ContainerListResult{
			Items: []container.Summary{
				{
					ID:     "abc123",
					Image:  "img1",
					Labels: map[string]string{"devcontainer.local_folder": "/foo", "com.docker.compose.project": "proj"},
					State:  container.StateRunning,
				},
			},
		},
	}

	projects, err := findComposeProjects(context.Background(), cli, "/foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0].Name != "proj" {
		t.Errorf("project.Name = %q, want proj", projects[0].Name)
	}
}

func TestFindComposeProjectsMultipleContainersSameProject(t *testing.T) {
	cli := &testDockerClient{
		containers: client.ContainerListResult{
			Items: []container.Summary{
				{
					ID:     "abc123",
					Labels: map[string]string{"devcontainer.local_folder": "/foo", "com.docker.compose.project": "proj"},
					State:  container.StateRunning,
				},
				{
					ID:     "def456",
					Labels: map[string]string{"devcontainer.local_folder": "/foo", "com.docker.compose.project": "proj"},
					State:  container.StateRunning,
				},
			},
		},
	}

	projects, err := findComposeProjects(context.Background(), cli, "/foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 unique project, got %d", len(projects))
	}
}

func TestFindComposeProjectsListError(t *testing.T) {
	cli := &testDockerClient{
		containerListErr: context.Canceled,
	}

	_, err := findComposeProjects(context.Background(), cli, "/foo")
	if err == nil {
		t.Fatal("expected error when container list fails")
	}
}
