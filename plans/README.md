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

The work is split into **nine separately implementable parts**, ordered to minimize risk and maximize incremental value. Each part can be developed, reviewed, and merged as its own PR. The approach is: build libraries first, replace `exec` early for daily-workflow impact, then replace `up` piece by piece, tackle the hardest problem (features) once the simpler paths are stable, and clean up at the end.

## Tracking Progress

When a part is fully implemented, tested, and merged, **move its plan file into `plans/done/`**. This keeps the `plans/` directory uncluttered and makes it obvious which parts remain. Do not delete plan files—archive them in `plans/done/` so the implementation history is preserved.

## Part Order

| # | File | Focus | Key Deliverable |
|---|------|-------|-----------------|
| 1 | `01-config-engine.md` | Typed spec parser + merge | `internal/devcontainer/spec` |
| 2 | `02-exec-replacement.md` | Direct `docker exec` | `dcx exec` without `devcontainer` binary |
| 3 | `03-workspace-mounts.md` | `workspaceMount` / `workspaceFolder` resolution | Standalone mount resolver |
| 4 | `04-image-building.md` | Pull or build images | `internal/devcontainer/image.go` |
| 5 | `05-container-creation.md` | Native `dcx up` for non-compose projects | `internal/devcontainer/up.go` |
| 6 | `06-post-create-commands.md` | `postCreateCommand` etc. | `internal/devcontainer/lifecycle.go` |
| 7 | `07-docker-compose-integration.md` | Native `dcx up` for compose projects | `internal/devcontainer/compose.go` |
| 8 | `08-features-support.md` | Download & install features | `internal/devcontainer/features/` |
| 9 | `09-remove-cli-dependency.md` | Delete dead code, update docs | Fully standalone `dcx` |

## Dealing with the Test Project

The `test/` project uses **features** (`ghcr.io/devcontainers/features/github-cli:1`). Parts 1–7 should be tested against a *minimal* image-only `devcontainer.json` that does not use features. Once Part 8 (features) lands, the full `test/` project validates the complete stack.

## Backward Compatibility During the Transition

- Each part should land without breaking the existing `devcontainer` CLI delegation path.
- New packages are introduced alongside existing code; old wrappers (`runner`, `flags`) are deleted only in Part 9.
- No changes to the user's `.devcontainer/devcontainer.json` on disk are ever made (this is a core `dcx` rule).

## Acceptance Criteria for the Whole Effort

- `dcx` runs on a machine with **only Docker** installed; the `devcontainer` CLI is not required.
- All existing `dcx` commands (`up`, `exec`, `stop`, `down`, `ps`) behave the same or better for image, Dockerfile, and Docker Compose projects.
- The test project in `test/` continues to work.
- CI continues to pass lint (`go vet ./...`), tests (`go test ./... -race`), and formatting (`gofmt -l .`).
