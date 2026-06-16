package features

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildContextIncludesEnvFile(t *testing.T) {
	fDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(fDir, "install.sh"), []byte("#!/bin/bash\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fDir, "devcontainer-feature.json"), []byte(`{"id":"docker-in-docker","options":{"moby":{"type":"boolean","default":true}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	features := []ResolvedFeature{
		{
			Ref:  FeatureRef{ID: "docker-in-docker", Options: map[string]interface{}{"moby": false}},
			Meta: FeatureMeta{ID: "docker-in-docker", Name: "Docker in Docker"},
			Path: fDir,
		},
	}

	ctxDir, _, err := BuildContext("base", features, "root", "root")
	if err != nil {
		t.Fatalf("BuildContext error: %v", err)
	}
	defer func() { _ = os.RemoveAll(ctxDir) }()

	envPath := filepath.Join(ctxDir, "f0", "devcontainer-features.env")
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("reading env file: %v", err)
	}
	if !strings.Contains(string(content), "MOBY=false") {
		t.Errorf("env file missing MOBY=false, got:\n%s", string(content))
	}

	wrapperPath := filepath.Join(ctxDir, "f0", "devcontainer-features-install.sh")
	wrapperContent, err := os.ReadFile(wrapperPath)
	if err != nil {
		t.Fatalf("reading wrapper: %v", err)
	}
	if !strings.Contains(string(wrapperContent), "devcontainer-features.env") {
		t.Errorf("wrapper missing source of env file")
	}
}
