package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/api/types/jsonstream"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

// mockDockerClient implements DockerClient for testing. Each field controls
// what the corresponding method returns. Unset fields return zero values and
// nil errors by default.
type mockDockerClient struct {
	pingErr            error
	containers         client.ContainerListResult
	containerListErr   error
	inspectResult      client.ContainerInspectResult
	inspectErr         error
	stopErr            error
	removeErr          error
	imageRemoveErr     error
	imageRemoveCount   int
	volumeRemoveErr    error
	copyErr            error
	execCreateErr      error
	execAttachErr      error
	execAttachResult   client.ExecAttachResult
	execStartErr       error
	execInspectErr     error
	execInspectResult  client.ExecInspectResult
	imagePullResult    client.ImagePullResponse
	imagePullErr       error
	imageBuildResult   client.ImageBuildResult
	imageBuildErr      error
	imageInspectResult client.ImageInspectResult
	imageInspectErr    error
	imageTagErr        error
	imageListResult    client.ImageListResult
	imageListErr       error
	closed             bool
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
	m.imageRemoveCount++
	return client.ImageRemoveResult{}, m.imageRemoveErr
}

func (m *mockDockerClient) VolumeRemove(_ context.Context, _ string, _ client.VolumeRemoveOptions) (client.VolumeRemoveResult, error) {
	return client.VolumeRemoveResult{}, m.volumeRemoveErr
}

func (m *mockDockerClient) CopyToContainer(_ context.Context, _ string, _ client.CopyToContainerOptions) (client.CopyToContainerResult, error) {
	return client.CopyToContainerResult{}, m.copyErr
}

func (m *mockDockerClient) ExecCreate(_ context.Context, _ string, _ client.ExecCreateOptions) (client.ExecCreateResult, error) {
	return client.ExecCreateResult{ID: "exec123"}, m.execCreateErr
}

func (m *mockDockerClient) ExecAttach(_ context.Context, _ string, _ client.ExecAttachOptions) (client.ExecAttachResult, error) {
	return m.execAttachResult, m.execAttachErr
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

func (m *mockDockerClient) ImagePull(_ context.Context, _ string, _ client.ImagePullOptions) (client.ImagePullResponse, error) {
	return m.imagePullResult, m.imagePullErr
}

func (m *mockDockerClient) ImageBuild(_ context.Context, _ io.Reader, _ client.ImageBuildOptions) (client.ImageBuildResult, error) {
	return m.imageBuildResult, m.imageBuildErr
}

func (m *mockDockerClient) ImageInspect(_ context.Context, _ string, _ ...client.ImageInspectOption) (client.ImageInspectResult, error) {
	return m.imageInspectResult, m.imageInspectErr
}

func (m *mockDockerClient) ImageTag(_ context.Context, _ client.ImageTagOptions) (client.ImageTagResult, error) {
	return client.ImageTagResult{}, m.imageTagErr
}

func (m *mockDockerClient) ImageList(_ context.Context, _ client.ImageListOptions) (client.ImageListResult, error) {
	return m.imageListResult, m.imageListErr
}

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
	if err != nil {
		t.Fatalf("expected no error when no container found, got %v", err)
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
		got := ShortID(tt.input)
		if got != tt.want {
			t.Errorf("ShortID(%q) = %q, want %q", tt.input, got, tt.want)
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

func TestCheckStaleMountsNoStale(t *testing.T) {
	tmpDir := t.TempDir()
	cli := &mockDockerClient{
		inspectResult: client.ContainerInspectResult{
			Container: container.InspectResponse{
				Mounts: []container.MountPoint{
					{Type: mount.TypeBind, Source: tmpDir, Destination: "/dest"},
					{Type: mount.TypeVolume, Source: "vol1", Destination: "/vol"},
				},
			},
		},
	}
	err := CheckStaleMounts(context.Background(), cli, "abc123")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestCheckStaleMountsReturnsError(t *testing.T) {
	cli := &mockDockerClient{
		inspectResult: client.ContainerInspectResult{
			Container: container.InspectResponse{
				Mounts: []container.MountPoint{
					{Type: mount.TypeBind, Source: "/nonexistent/path/that/does/not/exist", Destination: "/dest"},
				},
			},
		},
	}
	err := CheckStaleMounts(context.Background(), cli, "abc123")
	if err == nil {
		t.Fatal("expected error when stale mount found")
	}
	if !strings.Contains(err.Error(), "stale bind mounts detected") {
		t.Errorf("error should mention stale bind mounts, got: %s", err.Error())
	}
}

func TestCheckStaleMountsInspectError(t *testing.T) {
	cli := &mockDockerClient{
		inspectErr: fmt.Errorf("inspect failed"),
	}
	err := CheckStaleMounts(context.Background(), cli, "abc123")
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

func TestMkdirInContainerExecCreateError(t *testing.T) {
	cli := &mockDockerClient{
		execCreateErr: fmt.Errorf("exec create failed"),
	}

	err := MkdirInContainer(context.Background(), cli, "abc123", "/opt/dcx/gh-proxy")
	if err == nil {
		t.Fatal("expected error when exec create fails")
	}
	if !strings.Contains(err.Error(), "creating exec in container") {
		t.Errorf("error should mention creating exec in container, got: %s", err.Error())
	}
}

func TestIsContainerRunning(t *testing.T) {
	tests := []struct {
		name  string
		state container.ContainerState
		want  bool
	}{
		{"running", container.StateRunning, true},
		{"exited", container.StateExited, false},
		{"paused", container.StatePaused, false},
		{"dead", container.StateDead, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctr := container.Summary{State: tt.state}
			got := IsContainerRunning(ctr)
			if got != tt.want {
				t.Errorf("IsContainerRunning() = %v, want %v", got, tt.want)
			}
		})
	}
}

type eofConn struct{}

func (eofConn) Read(b []byte) (int, error)       { return 0, io.EOF }
func (eofConn) Write(b []byte) (int, error)      { return len(b), nil }
func (eofConn) Close() error                     { return nil }
func (eofConn) LocalAddr() net.Addr              { return nil }
func (eofConn) RemoteAddr() net.Addr             { return nil }
func (eofConn) SetDeadline(time.Time) error      { return nil }
func (eofConn) SetReadDeadline(time.Time) error  { return nil }
func (eofConn) SetWriteDeadline(time.Time) error { return nil }

func TestExecInteractiveCreateError(t *testing.T) {
	cli := &mockDockerClient{execCreateErr: fmt.Errorf("create failed")}
	err := ExecInteractive(context.Background(), cli, "abc123", "", "/workspace", nil, []string{"bash"})
	if err == nil {
		t.Fatal("expected error when exec create fails")
	}
	if !strings.Contains(err.Error(), "creating exec in container") {
		t.Errorf("error should mention creating exec, got: %s", err.Error())
	}
}

func TestExecInteractiveAttachError(t *testing.T) {
	cli := &mockDockerClient{execAttachErr: fmt.Errorf("attach failed")}
	err := ExecInteractive(context.Background(), cli, "abc123", "", "/workspace", nil, []string{"bash"})
	if err == nil {
		t.Fatal("expected error when exec attach fails")
	}
	if !strings.Contains(err.Error(), "attaching to exec in container") {
		t.Errorf("error should mention attaching to exec, got: %s", err.Error())
	}
}

func TestExecInteractiveExitCodeError(t *testing.T) {
	cli := &mockDockerClient{
		execAttachResult: client.ExecAttachResult{
			HijackedResponse: client.NewHijackedResponse(eofConn{}, ""),
		},
		execInspectResult: client.ExecInspectResult{ExitCode: 42},
	}

	err := ExecInteractive(context.Background(), cli, "abc123", "", "/workspace", nil, []string{"bash"})
	if err == nil {
		t.Fatal("expected error when exec exits non-zero")
	}

	var exitErr *ExitCodeError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *ExitCodeError, got %T", err)
	}
	if exitErr.ExitCode != 42 {
		t.Errorf("expected exit code 42, got %d", exitErr.ExitCode)
	}
}

func TestExecInteractiveSuccess(t *testing.T) {
	cli := &mockDockerClient{
		execAttachResult: client.ExecAttachResult{
			HijackedResponse: client.NewHijackedResponse(eofConn{}, ""),
		},
		execInspectResult: client.ExecInspectResult{ExitCode: 0},
	}

	err := ExecInteractive(context.Background(), cli, "abc123", "", "/workspace", nil, []string{"bash"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
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

func TestImagePullIfMissingAlreadyPresent(t *testing.T) {
	cli := &mockDockerClient{
		imageInspectResult: client.ImageInspectResult{InspectResponse: image.InspectResponse{ID: "sha256:abc123"}},
	}
	err := ImagePullIfMissing(context.Background(), cli, "alpine:3.19", false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cli.imagePullResult != nil {
		t.Error("expected ImagePull to not be called when image is present")
	}
}

func TestImagePullIfMissingPulls(t *testing.T) {
	cli := &mockDockerClient{
		imageInspectErr: fmt.Errorf("not found"),
		imagePullResult: &mockImagePullResponse{
			ReadCloser: io.NopCloser(bytes.NewReader(nil)),
			waitErr:    nil,
		},
	}
	err := ImagePullIfMissing(context.Background(), cli, "alpine:3.19", false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestImagePullIfMissingPullWaitFails(t *testing.T) {
	cli := &mockDockerClient{
		imageInspectErr: fmt.Errorf("not found"),
		imagePullResult: &mockImagePullResponse{
			ReadCloser: io.NopCloser(bytes.NewReader(nil)),
			waitErr:    fmt.Errorf("pull failed"),
		},
	}
	err := ImagePullIfMissing(context.Background(), cli, "alpine:3.19", false)
	if err == nil {
		t.Fatal("expected error when pull wait fails")
	}
	if !strings.Contains(err.Error(), "waiting for image pull") {
		t.Errorf("error should mention waiting for image pull, got: %s", err.Error())
	}
}

func TestImagePullIfMissingForcePull(t *testing.T) {
	cli := &mockDockerClient{
		imageInspectResult: client.ImageInspectResult{InspectResponse: image.InspectResponse{ID: "sha256:abc123"}},
		imagePullResult: &mockImagePullResponse{
			ReadCloser: io.NopCloser(bytes.NewReader(nil)),
			waitErr:    nil,
		},
	}
	err := ImagePullIfMissing(context.Background(), cli, "alpine:3.19", true)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cli.imagePullResult == nil {
		t.Error("expected ImagePull to be called when force=true")
	}
}

func TestImageBuildFromDirSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	dockerfile := filepath.Join(tmpDir, "Dockerfile")
	if err := os.WriteFile(dockerfile, []byte("FROM alpine:3.19\n"), 0644); err != nil {
		t.Fatalf("writing Dockerfile: %v", err)
	}

	cli := &mockDockerClient{
		imageBuildResult: client.ImageBuildResult{
			Body: io.NopCloser(bytes.NewReader(nil)),
		},
		imageInspectResult: client.ImageInspectResult{InspectResponse: image.InspectResponse{ID: "sha256:built123"}},
	}

	opts := client.ImageBuildOptions{Tags: []string{"dcx-test:abc123"}}
	id, err := ImageBuildFromDir(context.Background(), cli, tmpDir, opts)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if id != "sha256:built123" {
		t.Errorf("expected id sha256:built123, got %s", id)
	}
}

func TestImageBuildFromDirStreamError(t *testing.T) {
	tmpDir := t.TempDir()
	dockerfile := filepath.Join(tmpDir, "Dockerfile")
	if err := os.WriteFile(dockerfile, []byte("FROM alpine:3.19\n"), 0644); err != nil {
		t.Fatalf("writing Dockerfile: %v", err)
	}

	body := `{"errorDetail":{"message":"syntax error in Dockerfile"}}`
	cli := &mockDockerClient{
		imageBuildResult: client.ImageBuildResult{
			Body: io.NopCloser(bytes.NewReader([]byte(body))),
		},
	}

	opts := client.ImageBuildOptions{Tags: []string{"dcx-test:abc123"}}
	_, err := ImageBuildFromDir(context.Background(), cli, tmpDir, opts)
	if err == nil {
		t.Fatal("expected error when build stream has error")
	}
	if !strings.Contains(err.Error(), "syntax error") {
		t.Errorf("error should mention syntax error, got: %s", err.Error())
	}
}

func TestImageBuildFromDirInspectFails(t *testing.T) {
	tmpDir := t.TempDir()
	dockerfile := filepath.Join(tmpDir, "Dockerfile")
	if err := os.WriteFile(dockerfile, []byte("FROM alpine:3.19\n"), 0644); err != nil {
		t.Fatalf("writing Dockerfile: %v", err)
	}

	cli := &mockDockerClient{
		imageBuildResult: client.ImageBuildResult{
			Body: io.NopCloser(bytes.NewReader(nil)),
		},
		imageInspectErr: fmt.Errorf("image not found"),
	}

	opts := client.ImageBuildOptions{Tags: []string{"dcx-test:abc123"}}
	_, err := ImageBuildFromDir(context.Background(), cli, tmpDir, opts)
	if err == nil {
		t.Fatal("expected error when inspect fails")
	}
	if !strings.Contains(err.Error(), "could not inspect image") {
		t.Errorf("error should mention inspect failure, got: %s", err.Error())
	}
}

func TestDownCleansUpDcxImages(t *testing.T) {
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
		imageListResult: client.ImageListResult{
			Items: []image.Summary{
				{
					ID:       "sha256:dcximg",
					RepoTags: []string{"dcx-myproject:deadbeef"},
				},
			},
		},
	}
	err := Down(context.Background(), cli, ".")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cli.imageRemoveCount != 2 {
		t.Errorf("expected ImageRemove to be called twice (container image + dcx image), got %d", cli.imageRemoveCount)
	}
}
