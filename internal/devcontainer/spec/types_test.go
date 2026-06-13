package spec

import (
	"encoding/json"
	"testing"
)

func TestUnmarshalBasicFields(t *testing.T) {
	data := []byte(`{
		"name": "my-devcontainer",
		"image": "ubuntu:22.04",
		"workspaceFolder": "/workspace",
		"workspaceMount": "source=.,target=/workspace,type=bind",
		"remoteUser": "vscode",
		"containerUser": "root",
		"shutdownAction": "stopContainer",
		"service": "app",
		"runServices": ["app", "db"]
	}`)

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if cfg.Name != "my-devcontainer" {
		t.Errorf("Name = %q, want my-devcontainer", cfg.Name)
	}
	if cfg.Image != "ubuntu:22.04" {
		t.Errorf("Image = %q, want ubuntu:22.04", cfg.Image)
	}
	if cfg.WorkspaceFolder != "/workspace" {
		t.Errorf("WorkspaceFolder = %q, want /workspace", cfg.WorkspaceFolder)
	}
	if cfg.WorkspaceMount != "source=.,target=/workspace,type=bind" {
		t.Errorf("WorkspaceMount = %q, want bind mount string", cfg.WorkspaceMount)
	}
	if cfg.RemoteUser != "vscode" {
		t.Errorf("RemoteUser = %q, want vscode", cfg.RemoteUser)
	}
	if cfg.ContainerUser != "root" {
		t.Errorf("ContainerUser = %q, want root", cfg.ContainerUser)
	}
	if cfg.ShutdownAction != "stopContainer" {
		t.Errorf("ShutdownAction = %q, want stopContainer", cfg.ShutdownAction)
	}
	if cfg.Service != "app" {
		t.Errorf("Service = %q, want app", cfg.Service)
	}
	if len(cfg.RunServices) != 2 || cfg.RunServices[0] != "app" || cfg.RunServices[1] != "db" {
		t.Errorf("RunServices = %v, want [app db]", cfg.RunServices)
	}
}

func TestUnmarshalBuildAsString(t *testing.T) {
	data := []byte(`{"build": "Dockerfile.dev"}`)
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if cfg.Build == nil {
		t.Fatal("expected Build to be non-nil")
	}
	if cfg.Build.Dockerfile != "Dockerfile.dev" {
		t.Errorf("Build.Dockerfile = %q, want Dockerfile.dev", cfg.Build.Dockerfile)
	}
}

func TestUnmarshalBuildAsObject(t *testing.T) {
	data := []byte(`{
		"build": {
			"dockerfile": "Dockerfile",
			"context": ".",
			"args": {"NODE_VERSION": "18"},
			"target": "development"
		}
	}`)
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if cfg.Build == nil {
		t.Fatal("expected Build to be non-nil")
	}
	if cfg.Build.Dockerfile != "Dockerfile" {
		t.Errorf("Build.Dockerfile = %q, want Dockerfile", cfg.Build.Dockerfile)
	}
	if cfg.Build.Context != "." {
		t.Errorf("Build.Context = %q, want .", cfg.Build.Context)
	}
	if cfg.Build.Args["NODE_VERSION"] != "18" {
		t.Errorf("Build.Args[NODE_VERSION] = %q, want 18", cfg.Build.Args["NODE_VERSION"])
	}
	if cfg.Build.Target != "development" {
		t.Errorf("Build.Target = %q, want development", cfg.Build.Target)
	}
}

func TestUnmarshalLegacyDockerfile(t *testing.T) {
	data := []byte(`{"dockerfile": "Dockerfile"}`)
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if cfg.LegacyDockerfile != "Dockerfile" {
		t.Errorf("LegacyDockerfile = %q, want Dockerfile", cfg.LegacyDockerfile)
	}
}

func TestUnmarshalDockerComposeFileAsString(t *testing.T) {
	data := []byte(`{"dockerComposeFile": "docker-compose.yml"}`)
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(cfg.DockerComposeFile) != 1 || cfg.DockerComposeFile[0] != "docker-compose.yml" {
		t.Errorf("DockerComposeFile = %v, want [docker-compose.yml]", cfg.DockerComposeFile)
	}
}

func TestUnmarshalDockerComposeFileAsArray(t *testing.T) {
	data := []byte(`{"dockerComposeFile": ["docker-compose.yml", "docker-compose.override.yml"]}`)
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(cfg.DockerComposeFile) != 2 {
		t.Fatalf("expected 2 compose files, got %d", len(cfg.DockerComposeFile))
	}
	if cfg.DockerComposeFile[0] != "docker-compose.yml" {
		t.Errorf("DockerComposeFile[0] = %q, want docker-compose.yml", cfg.DockerComposeFile[0])
	}
	if cfg.DockerComposeFile[1] != "docker-compose.override.yml" {
		t.Errorf("DockerComposeFile[1] = %q, want docker-compose.override.yml", cfg.DockerComposeFile[1])
	}
}

func TestUnmarshalLifecycleCommands(t *testing.T) {
	cases := []struct {
		name string
		json string
		want struct {
			postCreate, postStart, postAttach, init string
		}
	}{
		{
			name: "all strings",
			json: `{"postCreateCommand": "echo hello", "postStartCommand": "echo start", "postAttachCommand": "echo attach", "initializeCommand": "echo init"}`,
			want: struct{ postCreate, postStart, postAttach, init string }{
				"echo hello", "echo start", "echo attach", "echo init",
			},
		},
		{
			name: "arrays treated as absent",
			json: `{"postCreateCommand": ["echo", "hello"]}`,
			want: struct{ postCreate, postStart, postAttach, init string }{},
		},
		{
			name: "objects treated as absent",
			json: `{"postCreateCommand": {"bash": "echo hello", "zsh": "echo hi"}}`,
			want: struct{ postCreate, postStart, postAttach, init string }{},
		},
		{
			name: "absent fields",
			json: `{}`,
			want: struct{ postCreate, postStart, postAttach, init string }{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var cfg Config
			if err := json.Unmarshal([]byte(tc.json), &cfg); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}
			if cfg.PostCreateCommand != tc.want.postCreate {
				t.Errorf("PostCreateCommand = %q, want %q", cfg.PostCreateCommand, tc.want.postCreate)
			}
			if cfg.PostStartCommand != tc.want.postStart {
				t.Errorf("PostStartCommand = %q, want %q", cfg.PostStartCommand, tc.want.postStart)
			}
			if cfg.PostAttachCommand != tc.want.postAttach {
				t.Errorf("PostAttachCommand = %q, want %q", cfg.PostAttachCommand, tc.want.postAttach)
			}
			if cfg.InitializeCommand != tc.want.init {
				t.Errorf("InitializeCommand = %q, want %q", cfg.InitializeCommand, tc.want.init)
			}
		})
	}
}

func TestUnmarshalFeatures(t *testing.T) {
	data := []byte(`{"features": {"ghcr.io/devcontainers/features/go:1": {"version": "1.21"}}}`)
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(cfg.Features) != 1 {
		t.Fatalf("expected 1 feature, got %d", len(cfg.Features))
	}
	raw, ok := cfg.Features["ghcr.io/devcontainers/features/go:1"]
	if !ok {
		t.Fatal("expected feature key missing")
	}
	var opts map[string]interface{}
	if err := json.Unmarshal(raw, &opts); err != nil {
		t.Fatalf("unmarshal feature options: %v", err)
	}
	if opts["version"] != "1.21" {
		t.Errorf("feature version = %v, want 1.21", opts["version"])
	}
}

func TestUnmarshalPortsAttributes(t *testing.T) {
	data := []byte(`{"portsAttributes": {"8080": {"label": "app", "onAutoForward": "notify"}}}`)
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	raw, ok := cfg.PortsAttributes["8080"]
	if !ok {
		t.Fatal("expected port attribute missing")
	}
	var attrs map[string]interface{}
	if err := json.Unmarshal(raw, &attrs); err != nil {
		t.Fatalf("unmarshal port attributes: %v", err)
	}
	if attrs["label"] != "app" {
		t.Errorf("label = %v, want app", attrs["label"])
	}
	if attrs["onAutoForward"] != "notify" {
		t.Errorf("onAutoForward = %v, want notify", attrs["onAutoForward"])
	}
}

func TestUnmarshalPointerFields(t *testing.T) {
	trueVal := true
	falseVal := false
	cases := []struct {
		name   string
		json   string
		want   *bool
		field  string
		getter func(Config) *bool
	}{
		{
			name:   "overrideCommand true",
			json:   `{"overrideCommand": true}`,
			want:   &trueVal,
			field:  "OverrideCommand",
			getter: func(c Config) *bool { return c.OverrideCommand },
		},
		{
			name:   "overrideCommand false",
			json:   `{"overrideCommand": false}`,
			want:   &falseVal,
			field:  "OverrideCommand",
			getter: func(c Config) *bool { return c.OverrideCommand },
		},
		{
			name:   "updateRemoteUserUID true",
			json:   `{"updateRemoteUserUID": true}`,
			want:   &trueVal,
			field:  "UpdateRemoteUserUID",
			getter: func(c Config) *bool { return c.UpdateRemoteUserUID },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var cfg Config
			if err := json.Unmarshal([]byte(tc.json), &cfg); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}
			got := tc.getter(cfg)
			if got == nil || tc.want == nil {
				if got != tc.want {
					t.Errorf("%s = %v, want %v", tc.field, got, tc.want)
				}
				return
			}
			if *got != *tc.want {
				t.Errorf("%s = %v, want %v", tc.field, *got, *tc.want)
			}
		})
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	original := Config{
		Name:              "test",
		Image:             "ubuntu:latest",
		WorkspaceFolder:   "/workspace",
		RemoteUser:        "vscode",
		ContainerEnv:      map[string]string{"FOO": "bar"},
		Mounts:            []string{"type=bind,source=/tmp,target=/tmp"},
		PostCreateCommand: "echo hello",
		RunArgs:           []string{"--cap-add=SYS_PTRACE"},
		ForwardPorts:      []int{8080, 3000},
	}

	data, err := json.Marshal(&original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var parsed Config
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if parsed.Name != original.Name {
		t.Errorf("Name = %q, want %q", parsed.Name, original.Name)
	}
	if parsed.Image != original.Image {
		t.Errorf("Image = %q, want %q", parsed.Image, original.Image)
	}
	if parsed.WorkspaceFolder != original.WorkspaceFolder {
		t.Errorf("WorkspaceFolder = %q, want %q", parsed.WorkspaceFolder, original.WorkspaceFolder)
	}
	if parsed.RemoteUser != original.RemoteUser {
		t.Errorf("RemoteUser = %q, want %q", parsed.RemoteUser, original.RemoteUser)
	}
	if parsed.ContainerEnv["FOO"] != original.ContainerEnv["FOO"] {
		t.Errorf("ContainerEnv[FOO] = %q, want %q", parsed.ContainerEnv["FOO"], original.ContainerEnv["FOO"])
	}
	if len(parsed.Mounts) != 1 || parsed.Mounts[0] != original.Mounts[0] {
		t.Errorf("Mounts = %v, want %v", parsed.Mounts, original.Mounts)
	}
	if parsed.PostCreateCommand != original.PostCreateCommand {
		t.Errorf("PostCreateCommand = %q, want %q", parsed.PostCreateCommand, original.PostCreateCommand)
	}
	if len(parsed.RunArgs) != 1 || parsed.RunArgs[0] != original.RunArgs[0] {
		t.Errorf("RunArgs = %v, want %v", parsed.RunArgs, original.RunArgs)
	}
	if len(parsed.ForwardPorts) != 2 || parsed.ForwardPorts[0] != 8080 || parsed.ForwardPorts[1] != 3000 {
		t.Errorf("ForwardPorts = %v, want %v", parsed.ForwardPorts, original.ForwardPorts)
	}
}

func TestMarshalBuildIsAlwaysObject(t *testing.T) {
	cfg := Config{
		Image: "ubuntu:latest",
		Build: &Build{Dockerfile: "Dockerfile"},
	}
	data, err := json.Marshal(&cfg)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if string(data) == "" {
		t.Fatal("marshaled data is empty")
	}
	// The Build must be marshaled as an object to conform to the
	// devcontainer spec schema even when only Dockerfile is set.
	var rawMap map[string]interface{}
	if err := json.Unmarshal(data, &rawMap); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	if _, ok := rawMap["build"].(map[string]interface{}); !ok {
		t.Errorf("build should be an object, got %T", rawMap["build"])
	}
}

func TestMarshalBuildAsObject(t *testing.T) {
	cfg := Config{
		Image: "ubuntu:latest",
		Build: &Build{Dockerfile: "Dockerfile", Context: "."},
	}
	data, err := json.Marshal(&cfg)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var rawMap map[string]interface{}
	if err := json.Unmarshal(data, &rawMap); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	if _, ok := rawMap["build"].(map[string]interface{}); !ok {
		t.Errorf("build should be an object, got %T", rawMap["build"])
	}
}

func TestMarshalDockerComposeFileAsString(t *testing.T) {
	cfg := Config{
		Image:             "ubuntu:latest",
		DockerComposeFile: []string{"docker-compose.yml"},
	}
	data, err := json.Marshal(&cfg)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var rawMap map[string]interface{}
	if err := json.Unmarshal(data, &rawMap); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	if _, ok := rawMap["dockerComposeFile"].(string); !ok {
		t.Errorf("dockerComposeFile should be a string, got %T", rawMap["dockerComposeFile"])
	}
}

func TestMarshalDockerComposeFileAsArray(t *testing.T) {
	cfg := Config{
		Image:             "ubuntu:latest",
		DockerComposeFile: []string{"docker-compose.yml", "docker-compose.override.yml"},
	}
	data, err := json.Marshal(&cfg)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var rawMap map[string]interface{}
	if err := json.Unmarshal(data, &rawMap); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	if _, ok := rawMap["dockerComposeFile"].([]interface{}); !ok {
		t.Errorf("dockerComposeFile should be an array, got %T", rawMap["dockerComposeFile"])
	}
}

func TestEffectiveDockerfile(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{"build object", Config{Build: &Build{Dockerfile: "Dockerfile"}}, "Dockerfile"},
		{"legacy dockerfile", Config{LegacyDockerfile: "Dockerfile.legacy"}, "Dockerfile.legacy"},
		{"both prefer build", Config{Build: &Build{Dockerfile: "Dockerfile"}, LegacyDockerfile: "Dockerfile.legacy"}, "Dockerfile"},
		{"neither", Config{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.cfg.EffectiveDockerfile()
			if got != tc.want {
				t.Errorf("EffectiveDockerfile() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEffectiveDockerComposeFiles(t *testing.T) {
	cfg := Config{DockerComposeFile: []string{"a.yml", "b.yml"}}
	got := cfg.EffectiveDockerComposeFiles()
	if len(got) != 2 || got[0] != "a.yml" || got[1] != "b.yml" {
		t.Errorf("EffectiveDockerComposeFiles() = %v, want [a.yml b.yml]", got)
	}
	// Ensure it returns a copy.
	got[0] = "modified"
	if cfg.DockerComposeFile[0] == "modified" {
		t.Error("EffectiveDockerComposeFiles() returned a reference, not a copy")
	}
}

func TestHasFeatures(t *testing.T) {
	if (&Config{}).HasFeatures() {
		t.Error("HasFeatures() should be false for empty config")
	}
	if !(&Config{Features: map[string]json.RawMessage{"foo": {}}}).HasFeatures() {
		t.Error("HasFeatures() should be true when features are set")
	}
}

func TestIsComposeProject(t *testing.T) {
	if (&Config{Service: "app"}).IsComposeProject() {
		t.Error("IsComposeProject() should be false when only service is set without dockerComposeFile")
	}
	if !(&Config{DockerComposeFile: []string{"docker-compose.yml"}}).IsComposeProject() {
		t.Error("IsComposeProject() should be true when dockerComposeFile is set")
	}
}
