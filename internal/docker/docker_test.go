package docker

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

// mockDockerClient implements DockerClient for testing. Each field controls
// what the corresponding method returns. Unset fields return zero values and
// nil errors by default.
type mockDockerClient struct {
	pingErr           error
	containers        client.ContainerListResult
	containerListErr  error
	inspectResult     client.ContainerInspectResult
	inspectErr        error
	stopErr           error
	removeErr         error
	imageRemoveErr    error
	copyErr           error
	execCreateErr     error
	execStartErr      error
	execInspectErr    error
	execInspectResult client.ExecInspectResult
	closed            bool
}

func (m *mockDockerClient) Ping(_ context.Context, _ client.PingOptions) (client.PingResult, error) {
	return client.PingResult{}, m.pingErr
}

func (m *mockDockerClient) ContainerList(_ context.Context, _ client.ContainerListOptions) (client.ContainerListResult, error) {
	return m.containers, m.containerListErr
}

func (m *mockDockerClient) ContainerInspect(_ context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	return m.inspectResult, m.inspectErr
}

func (m *mockDockerClient) ContainerStop(_ context.Context, _ string, _ client.ContainerStopOptions) (client.ContainerStopResult, error) {
	return client.ContainerStopResult{}, m.stopErr
}

func (m *mockDockerClient) ContainerRemove(_ context.Context, _ string, _ client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
	return client.ContainerRemoveResult{}, m.removeErr
}

func (m *mockDockerClient) ImageRemove(_ context.Context, _ string, _ client.ImageRemoveOptions) (client.ImageRemoveResult, error) {
	return client.ImageRemoveResult{}, m.imageRemoveErr
}

func (m *mockDockerClient) CopyToContainer(_ context.Context, _ string, _ client.CopyToContainerOptions) (client.CopyToContainerResult, error) {
	return client.CopyToContainerResult{}, m.copyErr
}

func (m *mockDockerClient) ExecCreate(_ context.Context, _ string, _ client.ExecCreateOptions) (client.ExecCreateResult, error) {
	return client.ExecCreateResult{ID: "exec123"}, m.execCreateErr
}

func (m *mockDockerClient) ExecStart(_ context.Context, _ string, _ client.ExecStartOptions) (client.ExecStartResult, error) {
	return client.ExecStartResult{}, m.execStartErr
}

func (m *mockDockerClient) ExecInspect(_ context.Context, _ string, _ client.ExecInspectOptions) (client.ExecInspectResult, error) {
	return m.execInspectResult, m.execInspectErr
}

func (m *mockDockerClient) Close() error {
	m.closed = true
	return nil
}

func TestStopNoContainer(t *testing.T) {
	cli := &mockDockerClient{
		containers: client.ContainerListResult{Items: []container.Summary{}},
	}
	err := Stop(context.Background(), cli, ".")
	if err == nil {
		t.Fatal("expected error when no container found")
	}
	if !strings.Contains(err.Error(), "no devcontainer found") {
		t.Errorf("error should mention no devcontainer found, got: %s", err.Error())
	}
}

func TestStopSuccess(t *testing.T) {
	cli := &mockDockerClient{
		containers: client.ContainerListResult{
			Items: []container.Summary{
				{ID: "abc123def456", Image: "my-image:latest"},
			},
		},
	}
	err := Stop(context.Background(), cli, ".")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestStopError(t *testing.T) {
	cli := &mockDockerClient{
		containers: client.ContainerListResult{
			Items: []container.Summary{
				{ID: "abc123def456", Image: "my-image:latest"},
			},
		},
		stopErr: fmt.Errorf("stop failed"),
	}
	err := Stop(context.Background(), cli, ".")
	if err == nil {
		t.Fatal("expected error when stop fails")
	}
	if !strings.Contains(err.Error(), "stopping container") {
		t.Errorf("error should mention stopping container, got: %s", err.Error())
	}
}

func TestDownNoContainer(t *testing.T) {
	cli := &mockDockerClient{
		containers: client.ContainerListResult{Items: []container.Summary{}},
	}
	err := Down(context.Background(), cli, ".")
	if err == nil {
		t.Fatal("expected error when no container found")
	}
	if !strings.Contains(err.Error(), "no devcontainer found") {
		t.Errorf("error should mention no devcontainer found, got: %s", err.Error())
	}
}

func TestDownSuccess(t *testing.T) {
	cli := &mockDockerClient{
		containers: client.ContainerListResult{
			Items: []container.Summary{
				{ID: "abc123def456", Image: "my-image:latest"},
			},
		},
		inspectResult: client.ContainerInspectResult{
			Container: container.InspectResponse{
				Image: "sha256:image123",
			},
		},
	}
	err := Down(context.Background(), cli, ".")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestDownImageInUse(t *testing.T) {
	cli := &mockDockerClient{
		containers: client.ContainerListResult{
			Items: []container.Summary{
				{ID: "abc123def456", Image: "my-image:latest"},
			},
		},
		inspectResult: client.ContainerInspectResult{
			Container: container.InspectResponse{
				Image: "sha256:image123",
			},
		},
		imageRemoveErr: fmt.Errorf("image has dependent child images"),
	}
	// Image removal failure should not be fatal.
	err := Down(context.Background(), cli, ".")
	if err != nil {
		t.Fatalf("expected no error when image removal fails gracefully, got %v", err)
	}
}

func TestDownStopError(t *testing.T) {
	cli := &mockDockerClient{
		containers: client.ContainerListResult{
			Items: []container.Summary{
				{ID: "abc123def456", Image: "my-image:latest"},
			},
		},
		inspectResult: client.ContainerInspectResult{
			Container: container.InspectResponse{
				Image: "sha256:image123",
			},
		},
		stopErr: fmt.Errorf("stop failed"),
	}
	err := Down(context.Background(), cli, ".")
	if err == nil {
		t.Fatal("expected error when stop fails")
	}
	if !strings.Contains(err.Error(), "stopping container") {
		t.Errorf("error should mention stopping container, got: %s", err.Error())
	}
}

func TestDownRemoveError(t *testing.T) {
	cli := &mockDockerClient{
		containers: client.ContainerListResult{
			Items: []container.Summary{
				{ID: "abc123def456", Image: "my-image:latest"},
			},
		},
		inspectResult: client.ContainerInspectResult{
			Container: container.InspectResponse{
				Image: "sha256:image123",
			},
		},
		removeErr: fmt.Errorf("remove failed"),
	}
	err := Down(context.Background(), cli, ".")
	if err == nil {
		t.Fatal("expected error when remove fails")
	}
	if !strings.Contains(err.Error(), "removing container") {
		t.Errorf("error should mention removing container, got: %s", err.Error())
	}
}

func TestDownInspectError(t *testing.T) {
	cli := &mockDockerClient{
		containers: client.ContainerListResult{
			Items: []container.Summary{
				{ID: "abc123def456", Image: "my-image:latest"},
			},
		},
		inspectErr: fmt.Errorf("inspect failed"),
	}
	err := Down(context.Background(), cli, ".")
	if err == nil {
		t.Fatal("expected error when inspect fails")
	}
	if !strings.Contains(err.Error(), "inspecting container") {
		t.Errorf("error should mention inspecting container, got: %s", err.Error())
	}
}

func TestDownContainerListError(t *testing.T) {
	cli := &mockDockerClient{
		containerListErr: fmt.Errorf("list failed"),
	}
	err := Down(context.Background(), cli, ".")
	if err == nil {
		t.Fatal("expected error when container list fails")
	}
	if !strings.Contains(err.Error(), "listing containers") {
		t.Errorf("error should mention listing containers, got: %s", err.Error())
	}
}

func TestShortID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"abc123def456789", "abc123def456"},
		{"short", "short"},
		{"", ""},
		{"abc123def456", "abc123def456"},
	}
	for _, tt := range tests {
		got := shortID(tt.input)
		if got != tt.want {
			t.Errorf("shortID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGatewayIP(t *testing.T) {
	// Test GatewayIP with a valid gateway IP in the container's network settings.
	gatewayIP, err := netip.ParseAddr("172.18.0.1")
	if err != nil {
		t.Fatalf("parsing gateway IP: %v", err)
	}

	cli := &mockDockerClient{
		inspectResult: client.ContainerInspectResult{
			Container: container.InspectResponse{
				NetworkSettings: &container.NetworkSettings{
					Networks: map[string]*network.EndpointSettings{
						"bridge": {
							Gateway: gatewayIP,
						},
					},
				},
			},
		},
	}

	got, err := GatewayIP(context.Background(), cli, "abc123")
	if err != nil {
		t.Fatalf("GatewayIP() error: %v", err)
	}
	if got != "172.18.0.1" {
		t.Errorf("GatewayIP() = %q, want %q", got, "172.18.0.1")
	}
}

func TestGatewayIPNoNetwork(t *testing.T) {
	// Test GatewayIP when the container has no network settings.
	cli := &mockDockerClient{
		inspectResult: client.ContainerInspectResult{
			Container: container.InspectResponse{
				NetworkSettings: &container.NetworkSettings{
					Networks: map[string]*network.EndpointSettings{},
				},
			},
		},
	}

	_, err := GatewayIP(context.Background(), cli, "abc123")
	if err == nil {
		t.Fatal("expected error when no gateway IP found")
	}
	if !strings.Contains(err.Error(), "no gateway IP found") {
		t.Errorf("error should mention no gateway IP found, got: %s", err.Error())
	}
}

func TestGatewayIPInspectError(t *testing.T) {
	cli := &mockDockerClient{
		inspectErr: fmt.Errorf("inspect failed"),
	}

	_, err := GatewayIP(context.Background(), cli, "abc123")
	if err == nil {
		t.Fatal("expected error when inspect fails")
	}
	if !strings.Contains(err.Error(), "inspecting container") {
		t.Errorf("error should mention inspecting container, got: %s", err.Error())
	}
}

func TestMkdirInContainer(t *testing.T) {
	// Test successful mkdir.
	cli := &mockDockerClient{
		execInspectResult: client.ExecInspectResult{ExitCode: 0},
	}

	err := MkdirInContainer(context.Background(), cli, "abc123", "/opt/dcx/gh-proxy")
	if err != nil {
		t.Fatalf("MkdirInContainer() error: %v", err)
	}
}

func TestMkdirInContainerNonZeroExit(t *testing.T) {
	// Test mkdir with non-zero exit code (e.g. permission denied).
	cli := &mockDockerClient{
		execInspectResult: client.ExecInspectResult{ExitCode: 1},
	}

	err := MkdirInContainer(context.Background(), cli, "abc123", "/opt/dcx/gh-proxy")
	if err == nil {
		t.Fatal("expected error when mkdir exits non-zero")
	}
	if !strings.Contains(err.Error(), "exited with code") {
		t.Errorf("error should mention exit code, got: %s", err.Error())
	}
}

func TestMkdirInContainerCreateError(t *testing.T) {
	cli := &mockDockerClient{
		execCreateErr: fmt.Errorf("exec create failed"),
	}

	err := MkdirInContainer(context.Background(), cli, "abc123", "/opt/dcx/gh-proxy")
	if err == nil {
		t.Fatal("expected error when exec create fails")
	}
	if !strings.Contains(err.Error(), "creating mkdir exec") {
		t.Errorf("error should mention creating mkdir exec, got: %s", err.Error())
	}
}

func TestIsMissingDockerConfigDir(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "missing docker config dir",
			err:  fmt.Errorf("creating Docker client: load config: default values: current docker host: current context: load docker config: config path: config dir: file does not exist (/home/vscode/.docker)"),
			want: true,
		},
		{
			name: "missing docker config dir with DOCKER_CONFIG override",
			err:  fmt.Errorf("load config: config path: config dir: file does not exist (/custom/.docker)"),
			want: true,
		},
		{
			name: "unrelated file does not exist",
			err:  fmt.Errorf("file does not exist (/tmp/other)"),
			want: false,
		},
		{
			name: "unrelated error",
			err:  fmt.Errorf("connection refused"),
			want: false,
		},
		{
			name: "docker error without missing file",
			err:  fmt.Errorf("docker daemon not ready"),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isMissingDockerConfigDir(tt.err)
			if got != tt.want {
				t.Errorf("isMissingDockerConfigDir() = %v, want %v", got, tt.want)
			}
		})
	}
}
