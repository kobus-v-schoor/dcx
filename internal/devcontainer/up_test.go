package devcontainer

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/kobus-v-schoor/dcx/internal/devcontainer/spec"
	"github.com/kobus-v-schoor/dcx/internal/docker"
	dockerspec "github.com/moby/docker-image-spec/specs-go/v1"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// mockClient implements docker.DockerClient for testing.
type mockClient struct {
	listResult         client.ContainerListResult
	listErr            error
	createResult       client.ContainerCreateResult
	createErr          error
	startErr           error
	stopErr            error
	removeErr          error
	inspectResult      client.ContainerInspectResult
	inspectErr         error
	imageInspectResult client.ImageInspectResult
	imageInspectErr    error

	lastCreateOptions client.ContainerCreateOptions
}

var _ docker.DockerClient = &mockClient{}

func (m *mockClient) Ping(_ context.Context, _ client.PingOptions) (client.PingResult, error) {
	return client.PingResult{}, nil
}

func (m *mockClient) ContainerList(_ context.Context, _ client.ContainerListOptions) (client.ContainerListResult, error) {
	return m.listResult, m.listErr
}

func (m *mockClient) ContainerInspect(_ context.Context, _ string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	return m.inspectResult, m.inspectErr
}

func (m *mockClient) ContainerStop(_ context.Context, _ string, _ client.ContainerStopOptions) (client.ContainerStopResult, error) {
	return client.ContainerStopResult{}, m.stopErr
}

func (m *mockClient) ContainerRemove(_ context.Context, _ string, _ client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
	return client.ContainerRemoveResult{}, m.removeErr
}

func (m *mockClient) ImageRemove(_ context.Context, _ string, _ client.ImageRemoveOptions) (client.ImageRemoveResult, error) {
	return client.ImageRemoveResult{}, nil
}

func (m *mockClient) VolumeRemove(_ context.Context, _ string, _ client.VolumeRemoveOptions) (client.VolumeRemoveResult, error) {
	return client.VolumeRemoveResult{}, nil
}

func (m *mockClient) CopyToContainer(_ context.Context, _ string, _ client.CopyToContainerOptions) (client.CopyToContainerResult, error) {
	return client.CopyToContainerResult{}, nil
}

func (m *mockClient) ExecCreate(_ context.Context, _ string, _ client.ExecCreateOptions) (client.ExecCreateResult, error) {
	return client.ExecCreateResult{}, nil
}

func (m *mockClient) ExecAttach(_ context.Context, _ string, _ client.ExecAttachOptions) (client.ExecAttachResult, error) {
	return client.ExecAttachResult{}, nil
}

func (m *mockClient) ExecStart(_ context.Context, _ string, _ client.ExecStartOptions) (client.ExecStartResult, error) {
	return client.ExecStartResult{}, nil
}

func (m *mockClient) ExecInspect(_ context.Context, _ string, _ client.ExecInspectOptions) (client.ExecInspectResult, error) {
	return client.ExecInspectResult{}, nil
}

func (m *mockClient) ImagePull(_ context.Context, _ string, _ client.ImagePullOptions) (client.ImagePullResponse, error) {
	return nil, nil
}

func (m *mockClient) ImageBuild(_ context.Context, _ io.Reader, _ client.ImageBuildOptions) (client.ImageBuildResult, error) {
	return client.ImageBuildResult{}, nil
}

func (m *mockClient) ImageInspect(_ context.Context, _ string, _ ...client.ImageInspectOption) (client.ImageInspectResult, error) {
	return m.imageInspectResult, m.imageInspectErr
}

func (m *mockClient) ImageTag(_ context.Context, _ client.ImageTagOptions) (client.ImageTagResult, error) {
	return client.ImageTagResult{}, nil
}

func (m *mockClient) ImageList(_ context.Context, _ client.ImageListOptions) (client.ImageListResult, error) {
	return client.ImageListResult{}, nil
}

func (m *mockClient) Close() error { return nil }

func (m *mockClient) ContainerCreate(_ context.Context, options client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
	m.lastCreateOptions = options
	return m.createResult, m.createErr
}

func (m *mockClient) ContainerStart(_ context.Context, _ string, _ client.ContainerStartOptions) (client.ContainerStartResult, error) {
	return client.ContainerStartResult{}, m.startErr
}

func TestUpNoExistingContainer(t *testing.T) {
	cli := &mockClient{
		listResult:   client.ContainerListResult{Items: []container.Summary{}},
		createResult: client.ContainerCreateResult{ID: "new123", Warnings: []string{}},
	}

	cfg := &spec.Config{
		Image:           "debian:stable-slim",
		WorkspaceFolder: "/workspace",
		WorkspaceMount:  "source=${localWorkspaceFolder},target=/workspace,type=bind",
		ContainerEnv: map[string]string{
			"TEST_VAR": "hello",
		},
	}

	id, err := Up(context.Background(), cli, cfg, "/tmp/workspace", "debian:stable-slim", false)
	if err != nil {
		t.Fatalf("Up error: %v", err)
	}
	if id != "new123" {
		t.Errorf("id = %q, want %q", id, "new123")
	}

	// Verify container config.
	if cli.lastCreateOptions.Config == nil {
		t.Fatal("expected Config to be set")
	}
	c := cli.lastCreateOptions.Config
	if c.Image != "debian:stable-slim" {
		t.Errorf("Image = %q, want %q", c.Image, "debian:stable-slim")
	}
	if c.Labels[docker.DevcontainerLabel] == "" {
		t.Errorf("label %q not set", docker.DevcontainerLabel)
	}
	if c.Labels["devcontainer.metadata"] == "" {
		t.Errorf("label devcontainer.metadata not set")
	}
	if !sliceContains(c.Env, "TEST_VAR=hello") {
		t.Errorf("Env missing TEST_VAR=hello, got %v", c.Env)
	}

	// Verify overrideCommand.
	if len(c.Entrypoint) != 1 || c.Entrypoint[0] != "/bin/sh" {
		t.Errorf("Entrypoint = %v, want [/bin/sh]", c.Entrypoint)
	}
	if len(c.Cmd) != 3 || c.Cmd[0] != "-c" || c.Cmd[2] != "-" {
		t.Errorf("Cmd = %v, want [-c <script> -]", c.Cmd)
	}
}

func TestUpExistingRunningNoRebuild(t *testing.T) {
	cli := &mockClient{
		listResult: client.ContainerListResult{
			Items: []container.Summary{
				{ID: "existing456", State: container.StateRunning},
			},
		},
	}

	cfg := &spec.Config{Image: "debian:stable-slim", WorkspaceFolder: "/workspace"}
	id, err := Up(context.Background(), cli, cfg, "/tmp/workspace", "debian:stable-slim", false)
	if err != nil {
		t.Fatalf("Up error: %v", err)
	}
	if id != "existing456" {
		t.Errorf("id = %q, want %q", id, "existing456")
	}
	if cli.lastCreateOptions.Config != nil {
		t.Error("expected ContainerCreate to NOT be called")
	}
}

func TestUpExistingRunningRebuild(t *testing.T) {
	cli := &mockClient{
		listResult: client.ContainerListResult{
			Items: []container.Summary{
				{ID: "old789", State: container.StateRunning},
			},
		},
		createResult: client.ContainerCreateResult{ID: "new789", Warnings: []string{}},
	}

	cfg := &spec.Config{Image: "debian:stable-slim", WorkspaceFolder: "/workspace"}
	id, err := Up(context.Background(), cli, cfg, "/tmp/workspace", "debian:stable-slim", true)
	if err != nil {
		t.Fatalf("Up error: %v", err)
	}
	if id != "new789" {
		t.Errorf("id = %q, want %q", id, "new789")
	}
	if cli.lastCreateOptions.Config == nil {
		t.Error("expected ContainerCreate to be called")
	}
}

func TestUpExistingStoppedNoRebuild(t *testing.T) {
	cli := &mockClient{
		listResult: client.ContainerListResult{
			Items: []container.Summary{
				{ID: "stopped111", State: container.StateExited},
			},
		},
	}

	cfg := &spec.Config{Image: "debian:stable-slim", WorkspaceFolder: "/workspace"}
	id, err := Up(context.Background(), cli, cfg, "/tmp/workspace", "debian:stable-slim", false)
	if err != nil {
		t.Fatalf("Up error: %v", err)
	}
	if id != "stopped111" {
		t.Errorf("id = %q, want %q", id, "stopped111")
	}
	if cli.lastCreateOptions.Config != nil {
		t.Error("expected ContainerCreate to NOT be called for stopped container")
	}
}

func TestUpMetadataLabelContainsRemoteUser(t *testing.T) {
	cli := &mockClient{
		listResult:   client.ContainerListResult{Items: []container.Summary{}},
		createResult: client.ContainerCreateResult{ID: "meta222", Warnings: []string{}},
	}

	cfg := &spec.Config{
		Image:           "debian:stable-slim",
		WorkspaceFolder: "/workspace",
		RemoteUser:      "vscode",
		ContainerUser:   "root",
		ContainerEnv:    map[string]string{"FOO": "bar"},
	}

	_, err := Up(context.Background(), cli, cfg, "/tmp/workspace", "debian:stable-slim", false)
	if err != nil {
		t.Fatalf("Up error: %v", err)
	}

	metaJSON := cli.lastCreateOptions.Config.Labels["devcontainer.metadata"]
	if metaJSON == "" {
		t.Fatal("devcontainer.metadata label not set")
	}
	if !strings.Contains(metaJSON, `"remoteUser":"vscode"`) {
		t.Errorf("metadata missing remoteUser: %s", metaJSON)
	}
	if !strings.Contains(metaJSON, `"containerUser":"root"`) {
		t.Errorf("metadata missing containerUser: %s", metaJSON)
	}
}

func TestUpMergesImageMetadata(t *testing.T) {
	cli := &mockClient{
		listResult:   client.ContainerListResult{Items: []container.Summary{}},
		createResult: client.ContainerCreateResult{ID: "meta333", Warnings: []string{}},
		imageInspectResult: client.ImageInspectResult{
			InspectResponse: image.InspectResponse{
				Config: &dockerspec.DockerOCIImageConfig{
					ImageConfig: ocispec.ImageConfig{
						Labels: map[string]string{
							"devcontainer.metadata": `[{"remoteUser":"baseuser","id":"base"}]`,
						},
					},
				},
			},
		},
	}

	cfg := &spec.Config{
		Image:           "myimage",
		WorkspaceFolder: "/workspace",
		RemoteUser:      "vscode",
	}

	_, err := Up(context.Background(), cli, cfg, "/tmp/workspace", "myimage", false)
	if err != nil {
		t.Fatalf("Up error: %v", err)
	}

	metaJSON := cli.lastCreateOptions.Config.Labels["devcontainer.metadata"]
	if !strings.Contains(metaJSON, `"remoteUser":"baseuser"`) {
		t.Errorf("metadata missing image remoteUser: %s", metaJSON)
	}
	if !strings.Contains(metaJSON, `"remoteUser":"vscode"`) {
		t.Errorf("metadata missing config remoteUser: %s", metaJSON)
	}
}

func TestUpWorkspaceMountInHostConfig(t *testing.T) {
	cli := &mockClient{
		listResult:   client.ContainerListResult{Items: []container.Summary{}},
		createResult: client.ContainerCreateResult{ID: "mount444", Warnings: []string{}},
	}

	cfg := &spec.Config{
		Image:           "debian:stable-slim",
		WorkspaceFolder: "/workspace",
		WorkspaceMount:  "source=${localWorkspaceFolder},target=/workspace,type=bind",
	}

	_, err := Up(context.Background(), cli, cfg, "/tmp/workspace", "debian:stable-slim", false)
	if err != nil {
		t.Fatalf("Up error: %v", err)
	}

	hc := cli.lastCreateOptions.HostConfig
	if hc == nil {
		t.Fatal("expected HostConfig to be set")
	}

	foundWorkspace := false
	for _, m := range hc.Mounts {
		if m.Target == "/workspace" && m.Type == mount.TypeBind {
			foundWorkspace = true
			break
		}
	}
	if !foundWorkspace {
		t.Errorf("workspace mount not found in HostConfig.Mounts: %v", hc.Mounts)
	}
}

func TestUpRunArgsPortBinding(t *testing.T) {
	cli := &mockClient{
		listResult:   client.ContainerListResult{Items: []container.Summary{}},
		createResult: client.ContainerCreateResult{ID: "port555", Warnings: []string{}},
	}

	cfg := &spec.Config{
		Image:           "debian:stable-slim",
		WorkspaceFolder: "/workspace",
		RunArgs:         []string{"-p", "8080:80"},
	}

	_, err := Up(context.Background(), cli, cfg, "/tmp/workspace", "debian:stable-slim", false)
	if err != nil {
		t.Fatalf("Up error: %v", err)
	}

	hc := cli.lastCreateOptions.HostConfig
	if hc == nil {
		t.Fatal("expected HostConfig to be set")
	}
	if len(hc.PortBindings) == 0 {
		t.Error("expected PortBindings to be set")
	}
}

func TestUpUnsupportedRunArg(t *testing.T) {
	cli := &mockClient{listResult: client.ContainerListResult{Items: []container.Summary{}}}

	cfg := &spec.Config{
		Image:           "debian:stable-slim",
		WorkspaceFolder: "/workspace",
		RunArgs:         []string{"--gpus", "all"},
	}

	_, err := Up(context.Background(), cli, cfg, "/tmp/workspace", "debian:stable-slim", false)
	if err == nil {
		t.Fatal("expected error for unsupported runArg")
	}
	if !strings.Contains(err.Error(), "unsupported runArg") {
		t.Errorf("expected unsupported runArg error, got: %v", err)
	}
}

func TestUpOverrideCommandFalse(t *testing.T) {
	f := false
	cli := &mockClient{
		listResult:   client.ContainerListResult{Items: []container.Summary{}},
		createResult: client.ContainerCreateResult{ID: "cmd666", Warnings: []string{}},
	}

	cfg := &spec.Config{
		Image:           "debian:stable-slim",
		WorkspaceFolder: "/workspace",
		OverrideCommand: &f,
	}

	_, err := Up(context.Background(), cli, cfg, "/tmp/workspace", "debian:stable-slim", false)
	if err != nil {
		t.Fatalf("Up error: %v", err)
	}

	c := cli.lastCreateOptions.Config
	if len(c.Entrypoint) > 0 {
		t.Errorf("expected empty Entrypoint when overrideCommand=false, got %v", c.Entrypoint)
	}
	if len(c.Cmd) > 0 {
		t.Errorf("expected empty Cmd when overrideCommand=false, got %v", c.Cmd)
	}
}

func sliceContains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
