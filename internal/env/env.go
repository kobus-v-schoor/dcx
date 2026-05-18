// Package env implements environment variable passthrough from the host to the
// devcontainer. It reads EnvVar declarations from the dcx config, resolves host
// environment variable values, and returns them for injection into the override
// devcontainer.json's containerEnv property.
package env

import (
	"os"
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

// Resolve parses an EnvVar declaration and resolves the host environment
// variable value. Two formats are supported:
//
//   - "NAME" — shorthand: reads the host env var NAME, sets NAME in the
//     container. Equivalent to "NAME=${NAME}".
//   - "CONTAINER_NAME=${HOST_VAR}" — explicit: reads HOST_VAR from the host
//     environment and assigns its value to CONTAINER_NAME in the container.
//
// If the referenced host variable is not set, nil is returned (the variable is
// silently skipped). This allows declaring a superset of variables where only
// those existing on the current machine are forwarded. Called by BuildFlags for
// each environment entry in the config.
func Resolve(ev config.EnvVar) *ResolvedEnv {
	s := string(ev)

	// Determine the container-side name and the host variable reference.
	var containerName, hostVarRef string

	if idx := strings.Index(s, "="); idx >= 0 {
		// Explicit form: CONTAINER_NAME=${HOST_VAR}
		containerName = s[:idx]
		hostVarRef = s[idx+1:]
	} else {
		// Shorthand form: NAME (equivalent to NAME=${NAME})
		containerName = s
		hostVarRef = "${" + s + "}"
	}

	// Parse the ${VAR} reference from the host variable specification.
	// The value after '=' must be in ${VAR} format.
	hostVarName := parseHostVarRef(hostVarRef)
	if hostVarName == "" {
		// If the reference isn't in ${VAR} format, treat the entire
		// right-hand side as a literal value. This supports edge cases
		// where the user wants to set a fixed value via config.
		return &ResolvedEnv{
			Name:  containerName,
			Value: hostVarRef,
		}
	}

	// Look up the host environment variable. If not set, silently skip.
	hostValue, ok := os.LookupEnv(hostVarName)
	if !ok {
		return nil
	}

	return &ResolvedEnv{
		Name:  containerName,
		Value: hostValue,
	}
}

// parseHostVarRef extracts the variable name from a ${VAR} reference string.
// Returns the variable name without braces, or an empty string if the input
// is not in ${VAR} format. Only supports the ${VAR} form (with braces) to
// match the config syntax specified in the issue.
func parseHostVarRef(ref string) string {
	if len(ref) < 3 || !strings.HasPrefix(ref, "${") || !strings.HasSuffix(ref, "}") {
		return ""
	}
	inner := ref[2 : len(ref)-1]
	if inner == "" {
		return ""
	}
	return inner
}

// ResolveAll resolves each environment variable declaration from the config
// and returns the set of successfully resolved entries. Entries whose
// referenced host variable is not set are silently skipped. Returns nil when
// the environment list is empty or all entries are skipped. Called by the
// cli package during dcx up to collect env vars for override config injection.
func ResolveAll(envVars []config.EnvVar) []ResolvedEnv {
	if len(envVars) == 0 {
		return nil
	}

	var result []ResolvedEnv

	for _, ev := range envVars {
		resolved := Resolve(ev)
		if resolved == nil {
			continue
		}
		result = append(result, *resolved)
	}

	return result
}
