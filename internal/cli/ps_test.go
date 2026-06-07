package cli

import (
	"bytes"
	"context"
	"fmt"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// mockPsClient is a minimal test double that satisfies docker.DockerClient.
type mockPsClient struct {
	devcontainers     client.ContainerListResult
	devcontainerErr   error
	composeContainers client.ContainerListResult
	composeListErr    error
}

func (m *mockPsClient) Ping(_ context.Context, _ client.PingOptions) (client.PingResult, error) {
	return client.PingResult{}, nil
}

func (m *mockPsClient) ContainerList(_ context.Context, opts client.ContainerListOptions) (client.ContainerListResult, error) {
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

func (m *mockPsClient) ContainerInspect(_ context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	return client.ContainerInspectResult{}, nil
}

func (m *mockPsClient) ContainerStop(_ context.Context, _ string, _ client.ContainerStopOptions) (client.ContainerStopResult, error) {
	return client.ContainerStopResult{}, nil
}

func (m *mockPsClient) ContainerRemove(_ context.Context, _ string, _ client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
	return client.ContainerRemoveResult{}, nil
}

func (m *mockPsClient) ImageRemove(_ context.Context, _ string, _ client.ImageRemoveOptions) (client.ImageRemoveResult, error) {
	return client.ImageRemoveResult{}, nil
}

func (m *mockPsClient) VolumeRemove(_ context.Context, _ string, _ client.VolumeRemoveOptions) (client.VolumeRemoveResult, error) {
	return client.VolumeRemoveResult{}, nil
}

func (m *mockPsClient) CopyToContainer(_ context.Context, _ string, _ client.CopyToContainerOptions) (client.CopyToContainerResult, error) {
	return client.CopyToContainerResult{}, nil
}

func (m *mockPsClient) ExecCreate(_ context.Context, _ string, _ client.ExecCreateOptions) (client.ExecCreateResult, error) {
	return client.ExecCreateResult{ID: "exec123"}, nil
}

func (m *mockPsClient) ExecStart(_ context.Context, _ string, _ client.ExecStartOptions) (client.ExecStartResult, error) {
	return client.ExecStartResult{}, nil
}

func (m *mockPsClient) ExecInspect(_ context.Context, _ string, _ client.ExecInspectOptions) (client.ExecInspectResult, error) {
	return client.ExecInspectResult{ExitCode: 0}, nil
}

func (m *mockPsClient) Close() error {
	return nil
}

func TestFormatName(t *testing.T) {
	tests := []struct {
		name string
		ctr  container.Summary
		want string
	}{
		{
			name: "docker name",
			ctr: container.Summary{
				ID:    "abc123def4567890123456789012",
				Names: []string{"/mycontainer"},
			},
			want: "mycontainer",
		},
		{
			name: "no name falls back to short id",
			ctr: container.Summary{
				ID: "abc123def4567890123456789012",
			},
			want: "abc123def456",
		},
		{
			name: "empty name slice falls back to short id",
			ctr: container.Summary{
				ID:    "abc123def4567890123456789012",
				Names: []string{},
			},
			want: "abc123def456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatName(tt.ctr)
			if got != tt.want {
				t.Errorf("formatName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatService(t *testing.T) {
	if got := formatService(container.Summary{Labels: map[string]string{"com.docker.compose.service": "web"}}); got != "web" {
		t.Errorf("formatService() = %q, want web", got)
	}
	if got := formatService(container.Summary{}); got != "-" {
		t.Errorf("formatService() = %q, want -", got)
	}
}

func TestEllipsis(t *testing.T) {
	tests := []struct {
		s      string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello world", 8, "hello..."},
		{"hello world", 5, "he..."},
		{"hello world", 3, "..."},
		{"hello world", 2, "..."},
		{"日本語テスト", 5, "日本..."},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_%d", tt.s, tt.maxLen), func(t *testing.T) {
			got := ellipsis(tt.s, tt.maxLen)
			if got != tt.want {
				t.Errorf("ellipsis(%q, %d) = %q, want %q", tt.s, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestFormatCommand(t *testing.T) {
	got := formatCommand("/bin/bash")
	want := `"/bin/bash"`
	if got != want {
		t.Errorf("formatCommand() = %q, want %q", got, want)
	}

	got = formatCommand("/usr/bin/sleep infinity")
	want = `"/usr/bin/sleep in..."`
	if got != want {
		t.Errorf("formatCommand() = %q, want %q", got, want)
	}
}

func TestHumanDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{5 * time.Second, "5 seconds"},
		{1 * time.Second, "1 second"},
		{60 * time.Second, "1 minute"},
		{90 * time.Second, "1 minute"},
		{30 * time.Minute, "30 minutes"},
		{1 * time.Hour, "1 hour"},
		{3 * time.Hour, "3 hours"},
		{24 * time.Hour, "1 day"},
		{48 * time.Hour, "2 days"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := humanDuration(tt.d)
			if got != tt.want {
				t.Errorf("humanDuration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestFormatPorts(t *testing.T) {
	tests := []struct {
		name  string
		ports []container.PortSummary
		want  string
	}{
		{
			name:  "empty",
			ports: nil,
			want:  "",
		},
		{
			name: "published with default ip",
			ports: []container.PortSummary{
				{IP: netip.MustParseAddr("0.0.0.0"), PrivatePort: 8080, PublicPort: 80, Type: "tcp"},
			},
			want: "0.0.0.0:80->8080/tcp",
		},
		{
			name: "published with specific ip",
			ports: []container.PortSummary{
				{IP: netip.MustParseAddr("127.0.0.1"), PrivatePort: 5432, PublicPort: 5432, Type: "tcp"},
			},
			want: "127.0.0.1:5432->5432/tcp",
		},
		{
			name: "exposed only",
			ports: []container.PortSummary{
				{PrivatePort: 3000, Type: "tcp"},
			},
			want: "3000/tcp",
		},
		{
			name: "multiple sorted",
			ports: []container.PortSummary{
				{IP: netip.MustParseAddr("0.0.0.0"), PrivatePort: 443, PublicPort: 443, Type: "tcp"},
				{IP: netip.MustParseAddr("0.0.0.0"), PrivatePort: 80, PublicPort: 80, Type: "tcp"},
			},
			want: "0.0.0.0:80->80/tcp, 0.0.0.0:443->443/tcp",
		},
		{
			name: "mixed exposed and published",
			ports: []container.PortSummary{
				{PrivatePort: 8080, Type: "tcp"},
				{IP: netip.MustParseAddr("0.0.0.0"), PrivatePort: 80, PublicPort: 80, Type: "tcp"},
			},
			want: "0.0.0.0:80->80/tcp, 8080/tcp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatPorts(tt.ports)
			if got != tt.want {
				t.Errorf("formatPorts() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrintContainers(t *testing.T) {
	containers := []container.Summary{
		{
			ID:      "abc123",
			Names:   []string{"/web"},
			Labels:  map[string]string{"com.docker.compose.service": "web"},
			Image:   "nginx:latest",
			Command: "nginx -g 'daemon off;'",
			Status:  "Up 2 hours",
			Created: time.Now().Add(-2 * time.Hour).Unix(),
		},
		{
			ID:      "def456",
			Names:   []string{"/devcontainer"},
			Image:   "mcr.microsoft.com/devcontainers/base:debian",
			Command: "/bin/bash",
			Status:  "Up 5 minutes",
			Created: time.Now().Add(-5 * time.Minute).Unix(),
		},
	}

	var buf bytes.Buffer
	if err := printContainers(&buf, containers); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "NAME") {
		t.Errorf("output should contain header NAME, got:\n%s", output)
	}
	if !strings.Contains(output, "web") {
		t.Errorf("output should contain service web, got:\n%s", output)
	}
	if !strings.Contains(output, "devcontainer") {
		t.Errorf("output should contain container devcontainer, got:\n%s", output)
	}
	if !strings.Contains(output, "Up 2 hours") {
		t.Errorf("output should contain status Up 2 hours, got:\n%s", output)
	}
	if !strings.Contains(output, "nginx:latest") {
		t.Errorf("output should contain image nginx:latest, got:\n%s", output)
	}
}

func TestFindProjectContainersDevcontainerOnly(t *testing.T) {
	cli := &mockPsClient{
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

	containers, err := findProjectContainers(context.Background(), cli, "/foo")
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
	cli := &mockPsClient{
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

	containers, err := findProjectContainers(context.Background(), cli, "/foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(containers) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(containers))
	}

	// Should be sorted by name.
	if formatName(containers[0]) != "proj_web_1" {
		t.Errorf("first container name = %q, want proj_web_1", formatName(containers[0]))
	}
	if formatName(containers[1]) != "vsc-project" {
		t.Errorf("second container name = %q, want vsc-project", formatName(containers[1]))
	}
}

func TestFindProjectContainersDedup(t *testing.T) {
	// If the devcontainer is also returned by compose list, it should not
	// appear twice.
	cli := &mockPsClient{
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

	containers, err := findProjectContainers(context.Background(), cli, "/foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}
}

func TestFindProjectContainersListError(t *testing.T) {
	cli := &mockPsClient{
		devcontainerErr: fmt.Errorf("list failed"),
	}

	_, err := findProjectContainers(context.Background(), cli, "/foo")
	if err == nil {
		t.Fatal("expected error when container list fails")
	}
}
