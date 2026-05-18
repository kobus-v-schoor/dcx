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
- **Never run `go` commands on the host.** All `go` commands (build, test, vet, get, etc.) must be run inside the devcontainer.
- `go build ./cmd/dcx` to build.
- `go test ./... -race` to run tests.
- `go vet ./...` to vet.
- No code generation. No CGO (`CGO_ENABLED=0` for release builds).

## Devcontainer

- Custom Dockerfile based on `mcr.microsoft.com/devcontainers/go:1` with the `devcontainer` CLI pre-installed (via upstream install script, binary at `/usr/local/bin/devcontainer`).
- Docker-in-Docker feature (`ghcr.io/devcontainers/features/docker-in-docker:2`, `moby: false`) enables running nested containers inside the devcontainer — used for integration testing.
- `devcontainer up` to start.
- `devcontainer exec ...` to run commands inside.
- Requires `-buildvcs=false` when building inside the devcontainer (VCS mount issue).

## Integration Testing

- The devcontainer has Docker-in-Docker and the `devcontainer` CLI, so full end-to-end integration tests can run **inside** the devcontainer.
- A test devcontainer is provided at `test/.devcontainer/` (simple `mcr.microsoft.com/devcontainers/base:debian` image) for use as the target container in integration tests.
- **Whenever changes are made to the codebase**, build `dcx` inside the devcontainer and run basic integration testing (e.g. `dcx up` against `test/`, verify the container starts, then `dcx down`) to catch regressions before committing.
- Integration tests should not be run on the host — only inside the devcontainer where Docker is available.

## Architecture

- `cmd/dcx/` — entry point
- `internal/config/` — user + project config loading, merge logic
- `internal/cli/` — Cobra command definitions
- `internal/docker/` — Docker client via docker/go-sdk (context-aware socket resolution, container stop/remove, image cleanup)
- `internal/features/` — default features → `--additional-features` JSON
- `internal/mounts/` — bind mount generation
- `internal/env/` — env var passthrough
- `internal/ssh/` — SSH agent auto-detection
- `internal/git/` — git config auto-detection
- `internal/shell/` — shell integration (mount configs, inject postCreateCommand)
- `internal/secrets/` — password manager providers (1Password, Bitwarden, GitHub token)
- `internal/compose/` — Docker Compose strategies (network join, overlay)
- `internal/init/` — project initialization (`dcx init`)

Key constraint: `dcx` communicates with `devcontainer` CLI only via flags (`--override-config`, `--additional-features`, `--mount`, `--remote-env`). Never modify the original `devcontainer.json` — write overrides to a temp dir and pass via `--override-config`.

## PR & Issue Workflow

- GitHub issues are the single source of truth for task tracking.
- Each PR references exactly one issue (e.g., `Fixes #3` in the PR body).
- Each PR implements only what its referenced issue specifies — no scope creep.
- PRs are squash-merged into `main`.
- PR title format: `[component] what the PR implements` (e.g., `[config] add env var support`).
- Implementation order follows issue numbering (#1 through #17).

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
- GitHub token provider creates ephemeral fine-grained PATs (scoped to current repo, revoked on `dcx down`).
- All bind mounts under `/opt/dcx/` to avoid conflicts with container-installed software.

## Documentation

- Every exported and unexported function must have a doc comment stating what it operates on and usage (when/why it's called).
- Types should have doc comments explaining their purpose and any non-obvious design decisions (e.g., pointer fields to distinguish "not set" from zero values).
- Function bodies should include inline comments for non-trivial blocks of logic (e.g. multi-step setup, sequential operations) so the intent of each step is clear without reading the surrounding context.
- When you implement new features or change behaviour, ensure that the docs in `docs` are still up-to-date

## CI

- PRs trigger lint + test workflow (`.github/workflows/ci.yml`).
- Tag push (`v*`) triggers release workflow — builds 4 platform binaries, creates GitHub release with changelog.
