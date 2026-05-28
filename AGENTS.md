# AGENTS.md

## Project

`dcx` is a Go CLI that wraps the `devcontainer` CLI, adding user-level persistence and workflow automation. It delegates all container lifecycle to the official CLI via its documented flags — it never calls internal APIs or modifies `devcontainer.json` on disk.

The end-goal of the `dcx` project is to make secure development sandboxing so convenient that there's no friction to use it:

- Code/deps everything executes in a sandboxed environment
- Credentials are either
  - Generated and injected on the fly, to grant least-privilege access to the sandbox
  - Or, not provided to the sandbox all-together, if possible
- Convenient config mechanisms to avoid needing long cli args
- Integration with the existing devcontainer and docker-compose standards to leverage existing container setups

## Language & Build

- Go. Single static binary named `dcx`.
- `go build ./cmd/dcx` to build.
- `go test ./... -race` to run tests.
- `go vet ./...` to vet.
- No code generation. No CGO (`CGO_ENABLED=0` for release builds).

## Integration Testing

- A test devcontainer setup is provided at `test/.devcontainer/` (simple `mcr.microsoft.com/devcontainers/base:debian` image) for use as the target container in integration tests.
- **Whenever changes are made to the codebase**, run basic integration testing (e.g. `dcx up` against `test/`, verify the container starts, then `dcx down`) to catch regressions before committing.

## Architecture

- `cmd/dcx/` — entry point
- `internal/config/` — user + project config loading, merge logic
- `internal/cli/` — Cobra command definitions
- `internal/docker/` — Docker client via docker/go-sdk (context-aware socket resolution, container stop/remove, image cleanup)
- `internal/features/` — default features → `--additional-features` JSON
- `internal/mounts/` — bind mount generation
- `internal/env/` — env var passthrough
- `internal/ssh/` — SSH agent auto-detection (with VM runtime awareness for Colima)
- `internal/colima/` — Colima runtime detection and SSH agent socket resolution inside the Colima VM
- `internal/git/` — git config auto-detection
- `internal/shell/` — shell integration (mount configs, inject postCreateCommand)
- `internal/compose/` — Docker Compose strategies (network join, overlay)
- `internal/init/` — project initialization (`dcx init`)
- `internal/flags/` — devcontainer CLI flag assembly
- `internal/override/` — temporary override `devcontainer.json` generation
- `internal/proxy/` — transparent MITM proxy (GitHub, etc.) for credential injection. A single proxy intercepts HTTPS traffic to configured domains, decrypts it using an ephemeral CA certificate injected into the container's trust store, injects credentials, and re-encrypts traffic before forwarding.
- `internal/runner/` — devcontainer CLI execution wrapper

Key constraint: `dcx` communicates with `devcontainer` CLI only via flags (`--override-config`, `--additional-features`, `--mount`, `--remote-env`). Never modify the original `devcontainer.json` — write overrides to a temp dir and pass via `--override-config`.

## PR & Issue Workflow

- GitHub issues are the single source of truth for task tracking.
- Each PR should adress at most one issue (e.g., add `Fixes #3` in the PR body).
- Each PR implements only what its referenced issue specifies — no scope creep.
- PRs are squash-merged into `main`.
- PR title format: `[component] what the PR implements` (e.g., `[config] add env var support`).

## Config Loading Order

In order of precedence, low to high:

1. User config: `~/.config/dcx/config.yaml`
2. Project config: `.devcontainer/dcx.yaml`
3. Environment variables: `DCX_*`
4. CLI flags

Higher precedence config should override and be merged with lower precedence
config.

## Security Rules

- Never log secret values, even with `--verbose`. Log by name only.
- Never cache secrets on disk.
- Host credentials (e.g. GitHub tokens) are injected at the network layer by the proxy and are never exposed inside the container.
- All bind mounts under `/opt/dcx/` to avoid conflicts with container-installed software.

## Documentation

- Every exported and unexported function must have a doc comment stating what it operates on and usage (when/why it's called).
- Types should have doc comments explaining their purpose and any non-obvious design decisions (e.g., pointer fields to distinguish "not set" from zero values).
- Function bodies should include inline comments for non-trivial blocks of logic (e.g. multi-step setup, sequential operations) so the intent of each step is clear without reading the surrounding context.
- When you implement new features or change behaviour, ensure that the docs in `docs` are still up-to-date

## CI

- PRs trigger lint + test workflow (`.github/workflows/ci.yml`).
- Tag push (`v*`) triggers release workflow — builds 4 platform binaries, creates GitHub release with changelog.

## Coding conventions

- Develop with long-term maintainability in mind. Keep code DRY, interfaces
  simple and well-organised.
- When writing tests, write simple tests that only tests the core logic and
  operation of the code you want to test. Avoid overly thorough testing (lots
  of maintenance to keep up), don't test std-lib/external library functionality
  and keep tests as simple as possible while covering core functionality and
  edge-cases
