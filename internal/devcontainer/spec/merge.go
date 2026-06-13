package spec

import "encoding/json"

// Merge returns a new Config that is the deep merge of base and override.
// Override values win on conflict. Arrays are replaced rather than merged,
// except for Mounts which are concatenated because devcontainer mount
// semantics are additive (user mounts + dcx-injected mounts). Maps are
// shallow-merged at the top level: override keys override base keys; keys
// present only in base are preserved.
func Merge(base, override *Config) *Config {
	if base == nil {
		if override == nil {
			return &Config{}
		}
		return deepCopy(override)
	}
	if override == nil {
		return deepCopy(base)
	}

	result := deepCopy(base)

	// Scalar string fields: override wins when non-empty. Empty string in
	// the override is treated as "not set" because the devcontainer spec
	// does not use empty string as a meaningful value for these fields.
	if override.Name != "" {
		result.Name = override.Name
	}
	if override.Image != "" {
		result.Image = override.Image
	}
	if override.Service != "" {
		result.Service = override.Service
	}
	if override.WorkspaceFolder != "" {
		result.WorkspaceFolder = override.WorkspaceFolder
	}
	if override.WorkspaceMount != "" {
		result.WorkspaceMount = override.WorkspaceMount
	}
	if override.RemoteUser != "" {
		result.RemoteUser = override.RemoteUser
	}
	if override.ContainerUser != "" {
		result.ContainerUser = override.ContainerUser
	}
	if override.ShutdownAction != "" {
		result.ShutdownAction = override.ShutdownAction
	}
	if override.LegacyDockerfile != "" {
		result.LegacyDockerfile = override.LegacyDockerfile
	}

	// Build: override replaces entirely.
	if override.Build != nil {
		result.Build = deepCopyBuild(override.Build)
	}

	// dockerComposeFile: override replaces.
	if override.DockerComposeFile != nil {
		result.DockerComposeFile = append([]string(nil), override.DockerComposeFile...)
	}

	// runServices: override replaces.
	if override.RunServices != nil {
		result.RunServices = append([]string(nil), override.RunServices...)
	}

	// Map fields: shallow merge, override keys win.
	result.ContainerEnv = mergeStringMaps(base.ContainerEnv, override.ContainerEnv)
	result.RemoteEnv = mergeStringMaps(base.RemoteEnv, override.RemoteEnv)
	result.Features = mergeRawMessageMaps(base.Features, override.Features)
	result.PortsAttributes = mergeRawMessageMaps(base.PortsAttributes, override.PortsAttributes)

	// Mounts: concatenated (additive semantics).
	result.Mounts = concatenateMounts(base.Mounts, override.Mounts)

	// Lifecycle commands: override wins when non-empty.
	if override.PostCreateCommand != "" {
		result.PostCreateCommand = override.PostCreateCommand
	}
	if override.PostStartCommand != "" {
		result.PostStartCommand = override.PostStartCommand
	}
	if override.PostAttachCommand != "" {
		result.PostAttachCommand = override.PostAttachCommand
	}
	if override.InitializeCommand != "" {
		result.InitializeCommand = override.InitializeCommand
	}

	// Arrays: override replaces.
	if override.RunArgs != nil {
		result.RunArgs = append([]string(nil), override.RunArgs...)
	}
	if override.ForwardPorts != nil {
		result.ForwardPorts = append([]int(nil), override.ForwardPorts...)
	}

	// Pointer fields: override wins when non-nil.
	if override.OverrideCommand != nil {
		result.OverrideCommand = override.OverrideCommand
	}
	if override.UpdateRemoteUserUID != nil {
		result.UpdateRemoteUserUID = override.UpdateRemoteUserUID
	}

	return result
}

// deepCopy creates a deep copy of a Config by round-tripping through JSON.
// This preserves all known fields and is simpler than writing an explicit
// deep-copy for every field. Configs are small enough that the overhead is
// negligible.
func deepCopy(c *Config) *Config {
	if c == nil {
		return nil
	}
	data, err := json.Marshal(c)
	if err != nil {
		panic("unreachable: Config marshaling cannot fail: " + err.Error())
	}
	var copy Config
	if err := json.Unmarshal(data, &copy); err != nil {
		panic("unreachable: Config unmarshaling cannot fail: " + err.Error())
	}
	return &copy
}

// deepCopyBuild creates a deep copy of a Build value.
func deepCopyBuild(b *Build) *Build {
	if b == nil {
		return nil
	}
	result := &Build{
		Dockerfile: b.Dockerfile,
		Context:    b.Context,
		Target:     b.Target,
	}
	if len(b.Args) > 0 {
		result.Args = make(map[string]string, len(b.Args))
		for k, v := range b.Args {
			result.Args[k] = v
		}
	}
	return result
}

// mergeStringMaps returns a shallow merge of base and override. Keys present
// in override replace those in base; keys present only in base are
// preserved. Returns nil when both inputs are nil or empty.
func mergeStringMaps(base, override map[string]string) map[string]string {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	result := make(map[string]string, len(base)+len(override))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range override {
		result[k] = v
	}
	return result
}

// mergeRawMessageMaps returns a shallow merge of base and override where
// values are json.RawMessage. The raw bytes are copied by reference (same
// underlying slice) because json.RawMessage is immutable once produced.
func mergeRawMessageMaps(base, override map[string]json.RawMessage) map[string]json.RawMessage {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	result := make(map[string]json.RawMessage, len(base)+len(override))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range override {
		result[k] = v
	}
	return result
}

// concatenateMounts concatenates base and override mount lists. Mount
// semantics in devcontainer.json are additive, so both lists are preserved.
func concatenateMounts(base, override []string) []string {
	if len(base) == 0 {
		return append([]string(nil), override...)
	}
	if len(override) == 0 {
		return append([]string(nil), base...)
	}
	result := make([]string, 0, len(base)+len(override))
	result = append(result, base...)
	result = append(result, override...)
	return result
}
