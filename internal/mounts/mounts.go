package mounts

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kobus-v-schoor/dcx/internal/config"
)

// ResolvedMount is a bind mount with its source path fully resolved (home dir
// and environment variables expanded). Used by Format to produce the Docker
// --mount flag string.
type ResolvedMount struct {
	Source   string
	Target   string
	ReadOnly bool
}

// envVarPattern matches ${VAR} references in source paths for expansion.
var envVarPattern = regexp.MustCompile(`\${(.+?)}`)

// Resolve expands a config.Mount's source path and validates it exists on the
// host filesystem. It performs three transformations in order: (1) ~ expansion
// to the user's home directory, (2) ${VAR} environment variable substitution,
// and (3) filepath.Clean for path normalization. If the resolved source path
// does not exist, a warning is logged and nil is returned — the mount is
// skipped rather than causing an error. Called by BuildFlags for each mount in
// the config.
func Resolve(m config.Mount) *ResolvedMount {
	source := expandHome(m.Source)
	source = expandEnvVars(source)
	source = filepath.Clean(source)

	if _, err := os.Stat(source); err != nil {
		slog.Warn("skipping mount: source path does not exist", "source", source)
		return nil
	}

	return &ResolvedMount{
		Source:   source,
		Target:   m.Target,
		ReadOnly: m.ReadOnly,
	}
}

// Format serializes a ResolvedMount into the Docker mount format string used by
// the --mount flag. The output format is type=bind,source=...,target=... with
// an optional ,readonly suffix when ReadOnly is true. The readonly option is
// omitted when ReadOnly is false. Source and target values containing spaces or
// special characters are properly quoted for Docker's comma-delimited parser.
// Called by BuildFlags after successful resolution.
func Format(m ResolvedMount) string {
	s := fmt.Sprintf("type=bind,source=%s,target=%s", quoteMountValue(m.Source), quoteMountValue(m.Target))
	if m.ReadOnly {
		s += ",readonly"
	}
	return s
}

// BuildFlags resolves each mount from the config and produces --mount flag
// pairs for the devcontainer CLI. Mounts whose source paths don't exist on the
// host are skipped with a warning. Duplicate targets across the resolved mount
// list produce a warning but are still passed through — Docker will handle the
// conflict. Returns nil when the mount list is empty or all mounts are skipped.
func BuildFlags(cfgMounts []config.Mount) []string {
	if len(cfgMounts) == 0 {
		return nil
	}

	var flags []string
	seenTargets := make(map[string]int)

	for _, m := range cfgMounts {
		resolved := Resolve(m)
		if resolved == nil {
			continue
		}

		if count, ok := seenTargets[resolved.Target]; ok {
			slog.Warn("duplicate mount target", "target", resolved.Target, "occurrence", count+1)
		}
		seenTargets[resolved.Target]++

		flags = append(flags, "--mount", Format(*resolved))
	}

	if len(flags) == 0 {
		return nil
	}

	return flags
}

// expandHome replaces a leading ~/ in the path with the user's home directory.
// Uses os.UserHomeDir which respects HOME on Unix and the appropriate directory
// on Windows. Paths without a leading ~/ are returned unchanged.
func expandHome(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return path
	}

	return filepath.Join(homeDir, path[2:])
}

// expandEnvVars replaces all ${VAR} references in the path with the
// corresponding environment variable value. References to unset variables are
// left unchanged in the string so the path can be inspected or logged for
// debugging. This is intentionally simpler than os.ExpandEnv to avoid
// expanding $VAR (without braces) which could inadvertently affect Docker
// mount syntax.
func expandEnvVars(path string) string {
	return envVarPattern.ReplaceAllStringFunc(path, func(match string) string {
		varName := envVarPattern.FindStringSubmatch(match)[1]
		if val, ok := os.LookupEnv(varName); ok {
			return val
		}
		return match
	})
}

// quoteMountValue wraps a mount source or target value in double quotes if it
// contains characters that would interfere with Docker's comma-delimited --mount
// parsing (spaces, commas, or equals signs). Docker accepts quoted values in
// --mount strings to handle such paths correctly.
func quoteMountValue(v string) string {
	if strings.ContainsAny(v, " ,=") {
		return `"` + v + `"`
	}
	return v
}
