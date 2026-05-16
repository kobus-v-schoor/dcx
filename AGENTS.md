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

- Image: `mcr.microsoft.com/devcontainers/go:1` (floating tag, resolves to latest Go 1.x).
- `devcontainer up --workspace-folder .` to start.
- `devcontainer exec --workspace-folder . bash -c "cd /workspace && ..."` to run commands inside.
- Requires `-buildvcs=false` when building inside the devcontainer (VCS mount issue).
- To stop and clean up a devcontainer:
  ```
  docker ps --filter "label=devcontainer.local_folder=$(pwd)" --quiet | xargs -r -I {} sh -c '
      docker stop {} &&
      docker rm {} &&
      IMAGE_ID=$(docker inspect --format "{{.Image}}" {}) &&
      [ -n "$IMAGE_ID" ] && docker rmi "$IMAGE_ID" || echo "No image found for container {}"
  '
  ```

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

In order of precedence, low to high:

1. User config: `~/.config/dcx/config.yaml`
2. Project config: `.devcontainer/dcx.yaml`
3. Environment variables: `DCX_*`
4. CLI flags

Higher precedence config should overide and be merged with lower precedence
config.

## Security Rules

- Never log secret values, even with `--verbose`. Log by name only.
- Never cache secrets on disk.
- GitHub token provider creates ephemeral fine-grained PATs (scoped to current repo, revoked on `dcx down`).
- All bind mounts under `/opt/dcx/` to avoid conflicts with container-installed software.

## Documentation

- Every exported and unexported function must have a doc comment stating its what it operates on and usage (when/why it's called).
- Types should have doc comments explaining their purpose and any non-obvious design decisions (e.g., pointer fields to distinguish "not set" from zero values).
- When you implement new features or change behaviour, ensure that the docs in `docs` are still up-to-date

## CI

- PRs trigger lint + test workflow (`.github/workflows/ci.yml`).
- Tag push (`v*`) triggers release workflow — builds 4 platform binaries, creates GitHub release with changelog.
