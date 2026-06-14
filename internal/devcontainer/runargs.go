package devcontainer

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/docker/go-connections/nat"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
)

// ParsedRunArgs holds the subset of Docker run flags that the native dcx up
// path supports. Unsupported flags produce an explicit error during parsing.
type ParsedRunArgs struct {
	PortBindings   network.PortMap
	NetworkMode    container.NetworkMode
	CapAdd         []string
	CapDrop        []string
	Privileged     bool
	SecurityOpt    []string
	Init           *bool
	Devices        []container.DeviceMapping
	Env            map[string]string
	Mounts         []mount.Mount
	Binds          []string
	Hostname       string
	WorkingDir     string
	Entrypoint     []string
	ReadonlyRootfs bool
	GroupAdd       []string
	Memory         int64
	NanoCPUs       int64
	Ulimits        []*container.Ulimit
	Tmpfs          map[string]string
}

// ParseRunArgs iterates over a runArgs string slice (devcontainer.json format)
// and produces a ParsedRunArgs. It supports both "--flag value" and
// "--flag=value" (and "-p8080:80" shorthand) forms. Unsupported flags return
// an explicit error so users know the native path does not handle them yet.
func ParseRunArgs(args []string) (*ParsedRunArgs, error) {
	result := &ParsedRunArgs{
		Env:    make(map[string]string),
		Tmpfs:  make(map[string]string),
		Mounts: make([]mount.Mount, 0),
		Binds:  make([]string, 0),
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		var flag, value string
		var hasValue bool

		// Detect --flag=value or -f=value forms.
		if idx := strings.Index(arg, "="); idx >= 0 {
			flag = arg[:idx]
			value = arg[idx+1:]
			hasValue = true
		} else if strings.HasPrefix(arg, "--") {
			flag = arg
			// Check if the next arg is a value (not another flag).
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				value = args[i+1]
				hasValue = true
				i++
			}
		} else if strings.HasPrefix(arg, "-") && len(arg) > 2 {
			// Shorthand like -p8080:80 (no space between flag and value).
			flag = arg[:2]
			value = arg[2:]
			hasValue = true
		} else if strings.HasPrefix(arg, "-") && len(arg) == 2 {
			// Single-letter shorthand like -p, -v, -m that takes a
			// separate value argument.
			flag = arg
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				value = args[i+1]
				hasValue = true
				i++
			}
		} else {
			flag = arg
		}

		switch flag {
		case "--publish", "-p":
			if !hasValue {
				return nil, fmt.Errorf("runArg %q requires a value", flag)
			}
			if err := parsePublishFlag(result, value); err != nil {
				return nil, fmt.Errorf("parsing %q: %w", flag, err)
			}
		case "--network":
			if !hasValue {
				return nil, fmt.Errorf("runArg %q requires a value", flag)
			}
			result.NetworkMode = container.NetworkMode(value)
		case "--cap-add":
			if !hasValue {
				return nil, fmt.Errorf("runArg %q requires a value", flag)
			}
			result.CapAdd = append(result.CapAdd, value)
		case "--cap-drop":
			if !hasValue {
				return nil, fmt.Errorf("runArg %q requires a value", flag)
			}
			result.CapDrop = append(result.CapDrop, value)
		case "--privileged":
			result.Privileged = true
		case "--security-opt":
			if !hasValue {
				return nil, fmt.Errorf("runArg %q requires a value", flag)
			}
			result.SecurityOpt = append(result.SecurityOpt, value)
		case "--init":
			result.Init = boolPtr(true)
		case "--device":
			if !hasValue {
				return nil, fmt.Errorf("runArg %q requires a value", flag)
			}
			if err := parseDeviceFlag(result, value); err != nil {
				return nil, fmt.Errorf("parsing %q: %w", flag, err)
			}
		case "--env", "-e":
			if !hasValue {
				return nil, fmt.Errorf("runArg %q requires a value", flag)
			}
			if err := parseEnvFlag(result, value); err != nil {
				return nil, fmt.Errorf("parsing %q: %w", flag, err)
			}
		case "--mount":
			if !hasValue {
				return nil, fmt.Errorf("runArg %q requires a value", flag)
			}
			m, err := parseMountFlag(value)
			if err != nil {
				return nil, fmt.Errorf("parsing %q: %w", flag, err)
			}
			result.Mounts = append(result.Mounts, m)
		case "--volume", "-v":
			if !hasValue {
				return nil, fmt.Errorf("runArg %q requires a value", flag)
			}
			result.Binds = append(result.Binds, value)
		case "--memory", "-m":
			if !hasValue {
				return nil, fmt.Errorf("runArg %q requires a value", flag)
			}
			mem, err := parseMemoryFlag(value)
			if err != nil {
				return nil, err
			}
			result.Memory = mem
		case "--cpus":
			if !hasValue {
				return nil, fmt.Errorf("runArg %q requires a value", flag)
			}
			cpus, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid --cpus value %q: %w", value, err)
			}
			result.NanoCPUs = int64(cpus * 1e9)
		case "--hostname":
			if !hasValue {
				return nil, fmt.Errorf("runArg %q requires a value", flag)
			}
			result.Hostname = value
		case "--workdir", "-w":
			if !hasValue {
				return nil, fmt.Errorf("runArg %q requires a value", flag)
			}
			result.WorkingDir = value
		case "--entrypoint":
			if !hasValue {
				return nil, fmt.Errorf("runArg %q requires a value", flag)
			}
			result.Entrypoint = []string{value}
		case "--group-add":
			if !hasValue {
				return nil, fmt.Errorf("runArg %q requires a value", flag)
			}
			result.GroupAdd = append(result.GroupAdd, value)
		case "--read-only":
			result.ReadonlyRootfs = true
		case "--tmpfs":
			if !hasValue {
				return nil, fmt.Errorf("runArg %q requires a value", flag)
			}
			if err := parseTmpfsFlag(result, value); err != nil {
				return nil, fmt.Errorf("parsing %q: %w", flag, err)
			}
		default:
			return nil, fmt.Errorf("unsupported runArg %q: file an issue or continue using the devcontainer CLI path", arg)
		}
	}

	return result, nil
}

// parsePublishFlag parses a Docker --publish argument and adds it to the
// ParsedRunArgs PortBindings using the Docker go-connections nat parser.
func parsePublishFlag(r *ParsedRunArgs, value string) error {
	mappings, err := nat.ParsePortSpec(value)
	if err != nil {
		return err
	}
	if r.PortBindings == nil {
		r.PortBindings = make(network.PortMap)
	}
	for _, pm := range mappings {
		port, err := network.ParsePort(string(pm.Port))
		if err != nil {
			return fmt.Errorf("parsing port %q: %w", pm.Port, err)
		}
		binding := network.PortBinding{HostPort: pm.Binding.HostPort}
		if pm.Binding.HostIP != "" {
			addr, err := netip.ParseAddr(pm.Binding.HostIP)
			if err != nil {
				return fmt.Errorf("parsing host IP %q: %w", pm.Binding.HostIP, err)
			}
			binding.HostIP = addr
		}
		r.PortBindings[port] = append(r.PortBindings[port], binding)
	}
	return nil
}

// parseDeviceFlag parses a Docker --device argument.
func parseDeviceFlag(r *ParsedRunArgs, value string) error {
	// Format: hostPath[:containerPath][:permissions]
	parts := strings.Split(value, ":")
	dm := container.DeviceMapping{
		PathOnHost:        parts[0],
		PathInContainer:   parts[0],
		CgroupPermissions: "rwm",
	}
	if len(parts) > 1 {
		dm.PathInContainer = parts[1]
	}
	if len(parts) > 2 {
		dm.CgroupPermissions = parts[2]
	}
	r.Devices = append(r.Devices, dm)
	return nil
}

// parseEnvFlag parses a Docker --env argument.
func parseEnvFlag(r *ParsedRunArgs, value string) error {
	name, val, _ := strings.Cut(value, "=")
	if val == "" && !strings.Contains(value, "=") {
		// Value is not of the form FOO=bar; treat the whole thing as a
		// name and look it up in the host environment.
		val = ""
	}
	r.Env[name] = val
	return nil
}

// parseMountFlag parses a Docker --mount argument into a mount.Mount.
func parseMountFlag(value string) (mount.Mount, error) {
	// Format: type=bind,source=/host,target=/container[,readonly,...]
	parts := strings.Split(value, ",")
	m := mount.Mount{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, val, ok := strings.Cut(part, "=")
		if !ok {
			// Flags without values.
			switch part {
			case "readonly", "ro":
				m.ReadOnly = true
			case "bind-propagation=rprivate":
				if m.BindOptions == nil {
					m.BindOptions = &mount.BindOptions{}
				}
				m.BindOptions.Propagation = mount.PropagationRPrivate
			default:
				// Unrecognised option; skip.
			}
			continue
		}
		name = strings.TrimSpace(name)
		val = strings.TrimSpace(val)
		switch name {
		case "type":
			m.Type = mount.Type(val)
		case "source", "src":
			m.Source = val
		case "target", "dst", "destination":
			m.Target = val
		case "readonly", "ro":
			if val == "true" || val == "" {
				m.ReadOnly = true
			}
		case "bind-propagation":
			if m.BindOptions == nil {
				m.BindOptions = &mount.BindOptions{}
			}
			m.BindOptions.Propagation = mount.Propagation(val)
		case "consistency":
			m.Consistency = mount.Consistency(val)
		case "volume-nocopy":
			if m.VolumeOptions == nil {
				m.VolumeOptions = &mount.VolumeOptions{}
			}
			m.VolumeOptions.NoCopy = val == "true"
		case "tmpfs-size":
			if m.TmpfsOptions == nil {
				m.TmpfsOptions = &mount.TmpfsOptions{}
			}
			size, err := parseMemoryFlag(val)
			if err != nil {
				return mount.Mount{}, fmt.Errorf("parsing tmpfs-size: %w", err)
			}
			m.TmpfsOptions.SizeBytes = size
		}
	}
	if m.Type == "" {
		return mount.Mount{}, fmt.Errorf("mount string missing required field 'type'")
	}
	if m.Source == "" {
		return mount.Mount{}, fmt.Errorf("mount string missing required field 'source'")
	}
	if m.Target == "" {
		return mount.Mount{}, fmt.Errorf("mount string missing required field 'target'")
	}
	return m, nil
}

// parseMemoryFlag converts human-readable memory strings (e.g. "512m", "1g")
// into bytes.
func parseMemoryFlag(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("empty memory value")
	}

	// Use a suffix-first matching strategy for common Docker memory units.
	lower := strings.ToLower(value)
	switch {
	case strings.HasSuffix(lower, "kib"):
		return parseMemoryNumber(value[:len(value)-3], 1024, value)
	case strings.HasSuffix(lower, "mib"):
		return parseMemoryNumber(value[:len(value)-3], 1024*1024, value)
	case strings.HasSuffix(lower, "gib"):
		return parseMemoryNumber(value[:len(value)-3], 1024*1024*1024, value)
	case strings.HasSuffix(lower, "tib"):
		return parseMemoryNumber(value[:len(value)-3], 1024*1024*1024*1024, value)
	case strings.HasSuffix(lower, "kb"):
		return parseMemoryNumber(value[:len(value)-2], 1000, value)
	case strings.HasSuffix(lower, "mb"):
		return parseMemoryNumber(value[:len(value)-2], 1000*1000, value)
	case strings.HasSuffix(lower, "gb"):
		return parseMemoryNumber(value[:len(value)-2], 1000*1000*1000, value)
	case strings.HasSuffix(lower, "tb"):
		return parseMemoryNumber(value[:len(value)-2], 1000*1000*1000*1000, value)
	case strings.HasSuffix(lower, "k"):
		return parseMemoryNumber(value[:len(value)-1], 1024, value)
	case strings.HasSuffix(lower, "m"):
		return parseMemoryNumber(value[:len(value)-1], 1024*1024, value)
	case strings.HasSuffix(lower, "g"):
		return parseMemoryNumber(value[:len(value)-1], 1024*1024*1024, value)
	case strings.HasSuffix(lower, "t"):
		return parseMemoryNumber(value[:len(value)-1], 1024*1024*1024*1024, value)
	case strings.HasSuffix(lower, "b"):
		return parseMemoryNumber(value[:len(value)-1], 1, value)
	default:
		return parseMemoryNumber(value, 1, value)
	}
}

func parseMemoryNumber(numStr string, unit int64, original string) (int64, error) {
	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory value %q: %w", original, err)
	}
	return int64(num * float64(unit)), nil
}

// parseTmpfsFlag parses a Docker --tmpfs argument.
func parseTmpfsFlag(r *ParsedRunArgs, value string) error {
	// Format: /mountpoint[:options]
	parts := strings.SplitN(value, ":", 2)
	mountpoint := parts[0]
	options := ""
	if len(parts) > 1 {
		options = parts[1]
	}
	r.Tmpfs[mountpoint] = options
	return nil
}

func boolPtr(b bool) *bool {
	return &b
}
