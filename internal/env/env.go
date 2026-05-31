// Package env implements environment variable passthrough from the host to the
// devcontainer. It reads EnvVar declarations from the dcx config, resolves host
// environment variable values, and returns them for injection into the override
// devcontainer.json's containerEnv property. It also provides AutoForward which
// returns environment variables that are automatically forwarded from the host
// without requiring user configuration.
package env

import (
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/kobus-v-schoor/dcx/internal/config"
	"github.com/kobus-v-schoor/dcx/internal/mounts"
)

// ResolvedEnv holds a fully-resolved environment variable ready to be passed as
// a --remote-env flag. Name is the variable name inside the container; Value is
// the resolved host value string.
type ResolvedEnv struct {
	Name  string
	Value string
}

// varRefRegex matches ${VAR} substitution references in environment variable
// value expressions. Used by expandValue to find and replace all ${VAR}
// occurrences with their host environment values.
// GitConfigEntry represents a single git configuration key-value pair to be
// injected into the container via the GIT_CONFIG_COUNT / GIT_CONFIG_KEY_n /
// GIT_CONFIG_VALUE_n environment variable mechanism supported by git 2.31+.
type GitConfigEntry struct {
	Key   string
	Value string
}

// BuildGitConfigEnv converts a slice of GitConfigEntry values into the
// GIT_CONFIG_COUNT, GIT_CONFIG_KEY_n, and GIT_CONFIG_VALUE_n environment
// variables that git recognises. Returns nil when entries is empty. This
// centralises index generation so that callers can merge multiple
// GitConfigEntry slices without manually tracking indices.
func BuildGitConfigEnv(entries []GitConfigEntry) []ResolvedEnv {
	if len(entries) == 0 {
		return nil
	}

	result := make([]ResolvedEnv, 0, 1+len(entries)*2)
	result = append(result, ResolvedEnv{
		Name:  "GIT_CONFIG_COUNT",
		Value: strconv.Itoa(len(entries)),
	})

	for i, entry := range entries {
		result = append(result, ResolvedEnv{
			Name:  fmt.Sprintf("GIT_CONFIG_KEY_%d", i),
			Value: entry.Key,
		})
		result = append(result, ResolvedEnv{
			Name:  fmt.Sprintf("GIT_CONFIG_VALUE_%d", i),
			Value: entry.Value,
		})
	}

	return result
}

var varRefRegex = regexp.MustCompile(`\$\{([^}]+)}`)

// Resolve parses an EnvVar declaration and resolves the host environment
// variable value. Two formats are supported:
//
//   - "NAME" — shorthand: reads the host env var NAME, sets NAME in the
//     container. Equivalent to "NAME=${NAME}".
//   - "CONTAINER_NAME=${HOST_VAR}" — explicit: reads HOST_VAR from the host
//     environment and assigns its value to CONTAINER_NAME in the container.
//
// The value part (after '=') supports composite expressions that mix
// substitutions and literal text, e.g. "PATH=${PATH}:/opt/bin". Each ${VAR}
// reference is replaced with the corresponding host environment variable value.
// If any referenced host variable is not set, a warning is logged and the
// reference is replaced with an empty string. Text outside ${...} is treated as
// a literal value. If the value part contains no ${...} references at all, it
// is treated as a plain literal string (useful for setting fixed values).
// Always returns a non-nil ResolvedEnv. Called by ResolveAll for each
// environment entry in the config.
func Resolve(ev config.EnvVar) *ResolvedEnv {
	s := string(ev)

	// Determine the container-side name and the value expression.
	var containerName, valueExpr string

	if idx := strings.Index(s, "="); idx >= 0 {
		// Explicit form: CONTAINER_NAME=<value expression>
		containerName = s[:idx]
		valueExpr = s[idx+1:]
	} else {
		// Shorthand form: NAME (equivalent to NAME=${NAME})
		containerName = s
		valueExpr = "${" + s + "}"
	}

	// Expand all ${VAR} references in the value expression. If there are no
	// references, the entire valueExpr is a literal string.
	value := expandValue(valueExpr)

	return &ResolvedEnv{
		Name:  containerName,
		Value: value,
	}
}

// expandValue replaces all ${VAR} references in the value expression with
// their corresponding host environment variable values. Text outside ${...}
// is preserved as-is, allowing composite expressions like ${PATH}:/opt/bin.
// If a referenced host variable is not set, a warning is logged and the
// reference is replaced with an empty string. If the value expression contains
// no ${...} references, it is returned unchanged (treated as a literal).
func expandValue(expr string) string {
	// If there are no ${...} patterns at all, return the expression as a literal.
	if !varRefRegex.MatchString(expr) {
		return expr
	}

	return varRefRegex.ReplaceAllStringFunc(expr, func(match string) string {
		// Extract the variable name from ${VAR}.
		varName := match[2 : len(match)-1]
		if varName == "" {
			return ""
		}

		hostValue, ok := os.LookupEnv(varName)
		if !ok {
			slog.Warn("referenced host variable not set, substituting empty string", "variable", varName)
			return ""
		}
		return hostValue
	})
}

// autoForwardNames lists environment variable names that are automatically
// forwarded from the host to the devcontainer without requiring user
// configuration. These variables are essential for correct terminal behaviour
// inside the container. If a listed variable is not set on the host, it is
// silently skipped (no warning). User-configured environment entries take
// precedence over auto-forwarded variables when both set the same
// container-side name.
var autoForwardNames = []string{
	"TERM",
	"COLORTERM",
}

// AutoForward returns resolved environment variables that are automatically
// forwarded from the host to the devcontainer. Currently this includes TERM
// and COLORTERM, which ensures that TUI applications making use of advanced
// terminal features (like true-colour support) work as expected inside the
// container.
// Variables that are not set on the host are silently skipped. Called by the
// cli package during dcx up before user-configured env vars are resolved, so
// that user config takes precedence on name conflict.
func AutoForward() []ResolvedEnv {
	var result []ResolvedEnv

	for _, name := range autoForwardNames {
		value, ok := os.LookupEnv(name)
		if !ok {
			continue
		}

		result = append(result, ResolvedEnv{
			Name:  name,
			Value: value,
		})
	}

	return result
}

// TerminfoResult holds the mount and environment variable produced by
// ForwardTerminfo. Either the Mount field is populated (terminfo forwarding
// active) or all fields are zero-valued (terminfo not set or directory does
// not exist).
type TerminfoResult struct {
	Mount    *mounts.ResolvedMount
	EnvName  string
	EnvValue string
}

// ForwardTerminfo checks the host environment for the TERMINFO variable. If
// it is set and points to an existing directory, it returns a TerminfoResult
// with a read-only bind mount to /opt/dcx/terminfo and the TERMINFO env var
// set to that container path. This ensures that TUI applications using
// terminal emulators not present in the container's default terminfo database
// work correctly inside the container. If TERMINFO is unset or the path does
// not exist, an empty result is returned and a warning is logged. Called by
// the cli package during dcx up alongside AutoForward.
func ForwardTerminfo() TerminfoResult {
	path := os.Getenv("TERMINFO")
	if path == "" {
		return TerminfoResult{}
	}

	info, err := os.Stat(path)
	if err != nil {
		slog.Warn("TERMINFO is set but path does not exist, skipping terminfo forwarding", "path", path)
		return TerminfoResult{}
	}
	if !info.IsDir() {
		slog.Warn("TERMINFO is set but path is not a directory, skipping terminfo forwarding", "path", path)
		return TerminfoResult{}
	}

	return TerminfoResult{
		Mount: &mounts.ResolvedMount{
			Source:   path,
			Target:   "/opt/dcx/terminfo",
			ReadOnly: true,
		},
		EnvName:  "TERMINFO",
		EnvValue: "/opt/dcx/terminfo",
	}
}

// ResolveAll resolves each environment variable declaration from the config
// and returns the set of resolved entries. Referenced host variables that are
// not set produce a warning and are substituted with empty strings. Returns
// nil when the environment list is empty. Called by the cli package during
// dcx up to collect env vars for override config injection.
func ResolveAll(envVars []config.EnvVar) []ResolvedEnv {
	if len(envVars) == 0 {
		return nil
	}

	result := make([]ResolvedEnv, 0, len(envVars))

	for _, ev := range envVars {
		resolved := Resolve(ev)
		result = append(result, *resolved)
	}

	return result
}
