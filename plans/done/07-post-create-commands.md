# Part 7: Post-Create Commands

## Goal

Re-implement `postCreateCommand` (and optionally `postStartCommand`, `postAttachCommand`) execution so that `dcx` runs lifecycle commands after the container is created/started, matching the devcontainer spec semantics.

## Architectural Approach

- Create `internal/devcontainer/lifecycle.go`:
  - `RunPostCreate(ctx, cli, containerID string, spec *spec.Config) error`
  - `RunPostStart(ctx, cli, containerID string, spec *spec.Config) error`
  - `RunPostAttach(ctx, cli, containerID string, spec *spec.Config) error`
- command resolution rules (per the spec):
  - String form: `"echo hello && echo world"` → execute in a shell (`bash -c "..."`).
  - Array form: `["echo", "hello"]` → execute directly (no shell).
  - Object form (rare): `{"command": "...", "type": "shell|exec"}` → out of scope unless trivial; document.
- Implementation:
  - After the container is started in the non-Compose `dcx up` path, call `RunPostCreate`.
  - After confirming the container is running (e.g. on first `dcx exec`), optionally call `RunPostStart`.
  - `RunPostAttach` can be triggered inside `dcx exec` before the user's command runs, if we want full spec parity.
  - Invoke lifecycle commands via `docker exec` on the CLI. Stream stdout/stderr directly to the host terminal so the user sees build logs. For non-interactive commands, capture output and log the result.
  - Exit codes: a failing post-create command should be logged as a warning but not fail the entire `dcx up` (matching the devcontainer CLI's lenient default behaviour). Document this decision.
- Update `internal/override/override.go`:
  - Ensure that commands injected into the override config (e.g. terminfo compilation) are still executed. Since `UpNative` reads the merged spec rather than the raw override JSON, the override layer's `InjectPostCreateCommand` must write into the typed `spec.Config`.

## Acceptance Criteria

- [ ] A devcontainer config with `"postCreateCommand": "echo hello > /tmp/post-create.txt"` causes the file to be written after `dcx up`.
- [ ] A failing post-create command logs a clear warning but does not abort `dcx up`.
- [ ] Array-form commands are executed directly (verified by checking `/proc/<pid>/cmdline` or equivalent).
- [ ] The terminfo compilation command injected by `env.PrepareTerminfo` still executes and the resulting terminfo entry works in the container.
- [ ] `dcx up --skip-post-create` (or equivalent flag added later) still works if implemented; otherwise note as follow-up.
