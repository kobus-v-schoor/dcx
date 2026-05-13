# AGENTS.md

## Project

`dcx` is a Go CLI that wraps the `devcontainer` CLI, adding user-level persistence and workflow automation. It delegates all container lifecycle to the official CLI via its documented flags — it never calls internal APIs or modifies `devcontainer.json` on disk.

Full specification: `PLAN.md`

## Language & Build

- Go. Single static binary named `dcx`.
- `go build ./cmd/dcx` to build.
- `go test ./... -race` to run tests.
- `go vet ./...` to vet.
- No code generation. No CGO (`CGO_ENABLED=0` for release builds).

## Architecture

- `cmd/dcx/` — entry point
- `internal/config/` — user + project config loading, merge logic
- `internal/cli/` — Cobra command definitions
- `internal/features/` — default features → `--additional-features` JSON
- `internal/mounts/` — bind mount generation
- `internal/env/` — env var passthrough
- `internal/ssh/` — SSH/git auto-detection
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

1. User config: `~/.config/dcx/config.yaml`
2. Project config: `.devcontainer/dcx.yaml`
3. Environment variables: `DCX_*`
4. CLI flags

Project config does not override user-level preferences (features, shell, secrets are personal). Only `compose_integration` is project-level.

## Security Rules

- Never log secret values, even with `--verbose`. Log by name only.
- Never cache secrets on disk.
- GitHub token provider creates ephemeral fine-grained PATs (scoped to current repo, revoked on `dcx down`).
- All bind mounts under `/opt/dcx/` to avoid conflicts with container-installed software.

## CI

- PRs trigger lint + test workflow (`.github/workflows/ci.yml`).
- Tag push (`v*`) triggers release workflow — builds 4 platform binaries, creates GitHub release with changelog.
