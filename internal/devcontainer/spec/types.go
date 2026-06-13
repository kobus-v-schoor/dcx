// Package spec provides a strongly-typed Go representation of the
// devcontainer.json configuration properties that dcx cares about.
// It handles polymorphic fields (build as string or object, dockerComposeFile
// as string or array) via custom JSON marshal/unmarshal methods.
package spec

import (
	"encoding/json"
	"fmt"
)

// Config represents the devcontainer.json configuration properties that dcx
// cares about. Optional fields use pointer types so that absent, zero, and
// explicit values can be distinguished after parsing. Fields with json:"-"
// tags are handled by custom MarshalJSON / UnmarshalJSON because they accept
// more than one JSON type.
type Config struct {
	Name                string                     `json:"name,omitempty"`
	Image               string                     `json:"image,omitempty"`
	Build               *Build                     `json:"-"`
	LegacyDockerfile    string                     `json:"dockerFile,omitempty"`
	DockerComposeFile   []string                   `json:"-"`
	Service             string                     `json:"service,omitempty"`
	RunServices         []string                   `json:"runServices,omitempty"`
	WorkspaceFolder     string                     `json:"workspaceFolder,omitempty"`
	WorkspaceMount      string                     `json:"workspaceMount,omitempty"`
	RemoteUser          string                     `json:"remoteUser,omitempty"`
	ContainerUser       string                     `json:"containerUser,omitempty"`
	ContainerEnv        map[string]string          `json:"containerEnv,omitempty"`
	RemoteEnv           map[string]string          `json:"remoteEnv,omitempty"`
	Mounts              []string                   `json:"mounts,omitempty"`
	Features            map[string]json.RawMessage `json:"features,omitempty"`
	PostCreateCommand   string                     `json:"postCreateCommand,omitempty"`
	PostStartCommand    string                     `json:"postStartCommand,omitempty"`
	PostAttachCommand   string                     `json:"postAttachCommand,omitempty"`
	InitializeCommand   string                     `json:"initializeCommand,omitempty"`
	RunArgs             []string                   `json:"runArgs,omitempty"`
	ShutdownAction      string                     `json:"shutdownAction,omitempty"`
	OverrideCommand     *bool                      `json:"overrideCommand,omitempty"`
	UpdateRemoteUserUID *bool                      `json:"updateRemoteUserUID,omitempty"`
	ForwardPorts        []int                      `json:"forwardPorts,omitempty"`
	PortsAttributes     map[string]json.RawMessage `json:"portsAttributes,omitempty"`
}

// Build represents the object form of the devcontainer.json build property.
// When the original JSON used the string shorthand, it is normalised to
// Build{Dockerfile: string} during unmarshal. MarshalJSON writes the string
// shorthand back when only Dockerfile is set, reducing noise in the override
// config.
type Build struct {
	Dockerfile string            `json:"dockerfile,omitempty"`
	Context    string            `json:"context,omitempty"`
	Args       map[string]string `json:"args,omitempty"`
	Target     string            `json:"target,omitempty"`
}

// UnmarshalJSON implements custom unmarshaling for Config so that polymorphic
// fields (build, dockerComposeFile, and lifecycle commands) are parsed
// correctly. Unknown fields are ignored. Called by json.Unmarshal when
// loading a devcontainer.json.
func (c *Config) UnmarshalJSON(data []byte) error {
	// Unmarshal the straightforward fields into a shadow struct. Fields that
	// can have multiple JSON shapes are captured as json.RawMessage and
	// parsed manually afterwards.
	type plain Config
	var shadow struct {
		*plain
		Build             json.RawMessage `json:"build"`
		DockerComposeFile json.RawMessage `json:"dockerComposeFile"`
		PostCreateCommand json.RawMessage `json:"postCreateCommand"`
		PostStartCommand  json.RawMessage `json:"postStartCommand"`
		PostAttachCommand json.RawMessage `json:"postAttachCommand"`
		InitializeCommand json.RawMessage `json:"initializeCommand"`
	}
	shadow.plain = (*plain)(c)

	if err := json.Unmarshal(data, &shadow); err != nil {
		return err
	}

	// build may be a string (dockerfile path shorthand) or an object.
	if len(shadow.Build) > 0 {
		var s string
		if err := json.Unmarshal(shadow.Build, &s); err == nil {
			c.Build = &Build{Dockerfile: s}
		} else {
			var b Build
			if err := json.Unmarshal(shadow.Build, &b); err != nil {
				return fmt.Errorf("parsing build: %w", err)
			}
			c.Build = &b
		}
	}

	// dockerComposeFile may be a single string or an array of strings.
	if len(shadow.DockerComposeFile) > 0 {
		var s string
		if err := json.Unmarshal(shadow.DockerComposeFile, &s); err == nil {
			c.DockerComposeFile = []string{s}
		} else {
			var arr []string
			if err := json.Unmarshal(shadow.DockerComposeFile, &arr); err != nil {
				return fmt.Errorf("parsing dockerComposeFile: %w", err)
			}
			c.DockerComposeFile = arr
		}
	}

	// Lifecycle commands are accepted as string, array, or object by the
	// devcontainer spec. dcx only handles the string form directly; array
	// and object forms are treated as absent so that InjectPostCreateCommand
	// overwrites them rather than trying to merge (matching the current
	// override package behaviour).
	c.PostCreateCommand = unmarshalLifecycleCommand(shadow.PostCreateCommand)
	c.PostStartCommand = unmarshalLifecycleCommand(shadow.PostStartCommand)
	c.PostAttachCommand = unmarshalLifecycleCommand(shadow.PostAttachCommand)
	c.InitializeCommand = unmarshalLifecycleCommand(shadow.InitializeCommand)

	return nil
}

// MarshalJSON implements custom marshaling for Config so that polymorphic
// fields are written back in the shape expected by the devcontainer CLI.
// Build is written as a string when only Dockerfile is set, and as an object
// otherwise. dockerComposeFile is written as a string for a single file, or
// an array for multiple files.
func (c *Config) MarshalJSON() ([]byte, error) {
	type plain Config
	shadow := struct {
		*plain
		Build             any `json:"build,omitempty"`
		DockerComposeFile any `json:"dockerComposeFile,omitempty"`
	}{
		plain: (*plain)(c),
	}

	if c.Build != nil && (c.Build.Dockerfile != "" || c.Build.Context != "" || len(c.Build.Args) > 0 || c.Build.Target != "") {
		// The devcontainer spec schema requires build to be an object;
		// writing it as a string is not schema-valid even though the
		// devcontainer CLI accepts the shorthand form.
		shadow.Build = c.Build
	}

	if len(c.DockerComposeFile) == 1 {
		shadow.DockerComposeFile = c.DockerComposeFile[0]
	} else if len(c.DockerComposeFile) > 1 {
		shadow.DockerComposeFile = c.DockerComposeFile
	}

	return json.Marshal(shadow)
}

// unmarshalLifecycleCommand extracts the string form of a lifecycle
// command (postCreateCommand, postStartCommand, etc.) from raw JSON.
// If the value is an array or object, an empty string is returned so that
// downstream dcx logic treats it as absent and overwrites rather than
// attempting to merge.
func unmarshalLifecycleCommand(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return ""
}

// EffectiveDockerfile returns the Dockerfile path that should be used for
// building the devcontainer image. The devcontainer spec allows build as
// either a string or an object; this helper normalises both forms. It also
// respects the legacy top-level dockerfile field when build is absent.
// Returns an empty string when neither build nor dockerfile is configured.
func (c *Config) EffectiveDockerfile() string {
	if c.Build != nil && c.Build.Dockerfile != "" {
		return c.Build.Dockerfile
	}
	return c.LegacyDockerfile
}

// EffectiveDockerComposeFiles returns the list of docker-compose files.
// It normalises the single-string form into a single-element slice.
func (c *Config) EffectiveDockerComposeFiles() []string {
	if c.DockerComposeFile == nil {
		return nil
	}
	return append([]string(nil), c.DockerComposeFile...)
}

// HasFeatures reports whether any devcontainer features are configured.
func (c *Config) HasFeatures() bool {
	return len(c.Features) > 0
}

// IsComposeProject reports whether the configuration uses Docker Compose
// (i.e. dockerComposeFile or service is set).
func (c *Config) IsComposeProject() bool {
	return len(c.DockerComposeFile) > 0
}
