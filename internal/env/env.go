// Package env implements environment variable passthrough from the host to the
// devcontainer. It reads EnvVar declarations from the dcx config, resolves host
// environment variable values, and returns them for injection into the override
// devcontainer.json's containerEnv property.
package env

import (
	"log/slog"
	"os"
	"regexp"
	"strings"

	"github.com/kobus-v-schoor/dcx/internal/config"
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
