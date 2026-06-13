package spec

import (
	"encoding/json"
	"reflect"
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
	data := []byte(`{"dockerFile": "Dockerfile"}`)
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
			postCreateStr, postStartStr, postAttachStr, initStr string
		}
	}{
		{
			name: "all strings",
			json: `{"postCreateCommand": "echo hello", "postStartCommand": "echo start", "postAttachCommand": "echo attach", "initializeCommand": "echo init"}`,
			want: struct{ postCreateStr, postStartStr, postAttachStr, initStr string }{
				"echo hello", "echo start", "echo attach", "echo init",
			},
		},
		{
			name: "arrays preserved but AsString empty",
			json: `{"postCreateCommand": ["echo", "hello"]}`,
			want: struct{ postCreateStr, postStartStr, postAttachStr, initStr string }{},
		},
		{
			name: "objects preserved but AsString empty",
			json: `{"postCreateCommand": {"bash": "echo hello", "zsh": "echo hi"}}`,
			want: struct{ postCreateStr, postStartStr, postAttachStr, initStr string }{},
		},
		{
			name: "absent fields",
			json: `{}`,
			want: struct{ postCreateStr, postStartStr, postAttachStr, initStr string }{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var cfg Config
			if err := json.Unmarshal([]byte(tc.json), &cfg); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}
			if got, _ := cfg.PostCreateCommand.AsString(); got != tc.want.postCreateStr {
				t.Errorf("PostCreateCommand.AsString() = %q, want %q", got, tc.want.postCreateStr)
			}
			if got, _ := cfg.PostStartCommand.AsString(); got != tc.want.postStartStr {
				t.Errorf("PostStartCommand.AsString() = %q, want %q", got, tc.want.postStartStr)
			}
			if got, _ := cfg.PostAttachCommand.AsString(); got != tc.want.postAttachStr {
				t.Errorf("PostAttachCommand.AsString() = %q, want %q", got, tc.want.postAttachStr)
			}
			if got, _ := cfg.InitializeCommand.AsString(); got != tc.want.initStr {
				t.Errorf("InitializeCommand.AsString() = %q, want %q", got, tc.want.initStr)
			}
		})
	}
}

func TestUnmarshalLifecycleArray(t *testing.T) {
	data := []byte(`{"postCreateCommand": ["npm", "install"]}`)
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	arr, ok := cfg.PostCreateCommand.AsArray()
	if !ok {
		t.Fatal("expected array")
	}
	want := []string{"npm", "install"}
	if !reflect.DeepEqual(arr, want) {
		t.Errorf("PostCreateCommand.AsArray() = %v, want %v", arr, want)
	}
	if _, ok := cfg.PostCreateCommand.AsString(); ok {
		t.Error("expected AsString() to fail for array")
	}
}

func TestUnmarshalLifecycleObject(t *testing.T) {
	data := []byte(`{"postCreateCommand": {"server": "npm start", "db": ["mysql", "-u", "root"]}}`)
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	obj, ok := cfg.PostCreateCommand.AsObject()
	if !ok {
		t.Fatal("expected object")
	}
	if obj["server"] != "npm start" {
		t.Errorf("server = %v, want npm start", obj["server"])
	}
	if _, ok := cfg.PostCreateCommand.AsString(); ok {
		t.Error("expected AsString() to fail for object")
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

func TestUnmarshalForwardPorts(t *testing.T) {
	data := []byte(`{"forwardPorts": [8080, "db:5432", 3000]}`)
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(cfg.ForwardPorts) != 3 {
		t.Fatalf("expected 3 ports, got %d", len(cfg.ForwardPorts))
	}
	if i, ok := cfg.ForwardPorts[0].AsInt(); !ok || i != 8080 {
		t.Errorf("port[0] = %v, want 8080", cfg.ForwardPorts[0])
	}
	if s, ok := cfg.ForwardPorts[1].AsString(); !ok || s != "db:5432" {
		t.Errorf("port[1] = %v, want db:5432", cfg.ForwardPorts[1])
	}
	if i, ok := cfg.ForwardPorts[2].AsInt(); !ok || i != 3000 {
		t.Errorf("port[2] = %v, want 3000", cfg.ForwardPorts[2])
	}
}

func TestUnmarshalMountsObject(t *testing.T) {
	data := []byte(`{"mounts": [{"source": "myvol", "target": "/data", "type": "volume"}]}`)
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(cfg.Mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(cfg.Mounts))
	}
	if s, ok := cfg.Mounts[0].AsString(); ok {
		t.Errorf("expected object mount, got string: %q", s)
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(cfg.Mounts[0], &obj); err != nil {
		t.Fatalf("mount is not valid JSON object: %v", err)
	}
	if obj["type"] != "volume" {
		t.Errorf("type = %v, want volume", obj["type"])
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
		Mounts:            []MountEntry{NewMountEntryString("type=bind,source=/tmp,target=/tmp")},
		PostCreateCommand: NewLifecycleCommandString("echo hello"),
		RunArgs:           []string{"--cap-add=SYS_PTRACE"},
		ForwardPorts:      []ForwardPort{NewForwardPortInt(8080), NewForwardPortInt(3000)},
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
	if len(parsed.Mounts) != 1 {
		t.Errorf("Mounts length = %d, want 1", len(parsed.Mounts))
	} else if s, _ := parsed.Mounts[0].AsString(); s != "type=bind,source=/tmp,target=/tmp" {
		t.Errorf("Mounts[0] = %q, want bind mount string", s)
	}
	if s, _ := parsed.PostCreateCommand.AsString(); s != "echo hello" {
		t.Errorf("PostCreateCommand = %q, want echo hello", s)
	}
	if len(parsed.RunArgs) != 1 || parsed.RunArgs[0] != original.RunArgs[0] {
		t.Errorf("RunArgs = %v, want %v", parsed.RunArgs, original.RunArgs)
	}
	if len(parsed.ForwardPorts) != 2 {
		t.Errorf("ForwardPorts length = %d, want 2", len(parsed.ForwardPorts))
	} else {
		if i, ok := parsed.ForwardPorts[0].AsInt(); !ok || i != 8080 {
			t.Errorf("ForwardPorts[0] = %v, want 8080", parsed.ForwardPorts[0])
		}
		if i, ok := parsed.ForwardPorts[1].AsInt(); !ok || i != 3000 {
			t.Errorf("ForwardPorts[1] = %v, want 3000", parsed.ForwardPorts[1])
		}
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
