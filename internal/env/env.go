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
	"os/exec"
	"path/filepath"
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

// TerminfoResult holds the mount and postCreateCommand produced by
// PrepareTerminfo. Either the Mount field is populated (terminfo forwarding
// active) or all fields are zero-valued (terminfo not set or infocmp missing).
// Unlike the previous TERMINFO directory bind-mount approach, this uses
// infocmp to capture the host terminal's source description and tic inside the
// container to compile it into the container user's ~/.terminfo directory.
type TerminfoResult struct {
	Mount             *mounts.ResolvedMount
	PostCreateCommand string
}

// PrepareTerminfo checks the host environment for the TERM variable and
// attempts to capture the local terminal's terminfo source using infocmp.
// It writes the source into a stable file under the user's home directory
// (~/.config/dcx/terminfo.src) so the bind mount survives container
// stop/start cycles. Within the container, a postCreateCommand compiles the
// source with tic and installs it into the container user's ~/.terminfo.
//
// If TERM is not set, infocmp is not on PATH, or infocmp fails (e.g. the
// terminal entry is unknown), an empty result is returned and a warning is
// logged. Called by the cli package during dcx up.
func PrepareTerminfo(containerHomeDir string) TerminfoResult {
	if containerHomeDir == "" {
		return TerminfoResult{}
	}

	term := os.Getenv("TERM")
	if term == "" {
		return TerminfoResult{}
	}

	infocmpPath, err := exec.LookPath("infocmp")
	if err != nil {
		slog.Warn("infocmp not found on host, skipping terminfo forwarding")
		return TerminfoResult{}
	}

	// -x includes extended capabilities, needed for modern terminal emulators
	// such as Ghostty to preserve full feature support after compilation.
	cmd := exec.Command(infocmpPath, "-x", term)
	src, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			slog.Warn("infocmp failed for terminal entry, skipping terminfo forwarding", "term", term, "stderr", string(exitErr.Stderr))
		} else {
			slog.Warn("infocmp execution failed, skipping terminfo forwarding", "term", term, "error", err)
		}
		return TerminfoResult{}
	}

	// Write the infocmp output to a stable file under the dcx config directory
	// so the bind mount source persists across dcx up invocations and container
	// stop/start cycles. A dedicated terminfo subdirectory holds one file per
	// TERM value so that switching terminals never causes clashes.
	cacheDir, err := config.UserConfigDir()
	if err != nil {
		slog.Warn("could not determine dcx config directory, skipping terminfo forwarding", "error", err)
		return TerminfoResult{}
	}
	terminfoDir := filepath.Join(cacheDir, "terminfo")
	if err := os.MkdirAll(terminfoDir, 0o755); err != nil {
		slog.Warn("could not create terminfo cache directory, skipping terminfo forwarding", "path", terminfoDir, "error", err)
		return TerminfoResult{}
	}
	srcPath := filepath.Join(terminfoDir, term+".src")
	if err := os.WriteFile(srcPath, src, 0o644); err != nil {
		slog.Warn("failed to write terminfo source, skipping terminfo forwarding", "path", srcPath, "error", err)
		return TerminfoResult{}
	}

	compileDest := filepath.Join(containerHomeDir, ".terminfo")

	// Build a postCreateCommand that creates the target directory and compiles
	// the source entry with tic. We guard against missing tic and suppress its
	// exit code so that a container without ncurses-bin never fails to start.
	// The devcontainer CLI wraps string postCreateCommands in sh -c, so we
	// do not include the shell invocation here.
	postCmd := fmt.Sprintf("mkdir -p %s && if command -v tic >/dev/null 2>&1; then tic -x -o %s %s || true; fi", compileDest, compileDest, "/opt/dcx/terminfo.src")

	return TerminfoResult{
		Mount: &mounts.ResolvedMount{
			Source:   srcPath,
			Target:   "/opt/dcx/terminfo.src",
			ReadOnly: true,
		},
		PostCreateCommand: postCmd,
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
