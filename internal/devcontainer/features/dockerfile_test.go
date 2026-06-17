package features

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kobus-v-schoor/dcx/internal/devcontainer/spec"
)

func TestDockerfileContainsAllFeatures(t *testing.T) {
	f0Dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(f0Dir, "install.sh"), []byte("#!/bin/bash\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f0Dir, "devcontainer-feature.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	f1Dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(f1Dir, "install.sh"), []byte("#!/bin/bash\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f1Dir, "devcontainer-feature.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	features := []ResolvedFeature{
		{
			Ref:  FeatureRef{ID: "github-cli"},
			Meta: FeatureMeta{ID: "github-cli", Name: "GitHub CLI"},
			Path: f0Dir,
		},
		{
			Ref:  FeatureRef{ID: "docker-in-docker"},
			Meta: FeatureMeta{ID: "docker-in-docker", Name: "Docker-in-Docker"},
			Path: f1Dir,
		},
	}

	ctxDir, dockerfilePath, err := BuildContext("mcr.microsoft.com/devcontainers/base:debian", features, "root", "root")
	if err != nil {
		t.Fatalf("BuildContext error: %v", err)
	}
	defer func() { _ = os.RemoveAll(ctxDir) }()

	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("reading Dockerfile: %v", err)
	}
	df := string(content)

	if !strings.Contains(df, "FROM mcr.microsoft.com/devcontainers/base:debian AS dcx_features") {
		t.Error("Dockerfile missing expected FROM line")
	}
	if !strings.Contains(df, "COPY ./f0 /tmp/dcx-features/github-cli/") {
		t.Error("Dockerfile missing COPY for github-cli")
	}
	if !strings.Contains(df, "COPY ./f1 /tmp/dcx-features/docker-in-docker/") {
		t.Error("Dockerfile missing COPY for docker-in-docker")
	}
	if !strings.Contains(df, "RUN cd /tmp/dcx-features/github-cli") {
		t.Error("Dockerfile missing RUN for github-cli")
	}
	if !strings.Contains(df, "FROM dcx_features AS final") {
		t.Error("Dockerfile missing final stage")
	}
}

func TestDockerfileMetadataLabel(t *testing.T) {
	fDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(fDir, "install.sh"), []byte("#!/bin/bash\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fDir, "devcontainer-feature.json"), []byte(`{"id":"github-cli"}`), 0644); err != nil {
		t.Fatal(err)
	}

	features := []ResolvedFeature{
		{
			Ref:    FeatureRef{ID: "github-cli", Version: "1"},
			Meta:   FeatureMeta{ID: "github-cli", Name: "GitHub CLI", Version: "1.0.0"},
			Digest: "sha256:abc123",
			Path:   fDir,
		},
	}

	ctxDir, dockerfilePath, err := BuildContext("base", features, "root", "vscode")
	if err != nil {
		t.Fatalf("BuildContext error: %v", err)
	}
	defer func() { _ = os.RemoveAll(ctxDir) }()

	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("reading Dockerfile: %v", err)
	}
	df := string(content)

	if !strings.Contains(df, "LABEL devcontainer.metadata='[") {
		t.Error("Dockerfile missing metadata label")
	}
	if !strings.Contains(df, `"id":"github-cli"`) {
		t.Error("metadata label missing id")
	}
}

func TestDockerfileMetadataLabelIncludesAllFeatureProperties(t *testing.T) {
	fDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(fDir, "install.sh"), []byte("#!/bin/bash\n"), 0755); err != nil {
		t.Fatal(err)
	}
	featureJSON := `{
		"id": "docker-in-docker",
		"version": "1.0.0",
		"name": "Docker in Docker",
		"privileged": true,
		"init": true,
		"capAdd": ["SYS_ADMIN"],
		"securityOpt": ["seccomp=unconfined"],
		"entrypoint": "/usr/local/share/docker-init.sh",
		"mounts": [{"type": "volume", "source": "dind", "target": "/var/lib/docker"}],
		"containerEnv": {"DOCKER_BUILDKIT": "1"},
		"onCreateCommand": "true",
		"postCreateCommand": "echo postcreate",
		"postStartCommand": "echo poststart"
	}`
	if err := os.WriteFile(filepath.Join(fDir, "devcontainer-feature.json"), []byte(featureJSON), 0644); err != nil {
		t.Fatal(err)
	}

	features := []ResolvedFeature{
		{
			Ref: FeatureRef{ID: "docker-in-docker", Version: "1"},
			Meta: FeatureMeta{
				ID: "docker-in-docker", Name: "Docker in Docker", Version: "1.0.0",
				Privileged: true, Init: true,
				CapAdd: []string{"SYS_ADMIN"}, SecurityOpt: []string{"seccomp=unconfined"},
				Entrypoint:        "/usr/local/share/docker-init.sh",
				Mounts:            []byte(`[{"type":"volume","source":"dind","target":"/var/lib/docker"}]`),
				ContainerEnv:      map[string]string{"DOCKER_BUILDKIT": "1"},
				OnCreateCommand:   spec.NewLifecycleCommandString("true"),
				PostCreateCommand: spec.NewLifecycleCommandString("echo postcreate"),
				PostStartCommand:  spec.NewLifecycleCommandString("echo poststart"),
			},
			Path: fDir,
		},
	}

	ctxDir, dockerfilePath, err := BuildContext("base", features, "root", "vscode")
	if err != nil {
		t.Fatalf("BuildContext error: %v", err)
	}
	defer func() { _ = os.RemoveAll(ctxDir) }()

	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("reading Dockerfile: %v", err)
	}
	df := string(content)

	required := []string{
		`"privileged":true`,
		`"init":true`,
		`"capAdd"`,
		`"securityOpt"`,
		`"entrypoint":"/usr/local/share/docker-init.sh"`,
		`"mounts"`,
		`"containerEnv"`,
		`"onCreateCommand"`,
		`"postCreateCommand"`,
		`"postStartCommand"`,
	}
	for _, s := range required {
		if !strings.Contains(df, s) {
			t.Errorf("Dockerfile metadata label missing %q", s)
		}
	}
	if !strings.Contains(df, "ENV DOCKER_BUILDKIT=1") {
		t.Error("Dockerfile missing ENV instruction for feature containerEnv")
	}
}

func TestWrapperScript(t *testing.T) {
	dir := t.TempDir()
	if err := writeInstallWrapper(dir); err != nil {
		t.Fatalf("writeInstallWrapper error: %v", err)
	}

	wrapper := filepath.Join(dir, "devcontainer-features-install.sh")
	content, err := os.ReadFile(wrapper)
	if err != nil {
		t.Fatalf("reading wrapper: %v", err)
	}

	if !strings.Contains(string(content), "source devcontainer-features.env") {
		t.Error("wrapper missing source command")
	}
	if !strings.Contains(string(content), "exec ./install.sh") {
		t.Error("wrapper missing exec install.sh")
	}
	if !strings.Contains(string(content), "set -a") {
		t.Error("wrapper missing set -a (export all vars)")
	}
	if !strings.Contains(string(content), "set +a") {
		t.Error("wrapper missing set +a")
	}

	info, err := os.Stat(wrapper)
	if err != nil {
		t.Fatalf("stat wrapper: %v", err)
	}
	if info.Mode().Perm()&0111 == 0 {
		t.Error("wrapper should be executable")
	}
}

func TestCopyDir(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "b.txt"), []byte("world"), 0644); err != nil {
		t.Fatal(err)
	}

	dst := t.TempDir()
	if err := copyDir(src, dst); err != nil {
		t.Fatalf("copyDir error: %v", err)
	}

	gotA, err := os.ReadFile(filepath.Join(dst, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotA) != "hello" {
		t.Errorf("a.txt = %q, want hello", string(gotA))
	}
}
