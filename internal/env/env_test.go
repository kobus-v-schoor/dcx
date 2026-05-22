package env

import (
	"os"
	"testing"

	"github.com/kobus-v-schoor/dcx/internal/config"
)

func TestResolveShorthand(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")

	ev := config.EnvVar("AWS_ACCESS_KEY_ID")
	resolved := Resolve(ev)

	if resolved == nil {
		t.Fatal("expected non-nil resolved env")
	}
	if resolved.Name != "AWS_ACCESS_KEY_ID" {
		t.Errorf("Name = %q, want %q", resolved.Name, "AWS_ACCESS_KEY_ID")
	}
	if resolved.Value != "AKIAIOSFODNN7EXAMPLE" {
		t.Errorf("Value = %q, want %q", resolved.Value, "AKIAIOSFODNN7EXAMPLE")
	}
}

func TestResolveShorthandUnset(t *testing.T) {
	ev := config.EnvVar("UNSET_VAR_12345")
	resolved := Resolve(ev)

	if resolved == nil {
		t.Fatal("expected non-nil resolved env even for unset variable")
	}
	if resolved.Name != "UNSET_VAR_12345" {
		t.Errorf("Name = %q, want %q", resolved.Name, "UNSET_VAR_12345")
	}
	// Unset variable should resolve to empty string (with a warning logged).
	if resolved.Value != "" {
		t.Errorf("Value = %q, want empty string for unset variable", resolved.Value)
	}
}

func TestResolveExplicit(t *testing.T) {
	t.Setenv("AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")

	ev := config.EnvVar("SOMETHING_ELSE=${AWS_SECRET_ACCESS_KEY}")
	resolved := Resolve(ev)

	if resolved == nil {
		t.Fatal("expected non-nil resolved env")
	}
	if resolved.Name != "SOMETHING_ELSE" {
		t.Errorf("Name = %q, want %q", resolved.Name, "SOMETHING_ELSE")
	}
	if resolved.Value != "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY" {
		t.Errorf("Value = %q, want %q", resolved.Value, "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	}
}

func TestResolveExplicitUnsetHostVar(t *testing.T) {
	ev := config.EnvVar("MY_VAR=${UNSET_HOST_VAR_12345}")
	resolved := Resolve(ev)

	if resolved == nil {
		t.Fatal("expected non-nil resolved env even for unset host variable")
	}
	// Unset host variable should resolve to empty string (with a warning logged).
	if resolved.Value != "" {
		t.Errorf("Value = %q, want empty string for unset host variable", resolved.Value)
	}
}

func TestResolveExplicitSameName(t *testing.T) {
	t.Setenv("MY_VAR", "hello")

	ev := config.EnvVar("MY_VAR=${MY_VAR}")
	resolved := Resolve(ev)

	if resolved == nil {
		t.Fatal("expected non-nil resolved env")
	}
	if resolved.Name != "MY_VAR" {
		t.Errorf("Name = %q, want %q", resolved.Name, "MY_VAR")
	}
	if resolved.Value != "hello" {
		t.Errorf("Value = %q, want %q", resolved.Value, "hello")
	}
}

func TestResolveLiteralValue(t *testing.T) {
	// When the value part has no ${...} references, it's treated as a literal.
	ev := config.EnvVar("MY_FLAG=true")
	resolved := Resolve(ev)

	if resolved == nil {
		t.Fatal("expected non-nil resolved env")
	}
	if resolved.Name != "MY_FLAG" {
		t.Errorf("Name = %q, want %q", resolved.Name, "MY_FLAG")
	}
	if resolved.Value != "true" {
		t.Errorf("Value = %q, want %q", resolved.Value, "true")
	}
}

func TestResolveCompositeValue(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")

	ev := config.EnvVar("PATH=${PATH}:/opt/bin")
	resolved := Resolve(ev)

	if resolved == nil {
		t.Fatal("expected non-nil resolved env")
	}
	if resolved.Name != "PATH" {
		t.Errorf("Name = %q, want %q", resolved.Name, "PATH")
	}
	if resolved.Value != "/usr/bin:/opt/bin" {
		t.Errorf("Value = %q, want %q", resolved.Value, "/usr/bin:/opt/bin")
	}
}

func TestResolveAllEmpty(t *testing.T) {
	got := ResolveAll(nil)
	if got != nil {
		t.Errorf("ResolveAll(nil) = %v, want nil", got)
	}

	got = ResolveAll([]config.EnvVar{})
	if got != nil {
		t.Errorf("ResolveAll([]) = %v, want nil", got)
	}
}

func TestAutoForwardWithTerm(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("COLORTERM", "truecolor")

	result := AutoForward()

	if len(result) != 2 {
		t.Fatalf("expected 2 resolved envs, got %d", len(result))
	}
	if result[0].Name != "TERM" {
		t.Errorf("Name = %q, want %q", result[0].Name, "TERM")
	}
	if result[0].Value != "xterm-256color" {
		t.Errorf("Value = %q, want %q", result[0].Value, "xterm-256color")
	}
	if result[1].Name != "COLORTERM" {
		t.Errorf("Name = %q, want %q", result[1].Name, "COLORTERM")
	}
	if result[1].Value != "truecolor" {
		t.Errorf("Value = %q, want %q", result[1].Value, "truecolor")
	}
}

func TestAutoForwardWithoutTerm(t *testing.T) {
	// Ensure TERM and COLORTERM are unset — t.Setenv on a previously-set var
	// only overrides it within the test's scope.
	if err := os.Unsetenv("TERM"); err != nil {
		t.Fatalf("failed to unset TERM: %v", err)
	}
	if err := os.Unsetenv("COLORTERM"); err != nil {
		t.Fatalf("failed to unset COLORTERM: %v", err)
	}

	result := AutoForward()

	if len(result) != 0 {
		t.Fatalf("expected 0 resolved envs when TERM and COLORTERM are unset, got %d", len(result))
	}
}

func TestAutoForwardDoesNotWarnOnMissing(t *testing.T) {
	// Auto-forwarded variables that are unset should be silently skipped,
	// unlike user-configured vars which log a warning.
	if err := os.Unsetenv("TERM"); err != nil {
		t.Fatalf("failed to unset TERM: %v", err)
	}
	if err := os.Unsetenv("COLORTERM"); err != nil {
		t.Fatalf("failed to unset COLORTERM: %v", err)
	}

	result := AutoForward()
	if result != nil {
		t.Errorf("expected nil when all auto-forward vars are unset, got %v", result)
	}
}

func TestAutoForwardWithColortermOnly(t *testing.T) {
	if err := os.Unsetenv("TERM"); err != nil {
		t.Fatalf("failed to unset TERM: %v", err)
	}
	t.Setenv("COLORTERM", "truecolor")

	result := AutoForward()

	if len(result) != 1 {
		t.Fatalf("expected 1 resolved env, got %d", len(result))
	}
	if result[0].Name != "COLORTERM" {
		t.Errorf("Name = %q, want %q", result[0].Name, "COLORTERM")
	}
	if result[0].Value != "truecolor" {
		t.Errorf("Value = %q, want %q", result[0].Value, "truecolor")
	}
}

func TestBuildGitConfigEnvSingleEntry(t *testing.T) {
	entries := []GitConfigEntry{
		{Key: "safe.directory", Value: "/workspace"},
	}

	result := BuildGitConfigEnv(entries)

	if len(result) != 3 {
		t.Fatalf("expected 3 env vars, got %d", len(result))
	}

	found := make(map[string]string, len(result))
	for _, r := range result {
		found[r.Name] = r.Value
	}

	if found["GIT_CONFIG_COUNT"] != "1" {
		t.Errorf("GIT_CONFIG_COUNT = %q, want %q", found["GIT_CONFIG_COUNT"], "1")
	}
	if found["GIT_CONFIG_KEY_0"] != "safe.directory" {
		t.Errorf("GIT_CONFIG_KEY_0 = %q, want %q", found["GIT_CONFIG_KEY_0"], "safe.directory")
	}
	if found["GIT_CONFIG_VALUE_0"] != "/workspace" {
		t.Errorf("GIT_CONFIG_VALUE_0 = %q, want %q", found["GIT_CONFIG_VALUE_0"], "/workspace")
	}
}

func TestBuildGitConfigEnvMultipleEntries(t *testing.T) {
	entries := []GitConfigEntry{
		{Key: "safe.directory", Value: "/workspace"},
		{Key: "init.defaultBranch", Value: "main"},
	}

	result := BuildGitConfigEnv(entries)

	if len(result) != 5 {
		t.Fatalf("expected 5 env vars, got %d", len(result))
	}

	found := make(map[string]string, len(result))
	for _, r := range result {
		found[r.Name] = r.Value
	}

	if found["GIT_CONFIG_COUNT"] != "2" {
		t.Errorf("GIT_CONFIG_COUNT = %q, want %q", found["GIT_CONFIG_COUNT"], "2")
	}
	if found["GIT_CONFIG_KEY_0"] != "safe.directory" {
		t.Errorf("GIT_CONFIG_KEY_0 = %q, want %q", found["GIT_CONFIG_KEY_0"], "safe.directory")
	}
	if found["GIT_CONFIG_VALUE_0"] != "/workspace" {
		t.Errorf("GIT_CONFIG_VALUE_0 = %q, want %q", found["GIT_CONFIG_VALUE_0"], "/workspace")
	}
	if found["GIT_CONFIG_KEY_1"] != "init.defaultBranch" {
		t.Errorf("GIT_CONFIG_KEY_1 = %q, want %q", found["GIT_CONFIG_KEY_1"], "init.defaultBranch")
	}
	if found["GIT_CONFIG_VALUE_1"] != "main" {
		t.Errorf("GIT_CONFIG_VALUE_1 = %q, want %q", found["GIT_CONFIG_VALUE_1"], "main")
	}
}

func TestBuildGitConfigEnvEmpty(t *testing.T) {
	result := BuildGitConfigEnv(nil)
	if result != nil {
		t.Errorf("BuildGitConfigEnv(nil) = %v, want nil", result)
	}

	result = BuildGitConfigEnv([]GitConfigEntry{})
	if result != nil {
		t.Errorf("BuildGitConfigEnv([]) = %v, want nil", result)
	}
}

func TestResolveAllDelegatesToResolve(t *testing.T) {
	t.Setenv("MY_VAR", "hello")
	t.Setenv("OTHER_VAR", "world")

	result := ResolveAll([]config.EnvVar{"MY_VAR", "OTHER_VAR"})

	if len(result) != 2 {
		t.Fatalf("expected 2 resolved envs, got %d", len(result))
	}
	if result[0].Name != "MY_VAR" || result[0].Value != "hello" {
		t.Errorf("result[0] = {%q, %q}, want {MY_VAR, hello}", result[0].Name, result[0].Value)
	}
	if result[1].Name != "OTHER_VAR" || result[1].Value != "world" {
		t.Errorf("result[1] = {%q, %q}, want {OTHER_VAR, world}", result[1].Name, result[1].Value)
	}
}
