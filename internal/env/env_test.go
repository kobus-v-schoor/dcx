package env

import (
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

func TestResolveValueWithSpecialCharacters(t *testing.T) {
	t.Setenv("SPECIAL_VAR", "value with spaces & $pecial chars!")

	ev := config.EnvVar("SPECIAL_VAR")
	resolved := Resolve(ev)

	if resolved == nil {
		t.Fatal("expected non-nil resolved env")
	}
	if resolved.Value != "value with spaces & $pecial chars!" {
		t.Errorf("Value = %q, want %q", resolved.Value, "value with spaces & $pecial chars!")
	}
}

func TestResolveValueWithEquals(t *testing.T) {
	t.Setenv("CONN_STR", "host=db user=admin password=secret")

	ev := config.EnvVar("DATABASE_URL=${CONN_STR}")
	resolved := Resolve(ev)

	if resolved == nil {
		t.Fatal("expected non-nil resolved env")
	}
	if resolved.Name != "DATABASE_URL" {
		t.Errorf("Name = %q, want %q", resolved.Name, "DATABASE_URL")
	}
	if resolved.Value != "host=db user=admin password=secret" {
		t.Errorf("Value = %q, want %q", resolved.Value, "host=db user=admin password=secret")
	}
}

func TestResolveValueWithNewlines(t *testing.T) {
	t.Setenv("MULTILINE_VAR", "line1\nline2\nline3")

	ev := config.EnvVar("MULTILINE_VAR")
	resolved := Resolve(ev)

	if resolved == nil {
		t.Fatal("expected non-nil resolved env")
	}
	if resolved.Value != "line1\nline2\nline3" {
		t.Errorf("Value = %q, want multiline value", resolved.Value)
	}
}

func TestResolveEmptyHostValue(t *testing.T) {
	t.Setenv("EMPTY_VAR", "")

	ev := config.EnvVar("EMPTY_VAR")
	resolved := Resolve(ev)

	if resolved == nil {
		t.Fatal("expected non-nil resolved env for empty but set variable")
	}
	if resolved.Value != "" {
		t.Errorf("Value = %q, want empty string", resolved.Value)
	}
}

func TestExpandValueLiteral(t *testing.T) {
	// A value with no ${...} references should be returned as-is.
	got := expandValue("hello world")
	if got != "hello world" {
		t.Errorf("expandValue(%q) = %q, want %q", "hello world", got, "hello world")
	}
}

func TestExpandValueSingleVar(t *testing.T) {
	t.Setenv("MY_VAR", "value")

	got := expandValue("${MY_VAR}")
	if got != "value" {
		t.Errorf("expandValue(%q) = %q, want %q", "${MY_VAR}", got, "value")
	}
}

func TestExpandValueComposite(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")

	got := expandValue("${PATH}:/opt/bin")
	if got != "/usr/bin:/opt/bin" {
		t.Errorf("expandValue(%q) = %q, want %q", "${PATH}:/opt/bin", got, "/usr/bin:/opt/bin")
	}
}

func TestExpandValueMultipleVars(t *testing.T) {
	t.Setenv("VAR_A", "alpha")
	t.Setenv("VAR_B", "beta")

	got := expandValue("${VAR_A}-${VAR_B}")
	if got != "alpha-beta" {
		t.Errorf("expandValue(%q) = %q, want %q", "${VAR_A}-${VAR_B}", got, "alpha-beta")
	}
}

func TestExpandValueUnsetVar(t *testing.T) {
	// Unset variable should be replaced with empty string (with warning logged).
	got := expandValue("${UNSET_EXPAND_VAR_12345}")
	if got != "" {
		t.Errorf("expandValue(%q) = %q, want empty string", "${UNSET_EXPAND_VAR_12345}", got)
	}
}

func TestExpandValueUnsetVarInComposite(t *testing.T) {
	t.Setenv("SET_VAR", "exists")

	got := expandValue("prefix/${SET_VAR}/${UNSET_EXPAND_VAR_99999}/suffix")
	if got != "prefix/exists//suffix" {
		t.Errorf("expandValue() = %q, want %q", got, "prefix/exists//suffix")
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

func TestResolveAllSingleSet(t *testing.T) {
	t.Setenv("MY_VAR", "hello")

	envVars := []config.EnvVar{"MY_VAR"}
	result := ResolveAll(envVars)

	if len(result) != 1 {
		t.Fatalf("expected 1 resolved env, got %d: %v", len(result), result)
	}
	if result[0].Name != "MY_VAR" {
		t.Errorf("Name = %q, want %q", result[0].Name, "MY_VAR")
	}
	if result[0].Value != "hello" {
		t.Errorf("Value = %q, want %q", result[0].Value, "hello")
	}
}

func TestResolveAllMultipleSet(t *testing.T) {
	t.Setenv("VAR_A", "alpha")
	t.Setenv("VAR_B", "beta")

	envVars := []config.EnvVar{"VAR_A", "VAR_B"}
	result := ResolveAll(envVars)

	if len(result) != 2 {
		t.Fatalf("expected 2 resolved envs, got %d: %v", len(result), result)
	}
	if result[0].Name != "VAR_A" || result[0].Value != "alpha" {
		t.Errorf("result[0] = {%q, %q}, want {VAR_A, alpha}", result[0].Name, result[0].Value)
	}
	if result[1].Name != "VAR_B" || result[1].Value != "beta" {
		t.Errorf("result[1] = {%q, %q}, want {VAR_B, beta}", result[1].Name, result[1].Value)
	}
}

func TestResolveAllMixedSetAndUnset(t *testing.T) {
	t.Setenv("SET_VAR", "value")

	envVars := []config.EnvVar{"SET_VAR", "UNSET_VAR_12345"}
	result := ResolveAll(envVars)

	// Both should appear: set var with its value, unset var with empty string.
	if len(result) != 2 {
		t.Fatalf("expected 2 resolved envs, got %d: %v", len(result), result)
	}
	if result[0].Name != "SET_VAR" || result[0].Value != "value" {
		t.Errorf("result[0] = {%q, %q}, want {SET_VAR, value}", result[0].Name, result[0].Value)
	}
	if result[1].Name != "UNSET_VAR_12345" || result[1].Value != "" {
		t.Errorf("result[1] = {%q, %q}, want {UNSET_VAR_12345, }", result[1].Name, result[1].Value)
	}
}

func TestResolveAllExplicitForm(t *testing.T) {
	t.Setenv("HOST_VAR", "secret")

	envVars := []config.EnvVar{"CONTAINER_VAR=${HOST_VAR}"}
	result := ResolveAll(envVars)

	if len(result) != 1 {
		t.Fatalf("expected 1 resolved env, got %d: %v", len(result), result)
	}
	if result[0].Name != "CONTAINER_VAR" || result[0].Value != "secret" {
		t.Errorf("result[0] = {%q, %q}, want {CONTAINER_VAR, secret}", result[0].Name, result[0].Value)
	}
}
