# Implementation Plan: Issue #99 — Re-implement devcontainer CLI functionality

This directory contains the ordered plan for removing `dcx`'s dependency on the external `devcontainer` CLI by re-implementing the required functionality directly in Go.

## Background

GitHub issue #99 requests that `dcx` stop relying on the `devcontainer` CLI binary because of supply-chain security concerns. The critical parts to re-implement are:

1. devcontainer image creation (base image or from dockerfile)
2. devcontainer features
3. docker-compose integration
4. post create commands
5. workspace mount resolution

## Plan Overview

The work is split into **twelve separately implementable parts**, ordered to minimize risk and maximize incremental value. Each part can be developed, reviewed, and merged as its own PR. The approach is: build libraries first, replace `exec` early for daily-workflow impact, then replace `up` piece by piece, tackle the hardest problem (features) once the simpler paths are stable, and clean up at the end.

## Architecture: Docker CLI vs Moby Library

`dcx` uses a **hybrid strategy** for talking to Docker. Commands that are complex to replicate correctly in Go invoke the Docker CLI; everything else uses the Moby client library (`docker/go-sdk`) directly.

### Use the Docker CLI for

- **Image builds** (`docker build`) — BuildKit, `.dockerignore`, and Dockerfile syntax are all handled natively. This avoids the SDK's "no active sessions" BuildKit failure and the need to manually stream and parse JSON build output.
- **Docker Compose** (`docker compose up` / `down`) — No stable pure-Go Compose library exists; the CLI handles networking, volume creation, `depends_on`, and service profiles correctly.
- **Container creation** (`docker create` / `docker run`) — `runArgs` in `devcontainer.json` are already Docker CLI arguments. Passing them through verbatim removes the need to map dozens of flags (`--publish`, `--network`, `--cap-add`, etc.) into Moby API structs.
- **Lifecycle exec** (`docker exec`) — Replaces the multi-step `ExecCreate` → `ExecStart` → `ExecInspect` + `stdcopy` demuxing dance with a single shell-out that handles TTY and streaming automatically.
- **File copy** (`docker cp`) — No manual tar archive creation is required.

### Use the Moby library for

- **Container discovery** (`ContainerList` with label filters) — Needed to obtain container IDs before any other operation.
- **Container inspection** (`ContainerInspect`) — Reading labels (e.g. `devcontainer.metadata`), checking stale bind mounts, and resolving gateway IPs all need structured container state.
- **Container stop / remove** — Simple, synchronous calls used by `dcx stop`, `dcx down`, and the rebuild flow.
- **Image pull / inspect / list / remove / tag** — Fast typed lookups used for caching and cleanup.
- **Volume removal** (`VolumeRemove`) — Used during `dcx down --volumes`.

## Tracking Progress

When a part is fully implemented, tested, and merged, **move its plan file into `plans/done/`**. This keeps the `plans/` directory uncluttered and makes it obvious which parts remain. Do not delete plan files—archive them in `plans/done/` so the implementation history is preserved.

## Part Order

| # | File | Focus | Key Deliverable |
|---|------|-------|-----------------|
| 1 | `01-config-engine.md` | Typed spec parser + merge | `internal/devcontainer/spec` |
| 2 | `02-exec-replacement.md` | Direct `docker exec` | `dcx exec` without `devcontainer` binary |
| 3 | `03-workspace-mounts.md` | `workspaceMount` / `workspaceFolder` resolution | Standalone mount resolver |
| 4 | `04-image-building.md` | Pull or build images | `internal/devcontainer/image.go` |
| 5 | `05-container-creation.md` | Native `dcx up` for non-compose projects (Moby API) | `internal/devcontainer/up.go` |
| 6 | `06-cli-container-creation.md` | Migrate non-Compose `dcx up` to Docker CLI | `internal/docker/create.go` |
| 7 | `07-post-create-commands.md` | `postCreateCommand` etc. | `internal/devcontainer/lifecycle.go` |
| 8 | `08-docker-compose-integration.md` | Native `dcx up` for compose projects | `internal/devcontainer/compose.go` |
| 9 | `09-features-support.md` | Download & install features | `internal/devcontainer/features/` |
| 10 | `10-compose-features-support.md` | Add features support to Docker Compose projects | Compose override with feature-augmented images |
| 11 | `11-remove-cli-dependency.md` | Delete dead code, update docs | Fully standalone `dcx` |
| 12 | `12-cli-image-builds.md` | Switch image builds from Moby SDK to Docker CLI | `internal/docker/build.go` |

## Dealing with the Test Project

The `test/` project uses **features** (`ghcr.io/devcontainers/features/github-cli:1`). Parts 1–8 should be tested against a *minimal* image-only `devcontainer.json` that does not use features. Once Part 9 (features) lands, the full `test/` project validates the complete stack.

## Backward Compatibility During the Transition

- Each part should land without breaking the existing `devcontainer` CLI delegation path.
- New packages are introduced alongside existing code; old wrappers (`runner`, `flags`) are deleted only in Part 11.
- No changes to the user's `.devcontainer/devcontainer.json` on disk are ever made (this is a core `dcx` rule).

## Acceptance Criteria for the Whole Effort

- `dcx` runs on a machine with **only Docker** installed; the `devcontainer` CLI is not required.
- All existing `dcx` commands (`up`, `exec`, `stop`, `down`, `ps`) behave the same or better for image, Dockerfile, and Docker Compose projects.
- The test project in `test/` continues to work.
- CI continues to pass lint (`go vet ./...`), tests (`go test ./... -race`), and formatting (`gofmt -l .`).
