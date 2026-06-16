// Package spec provides a strongly-typed Go representation of the
// devcontainer.json configuration properties that dcx cares about.
// It handles polymorphic fields (build as string or object, dockerComposeFile
// as string or array) via custom JSON marshal/unmarshal methods.
package spec

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// ForwardPort represents an entry in the devcontainer.json forwardPorts
// array, which may be an integer port number or a "host:port" string per
// the official spec. The underlying raw JSON is preserved so the original
// form survives a marshal round-trip.
type ForwardPort json.RawMessage

// UnmarshalJSON implements json.Unmarshaler for ForwardPort.
func (f *ForwardPort) UnmarshalJSON(data []byte) error {
	*f = ForwardPort(append([]byte(nil), data...))
	return nil
}

// MarshalJSON implements json.Marshaler for ForwardPort.
func (f ForwardPort) MarshalJSON() ([]byte, error) {
	if f == nil {
		return []byte("null"), nil
	}
	return []byte(f), nil
}

// AsInt returns the port as an int when the raw JSON is an integer.
func (f ForwardPort) AsInt() (int, bool) {
	var i int
	if err := json.Unmarshal(f, &i); err == nil {
		return i, true
	}
	return 0, false
}

// AsString returns the port as a string when the raw JSON is a string.
func (f ForwardPort) AsString() (string, bool) {
	var s string
	if err := json.Unmarshal(f, &s); err == nil {
		return s, true
	}
	return "", false
}

// String returns the best string representation of the port.
func (f ForwardPort) String() string {
	if s, ok := f.AsString(); ok {
		return s
	}
	if i, ok := f.AsInt(); ok {
		return strconv.Itoa(i)
	}
	return ""
}

// IsEmpty reports whether the forward port has no value.
func (f ForwardPort) IsEmpty() bool {
	return len(f) == 0 || string(f) == "null"
}

// Clone returns a deep copy of the ForwardPort.
func (f ForwardPort) Clone() ForwardPort {
	if f == nil {
		return nil
	}
	return ForwardPort(append([]byte(nil), f...))
}

// NewForwardPortInt creates a ForwardPort from an integer.
func NewForwardPortInt(i int) ForwardPort {
	raw, _ := json.Marshal(i)
	return ForwardPort(raw)
}

// NewForwardPortString creates a ForwardPort from a string.
func NewForwardPortString(s string) ForwardPort {
	raw, _ := json.Marshal(s)
	return ForwardPort(raw)
}

// MountEntry represents a single mount in the devcontainer.json mounts array.
// The spec accepts both Docker --mount format strings and structured mount
// objects. The original JSON is preserved so the form survives round-trip.
type MountEntry json.RawMessage

// UnmarshalJSON implements json.Unmarshaler for MountEntry.
func (m *MountEntry) UnmarshalJSON(data []byte) error {
	*m = MountEntry(append([]byte(nil), data...))
	return nil
}

// MarshalJSON implements json.Marshaler for MountEntry.
func (m MountEntry) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	return []byte(m), nil
}

// AsString returns the mount as a string when the raw JSON is a string.
func (m MountEntry) AsString() (string, bool) {
	var s string
	if err := json.Unmarshal(m, &s); err == nil {
		return s, true
	}
	return "", false
}

// IsEmpty reports whether the mount entry has no value.
func (m MountEntry) IsEmpty() bool {
	return len(m) == 0 || string(m) == "null"
}

// Clone returns a deep copy of the MountEntry.
func (m MountEntry) Clone() MountEntry {
	if m == nil {
		return nil
	}
	return MountEntry(append([]byte(nil), m...))
}

// NewMountEntryString creates a MountEntry from a Docker --mount string.
func NewMountEntryString(s string) MountEntry {
	raw, _ := json.Marshal(s)
	return MountEntry(raw)
}

// LifecycleCommand represents a devcontainer lifecycle command (e.g.
// postCreateCommand, postStartCommand). The spec accepts string, array,
// and object forms. The original JSON is preserved so the form survives
// round-trip.
type LifecycleCommand json.RawMessage

// UnmarshalJSON implements json.Unmarshaler for LifecycleCommand.
func (lc *LifecycleCommand) UnmarshalJSON(data []byte) error {
	*lc = LifecycleCommand(append([]byte(nil), data...))
	return nil
}

// MarshalJSON implements json.Marshaler for LifecycleCommand.
func (lc LifecycleCommand) MarshalJSON() ([]byte, error) {
	if lc == nil {
		return []byte("null"), nil
	}
	return []byte(lc), nil
}

// IsEmpty reports whether the lifecycle command has no value.
func (lc LifecycleCommand) IsEmpty() bool {
	return len(lc) == 0 || string(lc) == "null"
}

// AsString returns the command as a string when the raw JSON is a string.
func (lc LifecycleCommand) AsString() (string, bool) {
	var s string
	if err := json.Unmarshal(lc, &s); err == nil {
		return s, true
	}
	return "", false
}

// AsArray returns the command as a string slice when the raw JSON is an array.
func (lc LifecycleCommand) AsArray() ([]string, bool) {
	var arr []string
	if err := json.Unmarshal(lc, &arr); err == nil {
		return arr, true
	}
	return nil, false
}

// AsObject returns the command as a map when the raw JSON is an object.
func (lc LifecycleCommand) AsObject() (map[string]interface{}, bool) {
	var obj map[string]interface{}
	if err := json.Unmarshal(lc, &obj); err == nil {
		return obj, true
	}
	return nil, false
}

// Clone returns a deep copy of the LifecycleCommand.
func (lc LifecycleCommand) Clone() LifecycleCommand {
	if lc == nil {
		return nil
	}
	return LifecycleCommand(append([]byte(nil), lc...))
}

// NewLifecycleCommandString creates a LifecycleCommand from a string.
func NewLifecycleCommandString(s string) LifecycleCommand {
	raw, _ := json.Marshal(s)
	return LifecycleCommand(raw)
}

// NewLifecycleCommandArray creates a LifecycleCommand from a string slice.
func NewLifecycleCommandArray(cmds ...string) LifecycleCommand {
	raw, _ := json.Marshal(cmds)
	return LifecycleCommand(raw)
}

// NewLifecycleCommandObject creates a LifecycleCommand from a map.
func NewLifecycleCommandObject(obj map[string]interface{}) LifecycleCommand {
	raw, _ := json.Marshal(obj)
	return LifecycleCommand(raw)
}

// Config represents the devcontainer.json configuration properties that dcx
// cares about. Optional fields use pointer types so that absent, zero, and
// explicit values can be distinguished after parsing. Fields with json:"-"
// tags are handled by custom MarshalJSON / UnmarshalJSON because they accept
// more than one JSON type.
type Config struct {
	Name                 string                     `json:"name,omitempty"`
	Image                string                     `json:"image,omitempty"`
	Build                *Build                     `json:"-"`
	LegacyDockerfile     string                     `json:"dockerFile,omitempty"`
	DockerComposeFile    []string                   `json:"-"`
	Service              string                     `json:"service,omitempty"`
	RunServices          []string                   `json:"runServices,omitempty"`
	WorkspaceFolder      string                     `json:"workspaceFolder,omitempty"`
	WorkspaceMount       string                     `json:"workspaceMount,omitempty"`
	RemoteUser           string                     `json:"remoteUser,omitempty"`
	ContainerUser        string                     `json:"containerUser,omitempty"`
	ContainerEnv         map[string]string          `json:"containerEnv,omitempty"`
	RemoteEnv            map[string]string          `json:"remoteEnv,omitempty"`
	Mounts               []MountEntry               `json:"mounts,omitempty"`
	Features             map[string]json.RawMessage `json:"features,omitempty"`
	OnCreateCommand      LifecycleCommand           `json:"onCreateCommand,omitempty"`
	UpdateContentCommand LifecycleCommand           `json:"updateContentCommand,omitempty"`
	PostCreateCommand    LifecycleCommand           `json:"postCreateCommand,omitempty"`
	PostStartCommand     LifecycleCommand           `json:"postStartCommand,omitempty"`
	PostAttachCommand    LifecycleCommand           `json:"postAttachCommand,omitempty"`
	InitializeCommand    LifecycleCommand           `json:"initializeCommand,omitempty"` // Unsupported: not implemented by dcx.
	RunArgs              []string                   `json:"runArgs,omitempty"`
	ShutdownAction       string                     `json:"shutdownAction,omitempty"` // Unsupported: not implemented by dcx.
	OverrideCommand      *bool                      `json:"overrideCommand,omitempty"`
	UpdateRemoteUserUID  *bool                      `json:"updateRemoteUserUID,omitempty"` // Unsupported: not implemented by dcx.
	ForwardPorts         []ForwardPort              `json:"forwardPorts,omitempty"`        // Unsupported: not implemented by dcx.
	PortsAttributes      map[string]json.RawMessage `json:"portsAttributes,omitempty"`     // Unsupported: not implemented by dcx.
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
// fields (build and dockerComposeFile) are parsed correctly. Unknown fields are
// ignored. Lifecycle commands are handled by their own LifecycleCommand
// UnmarshalJSON so string, array, and object forms are all preserved. Called by
// json.Unmarshal when loading a devcontainer.json.
func (c *Config) UnmarshalJSON(data []byte) error {
	// Unmarshal the straightforward fields into a shadow struct. Fields that
	// can have multiple JSON shapes are captured as json.RawMessage and
	// parsed manually afterwards.
	type plain Config
	var shadow struct {
		*plain
		Build             json.RawMessage `json:"build"`
		DockerComposeFile json.RawMessage `json:"dockerComposeFile"`
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

	return nil
}

// MarshalJSON implements custom marshaling for Config so that polymorphic
// fields are written back in the shape expected by the devcontainer spec.
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
		// devcontainer spec accepts the shorthand form.
		shadow.Build = c.Build
	}

	if len(c.DockerComposeFile) == 1 {
		shadow.DockerComposeFile = c.DockerComposeFile[0]
	} else if len(c.DockerComposeFile) > 1 {
		shadow.DockerComposeFile = c.DockerComposeFile
	}

	return json.Marshal(shadow)
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
