package spec

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

//go:embed testdata/devcontainer.schema.json
var schemaFS embed.FS

// validateAgainstSchema marshals cfg to JSON and validates it against the
// official devcontainer spec schema bundled in testdata. It returns an error
// describing the first validation failure, or nil if the JSON is valid.
func validateAgainstSchema(cfg *Config) error {
	schemaData, err := schemaFS.ReadFile("testdata/devcontainer.schema.json")
	if err != nil {
		return fmt.Errorf("reading schema: %w", err)
	}

	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2019
	if err := compiler.AddResource("devcontainer.schema.json", strings.NewReader(string(schemaData))); err != nil {
		return fmt.Errorf("adding schema resource: %w", err)
	}

	sch, err := compiler.Compile("devcontainer.schema.json")
	if err != nil {
		return fmt.Errorf("compiling schema: %w", err)
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	var doc interface{}
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("unmarshaling doc: %w", err)
	}

	if err := sch.Validate(doc); err != nil {
		return fmt.Errorf("schema validation: %w", err)
	}
	return nil
}

func TestSchemaImageOnly(t *testing.T) {
	cfg := &Config{
		Image:           "mcr.microsoft.com/devcontainers/base:debian",
		Name:            "test",
		WorkspaceFolder: "/workspace",
	}
	if err := validateAgainstSchema(cfg); err != nil {
		t.Errorf("schema validation failed: %v", err)
	}
}

func TestSchemaDockerfile(t *testing.T) {
	cfg := &Config{
		Build: &Build{
			Dockerfile: "Dockerfile",
			Context:    ".",
			Args:       map[string]string{"NODE_VERSION": "18"},
			Target:     "development",
		},
		WorkspaceFolder: "/workspace",
		WorkspaceMount:  "source=.,target=/workspace,type=bind",
		Name:            "dockerfile-test",
	}
	if err := validateAgainstSchema(cfg); err != nil {
		t.Errorf("schema validation failed: %v", err)
	}
}

func TestSchemaDockerCompose(t *testing.T) {
	cfg := &Config{
		DockerComposeFile: []string{"docker-compose.yml"},
		Service:           "app",
		WorkspaceFolder:   "/workspace",
		RunServices:       []string{"app", "db"},
		Name:              "compose-test",
	}
	if err := validateAgainstSchema(cfg); err != nil {
		t.Errorf("schema validation failed: %v", err)
	}
}

func TestSchemaDockerComposeFileArray(t *testing.T) {
	cfg := &Config{
		DockerComposeFile: []string{"docker-compose.yml", "docker-compose.override.yml"},
		Service:           "app",
		WorkspaceFolder:   "/workspace",
		Name:              "compose-array-test",
	}
	if err := validateAgainstSchema(cfg); err != nil {
		t.Errorf("schema validation failed: %v", err)
	}
}

func TestSchemaFullConfig(t *testing.T) {
	cfg := &Config{
		Image:           "ubuntu:22.04",
		Name:            "full-test",
		WorkspaceFolder: "/workspace",
		WorkspaceMount:  "source=.,target=/workspace,type=bind",
		RemoteUser:      "vscode",
		ContainerUser:   "root",
		ContainerEnv: map[string]string{
			"FOO": "bar",
			"BAZ": "qux",
		},
		RemoteEnv: map[string]string{
			"EDITOR": "vim",
		},
		Mounts: []MountEntry{
			NewMountEntryString("type=bind,source=/tmp,target=/tmp"),
			NewMountEntryString("type=volume,source=myvol,target=/data"),
		},
		Features: map[string]json.RawMessage{
			"ghcr.io/devcontainers/features/go:1": json.RawMessage(`{"version":"1.21"}`),
		},
		PostCreateCommand: NewLifecycleCommandString("echo hello"),
		PostStartCommand:  NewLifecycleCommandString("echo start"),
		PostAttachCommand: NewLifecycleCommandString("echo attach"),
		InitializeCommand: NewLifecycleCommandString("echo init"),
		RunArgs:           []string{"--cap-add=SYS_PTRACE"},
		ShutdownAction:    "stopContainer",
		ForwardPorts: []ForwardPort{
			NewForwardPortInt(8080),
			NewForwardPortInt(3000),
		},
		PortsAttributes: map[string]json.RawMessage{
			"8080": json.RawMessage(`{"label":"app","onAutoForward":"notify"}`),
		},
	}
	if err := validateAgainstSchema(cfg); err != nil {
		t.Errorf("schema validation failed: %v", err)
	}
}

func TestSchemaBuildOnlyDockerfile(t *testing.T) {
	// Minimal build config: only Dockerfile is set.
	cfg := &Config{
		Build: &Build{
			Dockerfile: "Dockerfile",
		},
		WorkspaceFolder: "/workspace",
		Name:            "minimal-build",
	}
	if err := validateAgainstSchema(cfg); err != nil {
		t.Errorf("schema validation failed: %v", err)
	}
}

func TestSchemaLegacyDockerfile(t *testing.T) {
	// dockerFile (capital F) is the schema-recognized legacy form.
	// When both image and dockerFile are present the schema fails because
	// the root oneOf requires a config to match EITHER imageContainer OR
	// dockerfileContainer, not both. We therefore test the legacy form
	// without image.
	cfg := &Config{
		LegacyDockerfile: "Dockerfile",
		WorkspaceFolder:  "/workspace",
		Name:             "legacy-test",
	}
	if err := validateAgainstSchema(cfg); err != nil {
		t.Errorf("schema validation failed: %v", err)
	}
}

func TestSchemaPointerFields(t *testing.T) {
	trueVal := true
	falseVal := false
	cases := []struct {
		name string
		cfg  *Config
	}{
		{
			name: "overrideCommand true",
			cfg: &Config{
				Image:           "ubuntu:latest",
				OverrideCommand: &trueVal,
				WorkspaceFolder: "/workspace",
			},
		},
		{
			name: "overrideCommand false",
			cfg: &Config{
				Image:           "ubuntu:latest",
				OverrideCommand: &falseVal,
				WorkspaceFolder: "/workspace",
			},
		},
		{
			name: "updateRemoteUserUID true",
			cfg: &Config{
				Image:               "ubuntu:latest",
				UpdateRemoteUserUID: &trueVal,
				WorkspaceFolder:     "/workspace",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateAgainstSchema(tc.cfg); err != nil {
				t.Errorf("schema validation failed: %v", err)
			}
		})
	}
}

func TestSchemaRunServices(t *testing.T) {
	// runServices is only valid in compose projects.
	cfg := &Config{
		DockerComposeFile: []string{"docker-compose.yml"},
		Service:           "app",
		WorkspaceFolder:   "/workspace",
		RunServices:       []string{"app", "db"},
		Name:              "run-services",
	}
	if err := validateAgainstSchema(cfg); err != nil {
		t.Errorf("schema validation failed: %v", err)
	}
}
