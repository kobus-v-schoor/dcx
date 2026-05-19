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

	od, err := Create(workspace)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	// Config should be parsed into the map.
	if od.config == nil {
		t.Fatal("expected config map to be non-nil")
	}
	if _, ok := od.config["image"]; !ok {
		t.Error("expected 'image' key in parsed config")
	}
}

func TestCreateMissingDevcontainerJSON(t *testing.T) {
	workspace := t.TempDir()

	_, err := Create(workspace)
	if err == nil {
		t.Fatal("expected error when devcontainer.json is missing")
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

	od, err := Create(workspace)
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

func TestCreateRandomDirName(t *testing.T) {
	workspace := t.TempDir()
	devcontainerDir := filepath.Join(workspace, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	od1, err := Create(workspace)
	if err != nil {
		t.Fatalf("Create() first call error: %v", err)
	}
	od1.Close()

	if err := os.MkdirAll(devcontainerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	od2, err := Create(workspace)
	if err != nil {
		t.Fatalf("Create() second call error: %v", err)
	}
	od2.Close()

	if od1.Dir == od2.Dir {
		t.Errorf("dir names should be random across invocations: both %q", od1.Dir)
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

	od, err := Create(workspace)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	base := filepath.Base(od.Dir)
	if len(base) < 5 || base[:4] != "dcx-" {
		t.Errorf("dir base name should start with dcx-, got %q", base)
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

	od, err := Create(workspace)
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

	// The saved file should be valid JSON containing the original keys.
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

	od, err := Create(workspace)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	envVars := map[string]string{
		"AWS_ACCESS_KEY_ID": "AKIAIOSFODNN7EXAMPLE",
		"MY_VAR":            "hello",
	}

	od.InjectContainerEnv(envVars)

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

	od, err := Create(workspace)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	envVars := map[string]string{
		"NEW_VAR": "new_value",
	}

	od.InjectContainerEnv(envVars)

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

	od, err := Create(workspace)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	envVars := map[string]string{
		"MY_VAR": "new_value",
	}

	od.InjectContainerEnv(envVars)

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

	od, err := Create(workspace)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	// Empty map should be a no-op — config should not have containerEnv.
	od.InjectContainerEnv(map[string]string{})

	if _, ok := od.config["containerEnv"]; ok {
		t.Error("containerEnv should not be present when env vars map is empty")
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

	od, err := Create(workspace)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer od.Close()

	// First inject call.
	od.InjectContainerEnv(map[string]string{"VAR_A": "alpha"})
	// Second inject call — should merge with the first.
	od.InjectContainerEnv(map[string]string{"VAR_B": "beta"})

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
