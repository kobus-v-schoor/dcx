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
	"github.com/moby/moby/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// mockClient implements docker.DockerClient for testing.
type mockClient struct {
	listResult         client.ContainerListResult
	listErr            error
	startErr           error
	stopErr            error
	removeErr          error
	inspectResult      client.ContainerInspectResult
	inspectErr         error
	imageInspectResult client.ImageInspectResult
	imageInspectErr    error
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

func (m *mockClient) ContainerCreate(_ context.Context, _ client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
	return client.ContainerCreateResult{}, nil
}

func (m *mockClient) ContainerStart(_ context.Context, _ string, _ client.ContainerStartOptions) (client.ContainerStartResult, error) {
	return client.ContainerStartResult{}, m.startErr
}

// createCapture holds the arguments passed to the mocked createContainer.
type createCapture struct {
	imageRef   string
	runArgs    []string
	mounts     []string
	envs       []string
	labels     map[string]string
	user       string
	workdir    string
	entrypoint string
	cmdArgs    []string
}

func setupMockCreate(returnID string, cap *createCapture) func() {
	orig := createContainer
	createContainer = func(_ context.Context, imageRef string, runArgs, mounts, envs []string, labels map[string]string, user, workdir, entrypoint string, cmdArgs []string) (string, error) {
		if cap != nil {
			cap.imageRef = imageRef
			cap.runArgs = runArgs
			cap.mounts = mounts
			cap.envs = envs
			cap.labels = labels
			cap.user = user
			cap.workdir = workdir
			cap.entrypoint = entrypoint
			cap.cmdArgs = cmdArgs
		}
		return returnID, nil
	}
	return func() { createContainer = orig }
}

func setupMockStart(err error) func() {
	orig := startContainer
	startContainer = func(_ context.Context, _ string) error { return err }
	return func() { startContainer = orig }
}

func TestUpNoExistingContainer(t *testing.T) {
	var cap createCapture
	cleanup := setupMockCreate("new123", &cap)
	defer cleanup()
	defer setupMockStart(nil)()

	cli := &mockClient{
		listResult: client.ContainerListResult{Items: []container.Summary{}},
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

	if cap.imageRef != "debian:stable-slim" {
		t.Errorf("imageRef = %q, want %q", cap.imageRef, "debian:stable-slim")
	}
	if cap.labels[docker.DevcontainerLabel] == "" {
		t.Errorf("label %q not set", docker.DevcontainerLabel)
	}
	if cap.labels["devcontainer.metadata"] == "" {
		t.Errorf("label devcontainer.metadata not set")
	}
	if !sliceContains(cap.envs, "TEST_VAR=hello") {
		t.Errorf("envs missing TEST_VAR=hello, got %v", cap.envs)
	}
	if cap.entrypoint != "/bin/sh" {
		t.Errorf("entrypoint = %q, want %q", cap.entrypoint, "/bin/sh")
	}
	if len(cap.cmdArgs) != 3 || cap.cmdArgs[0] != "-c" || cap.cmdArgs[2] != "-" {
		t.Errorf("cmdArgs = %v, want [-c <script> -]", cap.cmdArgs)
	}
}

func TestUpExistingRunningNoRebuild(t *testing.T) {
	var createCalled bool
	cleanup := setupMockCreate("", nil)
	defer cleanup()
	defer setupMockStart(nil)()
	createContainer = func(_ context.Context, _ string, _, _, _ []string, _ map[string]string, _, _, _ string, _ []string) (string, error) {
		createCalled = true
		return "", nil
	}

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
	if createCalled {
		t.Error("expected createContainer to NOT be called")
	}
}

func TestUpExistingRunningRebuild(t *testing.T) {
	var cap createCapture
	cleanup := setupMockCreate("new789", &cap)
	defer cleanup()
	defer setupMockStart(nil)()

	cli := &mockClient{
		listResult: client.ContainerListResult{
			Items: []container.Summary{
				{ID: "old789", State: container.StateRunning},
			},
		},
	}

	cfg := &spec.Config{Image: "debian:stable-slim", WorkspaceFolder: "/workspace"}
	id, err := Up(context.Background(), cli, cfg, "/tmp/workspace", "debian:stable-slim", true)
	if err != nil {
		t.Fatalf("Up error: %v", err)
	}
	if id != "new789" {
		t.Errorf("id = %q, want %q", id, "new789")
	}
	if cap.imageRef == "" {
		t.Error("expected createContainer to be called")
	}
}

func TestUpExistingStoppedNoRebuild(t *testing.T) {
	var createCalled bool
	cleanup := setupMockCreate("", nil)
	defer cleanup()
	defer setupMockStart(nil)()
	createContainer = func(_ context.Context, _ string, _, _, _ []string, _ map[string]string, _, _, _ string, _ []string) (string, error) {
		createCalled = true
		return "", nil
	}

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
	if createCalled {
		t.Error("expected createContainer to NOT be called for stopped container")
	}
}

func TestUpMetadataLabelContainsRemoteUser(t *testing.T) {
	var cap createCapture
	cleanup := setupMockCreate("meta222", &cap)
	defer cleanup()
	defer setupMockStart(nil)()

	cli := &mockClient{
		listResult: client.ContainerListResult{Items: []container.Summary{}},
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

	metaJSON := cap.labels["devcontainer.metadata"]
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
	var cap createCapture
	cleanup := setupMockCreate("meta333", &cap)
	defer cleanup()
	defer setupMockStart(nil)()

	cli := &mockClient{
		listResult: client.ContainerListResult{Items: []container.Summary{}},
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

	metaJSON := cap.labels["devcontainer.metadata"]
	if !strings.Contains(metaJSON, `"remoteUser":"baseuser"`) {
		t.Errorf("metadata missing image remoteUser: %s", metaJSON)
	}
	if !strings.Contains(metaJSON, `"remoteUser":"vscode"`) {
		t.Errorf("metadata missing config remoteUser: %s", metaJSON)
	}
}

func TestUpWorkspaceMountPassed(t *testing.T) {
	var cap createCapture
	cleanup := setupMockCreate("mount444", &cap)
	defer cleanup()
	defer setupMockStart(nil)()

	cli := &mockClient{
		listResult: client.ContainerListResult{Items: []container.Summary{}},
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

	foundWorkspace := false
	for _, m := range cap.mounts {
		if strings.Contains(m, "/workspace") && strings.HasPrefix(m, "type=bind,") {
			foundWorkspace = true
			break
		}
	}
	if !foundWorkspace {
		t.Errorf("workspace mount not found in mounts: %v", cap.mounts)
	}
}

func TestUpRunArgsPassedVerbatim(t *testing.T) {
	var cap createCapture
	cleanup := setupMockCreate("port555", &cap)
	defer cleanup()
	defer setupMockStart(nil)()

	cli := &mockClient{
		listResult: client.ContainerListResult{Items: []container.Summary{}},
	}

	cfg := &spec.Config{
		Image:           "debian:stable-slim",
		WorkspaceFolder: "/workspace",
		RunArgs:         []string{"-p", "8080:80", "--network", "host"},
	}

	_, err := Up(context.Background(), cli, cfg, "/tmp/workspace", "debian:stable-slim", false)
	if err != nil {
		t.Fatalf("Up error: %v", err)
	}

	// Verify runArgs appear verbatim in the passed slice.
	if !sliceContains(cap.runArgs, "-p") {
		t.Error("expected runArgs to contain -p")
	}
	if !sliceContains(cap.runArgs, "8080:80") {
		t.Error("expected runArgs to contain 8080:80")
	}
	if !sliceContains(cap.runArgs, "--network") {
		t.Error("expected runArgs to contain --network")
	}
	if !sliceContains(cap.runArgs, "host") {
		t.Error("expected runArgs to contain host")
	}
}

func TestUpUnsupportedRunArgNoError(t *testing.T) {
	// With CLI-based creation, all runArgs are passed verbatim to docker
	// create, so there is no "unsupported runArg" error from dcx itself.
	var cap createCapture
	cleanup := setupMockCreate("gpus777", &cap)
	defer cleanup()
	defer setupMockStart(nil)()

	cli := &mockClient{listResult: client.ContainerListResult{Items: []container.Summary{}}}

	cfg := &spec.Config{
		Image:           "debian:stable-slim",
		WorkspaceFolder: "/workspace",
		RunArgs:         []string{"--gpus", "all"},
	}

	_, err := Up(context.Background(), cli, cfg, "/tmp/workspace", "debian:stable-slim", false)
	if err != nil {
		t.Fatalf("Up should not error for previously unsupported runArgs: %v", err)
	}
	if !sliceContains(cap.runArgs, "--gpus") {
		t.Error("expected runArgs to contain --gpus")
	}
}

func TestUpOverrideCommandFalse(t *testing.T) {
	f := false
	var cap createCapture
	cleanup := setupMockCreate("cmd666", &cap)
	defer cleanup()
	defer setupMockStart(nil)()

	cli := &mockClient{
		listResult: client.ContainerListResult{Items: []container.Summary{}},
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

	if cap.entrypoint != "" {
		t.Errorf("expected empty entrypoint when overrideCommand=false, got %q", cap.entrypoint)
	}
	if len(cap.cmdArgs) > 0 {
		t.Errorf("expected empty cmdArgs when overrideCommand=false, got %v", cap.cmdArgs)
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
