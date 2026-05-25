# Contributing to dcx

Thanks for your interest in contributing! This document covers the workflow and conventions we use.

## Issues-first workflow

GitHub issues are the single source of truth for task tracking. **Each PR must reference exactly one issue** (e.g. `Fixes #3` in the PR body) and implement only what that issue specifies - no scope creep. PRs are squash-merged into `main`.

## PR title format

Use the format:

```
[component] what the PR implements
```

**Examples:**

- `[config] add env var support`
- `[docker] make Down idempotent when no container exists`
- `[proxy] use MITM approach to make proxying fully transparent`

The `component` is usually the top-level Go package under `internal/` that the PR touches (e.g. `config`, `cli`, `docker`, `proxy`, `ssh`). If a PR spans several packages, pick the most relevant one or use a broader term like `docs` or `readme`.

## PR description

- Include `Fixes #<issue-number>` in the body.
- Explain the **what** and **why** - not just the diff.
- If the PR includes behavioural changes, note any integration-testing steps you performed (see below).

## Development setup

### Prerequisites

- Go 1.25+
- [Docker](https://docs.docker.com/get-docker/)
- [devcontainer CLI](https://github.com/devcontainers/cli) on your `$PATH`

### Build

```bash
go build ./cmd/dcx
```

### Test

```bash
go test ./... -race   # unit tests
go vet ./...          # static analysis
```

CI also runs `gofmt -l .`, so format your code before pushing:

```bash
gofmt -w .
```

### Dog-food your changes with dcx itself

`dcx` can (and should!) be used to develop `dcx`. The repo includes a `.devcontainer/` setup with Go and the devcontainer CLI pre-installed.

```bash
# from the repo root, use dcx to start the dev container
dcx up

# drop into the container shell
dcx exec

# inside the container you can build and test as usual
go build ./cmd/dcx
go test ./... -race

# devcontainer/docker works inside the container as well
dcx up # creates another container inside the container
```


### Integration testing

A minimal test devcontainer setup lives under `test/.devcontainer/`. **Whenever you make changes, run a quick smoke test:**

```bash
# run dcx up against the test directory
dcx up --workspace-folder test/
# verify the container starts successfully

dcx exec --workspace-folder test/ -- echo "it works"

# tear it down
dcx down --workspace-folder test/
```

This catches regressions in container lifecycle handling, mount generation, and proxy setup that unit tests may miss.

## Code conventions

- **Comments:** Every exported and unexported function must have a doc comment. Types should have doc comments explaining their purpose and any non-obvious design decisions. Inline comments are expected for non-trivial multi-step logic.
- **No CGO:** Release builds are static (`CGO_ENABLED=0`). Do not introduce CGO dependencies.
- **No code generation:** Keep the build plain `go build`.
- **DRY:** Keep interfaces simple and well-organised. Prefer clarity over cleverness.
- **Tests:** Write simple tests that cover core logic and edge cases. Avoid exhaustive table-driven tests of std-lib or external-library behaviour. Do not test implementation details that are likely to change.

## Architecture constraints

A few hard rules to keep in mind:

- `dcx` communicates with the `devcontainer` CLI **only via flags** (`--override-config`, `--additional-features`, `--mount`, `--remote-env`).
- **Never modify the original `devcontainer.json`** on disk. Write temporary overrides to a temp directory and pass them with `--override-config`.
- All bind mounts that `dcx` injects live under `/opt/dcx/` to avoid conflicting with software installed inside the container.

## Security rules

- **Never log secret values**, even with `--verbose`. Log by secret name only.
- **Never cache secrets on disk.**
- Host credentials (e.g. GitHub tokens) are injected at the network layer by the proxy and must never be exposed inside the container.

## Documentation

If your PR changes behaviour or adds new features, update the relevant files in the [`docs/`](docs/) directory so they stay in sync with the code.

## Release process

You do not need to cut releases - maintainers handle that. Pushing a tag matching `v*` triggers the release workflow (`.github/workflows/release.yml`) which builds binaries for four platforms and publishes a GitHub release.

## Questions?

Open a [GitHub issue](https://github.com/kobus-v-schoor/dcx/issues) — we use issues for bug reports, feature requests, and discussion.
