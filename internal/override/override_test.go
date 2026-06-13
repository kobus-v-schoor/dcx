package override

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateWritesOverride(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}

	original := `{"image": "test:latest"}`
	srcPath := filepath.Join(devcontainerDir, "devcontainer.json")
	if err := os.WriteFile(srcPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	od, err := Create(workspace, "")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	if od.Config == nil {
		t.Fatal("expected Config to be non-nil")
	}
	if od.Config.Image != "test:latest" {
		t.Errorf("Config.Image = %q, want test:latest", od.Config.Image)
	}
}

func TestCreateMissingDevcontainerJSONNoDefaultImage(t *testing.T) {
	workspace := t.TempDir()

	_, err := Create(workspace, "")
	if err == nil {
		t.Fatal("expected error when devcontainer.json is missing and no default_image is set")
	}
	if !strings.Contains(err.Error(), "default_image is not configured") {
		t.Errorf("error message should mention default_image, got: %v", err)
	}
}

func TestCreateGeneratesSpecWithDefaultImage(t *testing.T) {
	workspace := t.TempDir()

	od, err := Create(workspace, "mcr.microsoft.com/devcontainers/base:debian")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	if od.Config == nil {
		t.Fatal("expected Config to be non-nil")
	}
	if od.Config.Image != "mcr.microsoft.com/devcontainers/base:debian" {
		t.Errorf("Config.Image = %q, want mcr.microsoft.com/devcontainers/base:debian", od.Config.Image)
	}

	if od.ContainerWorkspaceFolder != workspace {
		t.Errorf("ContainerWorkspaceFolder = %q, want %q (host path when absent)", od.ContainerWorkspaceFolder, workspace)
	}

	if err := od.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(od.Dir, "devcontainer.json"))
	if err != nil {
		t.Fatalf("reading generated file: %v", err)
	}

	var saved map[string]interface{}
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("unmarshalling saved file: %v", err)
	}
	if saved["image"] != "mcr.microsoft.com/devcontainers/base:debian" {
		t.Errorf("saved image = %v, want mcr.microsoft.com/devcontainers/base:debian", saved["image"])
	}
}

func TestCreateExtractsWorkspaceFolder(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := `{"image": "test:latest", "workspaceFolder": "/workspace"}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	od, err := Create(workspace, "")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	if od.ContainerWorkspaceFolder != "/workspace" {
		t.Errorf("ContainerWorkspaceFolder = %q, want %q", od.ContainerWorkspaceFolder, "/workspace")
	}
}

func TestCreateExtractsRemoteUserHomeDir(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := `{"image": "test:latest", "remoteUser": "vscode"}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	od, err := Create(workspace, "")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	if od.ContainerHomeDir != "/home/vscode" {
		t.Errorf("ContainerHomeDir = %q, want %q", od.ContainerHomeDir, "/home/vscode")
	}
}

func TestCreateExtractsRootUserHomeDir(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := `{"image": "test:latest", "remoteUser": "root"}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	od, err := Create(workspace, "")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	if od.ContainerHomeDir != "/root" {
		t.Errorf("ContainerHomeDir = %q, want %q", od.ContainerHomeDir, "/root")
	}
}

func TestCreateDefaultsHomeDirWhenRemoteUserMissing(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := `{"image": "test:latest"}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	od, err := Create(workspace, "")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	if od.ContainerHomeDir != "" {
		t.Errorf("ContainerHomeDir = %q, want empty", od.ContainerHomeDir)
	}
}

func TestCreateDefaultsHomeDirForMicrosoftDevcontainerImage(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := `{"image": "mcr.microsoft.com/devcontainers/base:debian"}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	od, err := Create(workspace, "")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	if od.ContainerHomeDir != "/home/vscode" {
		t.Errorf("ContainerHomeDir = %q, want %q", od.ContainerHomeDir, "/home/vscode")
	}
}

func TestCreateDefaultsHomeDirForMicrosoftDevcontainerImageWithTag(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := `{"image": "mcr.microsoft.com/devcontainers/go:1"}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	od, err := Create(workspace, "")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	if od.ContainerHomeDir != "/home/vscode" {
		t.Errorf("ContainerHomeDir = %q, want %q", od.ContainerHomeDir, "/home/vscode")
	}
}

func TestCreateDoesNotOverrideExplicitRemoteUserForMicrosoftImage(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := `{"image": "mcr.microsoft.com/devcontainers/base:debian", "remoteUser": "root"}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	od, err := Create(workspace, "")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	if od.ContainerHomeDir != "/root" {
		t.Errorf("ContainerHomeDir = %q, want %q", od.ContainerHomeDir, "/root")
	}
}

func TestCreateDefaultsHomeDirForDefaultImageWhenMicrosoft(t *testing.T) {
	workspace := t.TempDir()

	od, err := Create(workspace, "mcr.microsoft.com/devcontainers/base:debian")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	if od.ContainerHomeDir != "/home/vscode" {
		t.Errorf("ContainerHomeDir = %q, want %q", od.ContainerHomeDir, "/home/vscode")
	}
}

func TestCreateInjectsRemoteUserForMicrosoftImage(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := `{"image": "mcr.microsoft.com/devcontainers/base:debian"}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	od, err := Create(workspace, "")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	if err := od.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(od.Dir, "devcontainer.json"))
	if err != nil {
		t.Fatalf("reading override file: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("unmarshalling override config: %v", err)
	}
	if config["remoteUser"] != "vscode" {
		t.Errorf("remoteUser = %v, want vscode", config["remoteUser"])
	}
}

func TestCreateDefaultsWorkspaceFolderToHostPath(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := `{"image": "test:latest"}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	od, err := Create(workspace, "")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	if od.ContainerWorkspaceFolder != workspace {
		t.Errorf("ContainerWorkspaceFolder = %q, want %q", od.ContainerWorkspaceFolder, workspace)
	}
}

func TestCreateCloseRemovesDir(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	od, err := Create(workspace, "")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if _, err := os.Stat(od.Dir); os.IsNotExist(err) {
		t.Fatal("override dir should exist before Close")
	}

	od.Close()

	if _, err := os.Stat(od.Dir); !os.IsNotExist(err) {
		t.Fatal("override dir should be removed after Close")
	}
}

func TestSavePersistsToDisk(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := `{"image": "test:latest"}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	od, err := Create(workspace, "")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	if err := od.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	overrideData, err := os.ReadFile(filepath.Join(od.Dir, "devcontainer.json"))
	if err != nil {
		t.Fatalf("reading override file: %v", err)
	}

	var saved map[string]interface{}
	if err := json.Unmarshal(overrideData, &saved); err != nil {
		t.Fatalf("unmarshalling saved file: %v", err)
	}
	if saved["image"] != "test:latest" {
		t.Errorf("saved image = %v, want test:latest", saved["image"])
	}
}

func TestInjectContainerEnvAddsNewContainerEnv(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(`{"image": "test:latest"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	od, err := Create(workspace, "")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	od.InjectContainerEnv(map[string]string{
		"AWS_ACCESS_KEY_ID": "AKIAIOSFODNN7EXAMPLE",
		"MY_VAR":            "hello",
	})

	if len(od.Config.ContainerEnv) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(od.Config.ContainerEnv))
	}
	if od.Config.ContainerEnv["AWS_ACCESS_KEY_ID"] != "AKIAIOSFODNN7EXAMPLE" {
		t.Errorf("ContainerEnv[AWS_ACCESS_KEY_ID] = %q", od.Config.ContainerEnv["AWS_ACCESS_KEY_ID"])
	}
	if od.Config.ContainerEnv["MY_VAR"] != "hello" {
		t.Errorf("ContainerEnv[MY_VAR] = %q", od.Config.ContainerEnv["MY_VAR"])
	}

	if err := od.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(od.Dir, "devcontainer.json"))
	if err != nil {
		t.Fatalf("reading override file: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("unmarshalling override config: %v", err)
	}

	containerEnv, ok := config["containerEnv"].(map[string]interface{})
	if !ok {
		t.Fatal("containerEnv key missing from override config")
	}
	if containerEnv["AWS_ACCESS_KEY_ID"] != "AKIAIOSFODNN7EXAMPLE" {
		t.Errorf("containerEnv[AWS_ACCESS_KEY_ID] = %v, want AKIAIOSFODNN7EXAMPLE", containerEnv["AWS_ACCESS_KEY_ID"])
	}
	if containerEnv["MY_VAR"] != "hello" {
		t.Errorf("containerEnv[MY_VAR] = %v, want hello", containerEnv["MY_VAR"])
	}
	if config["image"] != "test:latest" {
		t.Errorf("image = %v, want test:latest", config["image"])
	}
}

func TestInjectContainerEnvMergesWithExisting(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := `{"image": "test:latest", "containerEnv": {"EXISTING_VAR": "existing_value"}}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	od, err := Create(workspace, "")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	od.InjectContainerEnv(map[string]string{"NEW_VAR": "new_value"})

	if od.Config.ContainerEnv["EXISTING_VAR"] != "existing_value" {
		t.Errorf("ContainerEnv[EXISTING_VAR] = %q, want existing_value", od.Config.ContainerEnv["EXISTING_VAR"])
	}
	if od.Config.ContainerEnv["NEW_VAR"] != "new_value" {
		t.Errorf("ContainerEnv[NEW_VAR] = %q, want new_value", od.Config.ContainerEnv["NEW_VAR"])
	}

	if err := od.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(od.Dir, "devcontainer.json"))
	if err != nil {
		t.Fatalf("reading override file: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("unmarshalling override config: %v", err)
	}

	containerEnv, ok := config["containerEnv"].(map[string]interface{})
	if !ok {
		t.Fatal("containerEnv key missing from override config")
	}
	if containerEnv["EXISTING_VAR"] != "existing_value" {
		t.Errorf("containerEnv[EXISTING_VAR] = %v, want existing_value", containerEnv["EXISTING_VAR"])
	}
	if containerEnv["NEW_VAR"] != "new_value" {
		t.Errorf("containerEnv[NEW_VAR] = %v, want new_value", containerEnv["NEW_VAR"])
	}
}

func TestInjectContainerEnvOverridesDuplicateKey(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := `{"image": "test:latest", "containerEnv": {"MY_VAR": "old_value"}}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	od, err := Create(workspace, "")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	od.InjectContainerEnv(map[string]string{"MY_VAR": "new_value"})

	if od.Config.ContainerEnv["MY_VAR"] != "new_value" {
		t.Errorf("ContainerEnv[MY_VAR] = %q, want new_value", od.Config.ContainerEnv["MY_VAR"])
	}

	if err := od.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(od.Dir, "devcontainer.json"))
	if err != nil {
		t.Fatalf("reading override file: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("unmarshalling override config: %v", err)
	}

	containerEnv, ok := config["containerEnv"].(map[string]interface{})
	if !ok {
		t.Fatal("containerEnv key missing from override config")
	}
	if containerEnv["MY_VAR"] != "new_value" {
		t.Errorf("containerEnv[MY_VAR] = %v, want new_value", containerEnv["MY_VAR"])
	}
}

func TestInjectContainerEnvEmptyMapNoop(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := `{"image": "test:latest"}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	od, err := Create(workspace, "")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	// Empty map should be a no-op — config should not have containerEnv.
	od.InjectContainerEnv(map[string]string{})

	if od.Config.ContainerEnv != nil {
		t.Errorf("expected nil ContainerEnv, got %v", od.Config.ContainerEnv)
	}
}

func TestInjectMountsAddsNewMounts(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(`{"image": "test:latest"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	od, err := Create(workspace, "")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	od.InjectMounts([]string{
		"type=bind,source=/host/data,target=/container/data,readonly",
		"type=bind,source=/host/config,target=/container/config",
	})

	if len(od.Config.Mounts) != 2 {
		t.Fatalf("expected 2 mounts, got %d", len(od.Config.Mounts))
	}
	if od.Config.Mounts[0] != "type=bind,source=/host/data,target=/container/data,readonly" {
		t.Errorf("Mounts[0] = %v, want readonly mount", od.Config.Mounts[0])
	}
	if od.Config.Mounts[1] != "type=bind,source=/host/config,target=/container/config" {
		t.Errorf("Mounts[1] = %v, want non-readonly mount", od.Config.Mounts[1])
	}

	if err := od.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(od.Dir, "devcontainer.json"))
	if err != nil {
		t.Fatalf("reading override file: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("unmarshalling override config: %v", err)
	}

	mounts, ok := config["mounts"].([]interface{})
	if !ok {
		t.Fatal("mounts key missing from override config")
	}
	if len(mounts) != 2 {
		t.Fatalf("expected 2 mounts, got %d", len(mounts))
	}
	if mounts[0] != "type=bind,source=/host/data,target=/container/data,readonly" {
		t.Errorf("mounts[0] = %v, want readonly mount", mounts[0])
	}
	if mounts[1] != "type=bind,source=/host/config,target=/container/config" {
		t.Errorf("mounts[1] = %v, want non-readonly mount", mounts[1])
	}
}

func TestInjectMountsAppendsToExisting(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := `{"image": "test:latest", "mounts": ["type=volume,source=myvol,target=/data"]}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	od, err := Create(workspace, "")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	od.InjectMounts([]string{"type=bind,source=/host/path,target=/container/path,readonly"})

	if len(od.Config.Mounts) != 2 {
		t.Fatalf("expected 2 mounts (1 existing + 1 injected), got %d", len(od.Config.Mounts))
	}
	if od.Config.Mounts[0] != "type=volume,source=myvol,target=/data" {
		t.Errorf("Mounts[0] = %v, want existing mount", od.Config.Mounts[0])
	}
	if od.Config.Mounts[1] != "type=bind,source=/host/path,target=/container/path,readonly" {
		t.Errorf("Mounts[1] = %v, want injected mount", od.Config.Mounts[1])
	}

	if err := od.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(od.Dir, "devcontainer.json"))
	if err != nil {
		t.Fatalf("reading override file: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("unmarshalling override config: %v", err)
	}

	mounts, ok := config["mounts"].([]interface{})
	if !ok {
		t.Fatal("mounts key missing from override config")
	}
	if len(mounts) != 2 {
		t.Fatalf("expected 2 mounts (1 existing + 1 injected), got %d", len(mounts))
	}
	if mounts[0] != "type=volume,source=myvol,target=/data" {
		t.Errorf("mounts[0] = %v, want existing mount", mounts[0])
	}
	if mounts[1] != "type=bind,source=/host/path,target=/container/path,readonly" {
		t.Errorf("mounts[1] = %v, want injected mount", mounts[1])
	}
}

func TestInjectMountsEmptySliceNoop(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := `{"image": "test:latest"}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	od, err := Create(workspace, "")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	// Empty slice should be a no-op — config should not have mounts.
	od.InjectMounts([]string{})

	if od.Config.Mounts != nil && len(od.Config.Mounts) > 0 {
		t.Error("mounts should not be present when mount strings slice is empty")
	}
}

func TestMultipleInjectCallsBeforeSave(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(`{"image": "test:latest"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	od, err := Create(workspace, "")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	// First inject call.
	od.InjectContainerEnv(map[string]string{"VAR_A": "alpha"})
	// Second inject call — should merge with the first.
	od.InjectContainerEnv(map[string]string{"VAR_B": "beta"})

	if len(od.Config.ContainerEnv) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(od.Config.ContainerEnv))
	}
	if od.Config.ContainerEnv["VAR_A"] != "alpha" {
		t.Errorf("ContainerEnv[VAR_A] = %q, want alpha", od.Config.ContainerEnv["VAR_A"])
	}
	if od.Config.ContainerEnv["VAR_B"] != "beta" {
		t.Errorf("ContainerEnv[VAR_B] = %q, want beta", od.Config.ContainerEnv["VAR_B"])
	}

	if err := od.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(od.Dir, "devcontainer.json"))
	if err != nil {
		t.Fatalf("reading override file: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("unmarshalling override config: %v", err)
	}

	containerEnv, ok := config["containerEnv"].(map[string]interface{})
	if !ok {
		t.Fatal("containerEnv key missing from override config")
	}
	if containerEnv["VAR_A"] != "alpha" {
		t.Errorf("containerEnv[VAR_A] = %v, want alpha", containerEnv["VAR_A"])
	}
	if containerEnv["VAR_B"] != "beta" {
		t.Errorf("containerEnv[VAR_B] = %v, want beta", containerEnv["VAR_B"])
	}
}

func TestInjectPostCreateCommandAddsNew(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(`{"image": "test:latest"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	od, err := Create(workspace, "")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	od.InjectPostCreateCommand([]string{"echo hello"})

	if od.Config.PostCreateCommand != "echo hello" {
		t.Errorf("PostCreateCommand = %q, want echo hello", od.Config.PostCreateCommand)
	}

	if err := od.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(od.Dir, "devcontainer.json"))
	if err != nil {
		t.Fatalf("reading override file: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("unmarshalling override config: %v", err)
	}

	cmd, ok := config["postCreateCommand"].(string)
	if !ok {
		t.Fatalf("expected postCreateCommand to be a string, got %T", config["postCreateCommand"])
	}
	if cmd != "echo hello" {
		t.Errorf("postCreateCommand = %v, want echo hello", cmd)
	}
}

func TestInjectPostCreateCommandAppendsToString(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := `{"image": "test:latest", "postCreateCommand": "echo original"}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	od, err := Create(workspace, "")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	od.InjectPostCreateCommand([]string{"echo hello"})

	if od.Config.PostCreateCommand != "echo original && echo hello" {
		t.Errorf("PostCreateCommand = %q, want \"echo original && echo hello\"", od.Config.PostCreateCommand)
	}

	if err := od.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(od.Dir, "devcontainer.json"))
	if err != nil {
		t.Fatalf("reading override file: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("unmarshalling override config: %v", err)
	}

	cmd, ok := config["postCreateCommand"].(string)
	if !ok {
		t.Fatalf("expected postCreateCommand to be a string, got %T", config["postCreateCommand"])
	}
	if cmd != "echo original && echo hello" {
		t.Errorf("postCreateCommand = %v, want \"echo original && echo hello\"", cmd)
	}
}

func TestInjectPostCreateCommandOverwritesArray(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := `{"image": "test:latest", "postCreateCommand": ["echo", "original"]}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	od, err := Create(workspace, "")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	od.InjectPostCreateCommand([]string{"echo hello"})

	if od.Config.PostCreateCommand != "echo hello" {
		t.Errorf("PostCreateCommand = %q, want echo hello", od.Config.PostCreateCommand)
	}

	if err := od.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(od.Dir, "devcontainer.json"))
	if err != nil {
		t.Fatalf("reading override file: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("unmarshalling override config: %v", err)
	}

	cmd, ok := config["postCreateCommand"].(string)
	if !ok {
		t.Fatalf("expected postCreateCommand to be a string, got %T", config["postCreateCommand"])
	}
	if cmd != "echo hello" {
		t.Errorf("postCreateCommand = %v, want echo hello", cmd)
	}
}

func TestInjectPostCreateCommandMultiple(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(`{"image": "test:latest"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	od, err := Create(workspace, "")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	od.InjectPostCreateCommand([]string{"echo hello", "echo world"})

	if od.Config.PostCreateCommand != "echo hello && echo world" {
		t.Errorf("PostCreateCommand = %q, want \"echo hello && echo world\"", od.Config.PostCreateCommand)
	}

	if err := od.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(od.Dir, "devcontainer.json"))
	if err != nil {
		t.Fatalf("reading override file: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("unmarshalling override config: %v", err)
	}

	cmd, ok := config["postCreateCommand"].(string)
	if !ok {
		t.Fatalf("expected postCreateCommand to be a string, got %T", config["postCreateCommand"])
	}
	if cmd != "echo hello && echo world" {
		t.Errorf("postCreateCommand = %v, want \"echo hello && echo world\"", cmd)
	}
}

func TestInjectPostCreateCommandEmptyNoop(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := `{"image": "test:latest"}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	od, err := Create(workspace, "")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	od.InjectPostCreateCommand([]string{})

	if od.Config.PostCreateCommand != "" {
		t.Errorf("PostCreateCommand = %q, want empty", od.Config.PostCreateCommand)
	}
}
