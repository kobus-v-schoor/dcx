package override

import (
	"encoding/json"
	"os"
	"path/filepath"
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

	dir, cleanup, err := Create(workspace)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer cleanup()

	overrideData, err := os.ReadFile(filepath.Join(dir, "devcontainer.json"))
	if err != nil {
		t.Fatalf("reading override file: %v", err)
	}

	if string(overrideData) != original {
		t.Errorf("override content = %q, want %q", string(overrideData), original)
	}
}

func TestCreateMissingDevcontainerJSON(t *testing.T) {
	workspace := t.TempDir()

	_, _, err := Create(workspace)
	if err == nil {
		t.Fatal("expected error when devcontainer.json is missing")
	}
}

func TestCreateCleanupRemovesDir(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	dir, cleanup, err := Create(workspace)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatal("override dir should exist before cleanup")
	}

	cleanup()

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("override dir should be removed after cleanup")
	}
}

func TestCreateRandomDirName(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	dir1, cleanup1, err := Create(workspace)
	if err != nil {
		t.Fatalf("Create() first call error: %v", err)
	}
	cleanup1()

	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	dir2, cleanup2, err := Create(workspace)
	if err != nil {
		t.Fatalf("Create() second call error: %v", err)
	}
	cleanup2()

	if dir1 == dir2 {
		t.Errorf("dir names should be random across invocations: both %q", dir1)
	}
}

func TestCreateDirNamePrefix(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	dir, cleanup, err := Create(workspace)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer cleanup()

	base := filepath.Base(dir)
	if len(base) < 5 || base[:4] != "dcx-" {
		t.Errorf("dir base name should start with dcx-, got %q", base)
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

	dir, cleanup, err := Create(workspace)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer cleanup()

	envVars := map[string]string{
		"AWS_ACCESS_KEY_ID": "AKIAIOSFODNN7EXAMPLE",
		"MY_VAR":            "hello",
	}

	if err := InjectContainerEnv(dir, envVars); err != nil {
		t.Fatalf("InjectContainerEnv() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "devcontainer.json"))
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
	// Original key should still be present.
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

	dir, cleanup, err := Create(workspace)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer cleanup()

	envVars := map[string]string{
		"NEW_VAR": "new_value",
	}

	if err := InjectContainerEnv(dir, envVars); err != nil {
		t.Fatalf("InjectContainerEnv() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "devcontainer.json"))
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
	// Existing var should be preserved.
	if containerEnv["EXISTING_VAR"] != "existing_value" {
		t.Errorf("containerEnv[EXISTING_VAR] = %v, want existing_value", containerEnv["EXISTING_VAR"])
	}
	// New var should be added.
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

	dir, cleanup, err := Create(workspace)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer cleanup()

	envVars := map[string]string{
		"MY_VAR": "new_value",
	}

	if err := InjectContainerEnv(dir, envVars); err != nil {
		t.Fatalf("InjectContainerEnv() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "devcontainer.json"))
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
	// New value should win on key conflict.
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

	dir, cleanup, err := Create(workspace)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer cleanup()

	// Empty map should be a no-op — file should not be modified.
	if err := InjectContainerEnv(dir, map[string]string{}); err != nil {
		t.Fatalf("InjectContainerEnv() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "devcontainer.json"))
	if err != nil {
		t.Fatalf("reading override file: %v", err)
	}

	// File should remain unchanged (no containerEnv key added).
	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("unmarshalling override config: %v", err)
	}
	if _, ok := config["containerEnv"]; ok {
		t.Error("containerEnv should not be present when env vars map is empty")
	}
}
