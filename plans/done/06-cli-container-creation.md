# Part 6: Migrate Non-Compose Container Creation to Docker CLI

## Goal

Replace the Moby API-based container creation from Part 5 with `docker create` / `docker start` invocations. This makes `runArgs` pass-through trivial and removes the need to map Docker CLI flags into Moby API structs.

## Background

Part 5 implemented non-Compose `dcx up` using `docker.ContainerCreate` and `docker.ContainerStart`. While functional, it requires parsing `runArgs` (which are specified in `devcontainer.json` as Docker CLI strings) into Moby `container.Config` / `container.HostConfig` structs. This mapping is brittle: every new `runArgs` flag (`--publish`, `--network`, `--cap-add`, `--device`, etc.) must be explicitly handled in Go code. By switching to the Docker CLI, `runArgs` can be appended verbatim.

## Architectural Approach

- Create `internal/docker/create.go`:
  - `ContainerCreateCLI(ctx, imageRef string, runArgs, mounts, envs []string, labels map[string]string, user, workdir, entrypoint string) (containerID string, err error)`
  - Constructs a `docker create` command with:
    - `--label key=val` for each label
    - `--mount type=...,source=...,target=...` for each mount string
    - `--env KEY=VAL` for each environment variable
    - `--user`, `--workdir`, `--entrypoint` when present
    - `runArgs` appended verbatim
    - the image ref as the final argument
  - Executes the command, captures stdout for the container ID, and returns it.
  - `ContainerStartCLI(ctx, containerID string) error` wraps `docker start <id>`.
- Refactor `internal/devcontainer/up.go`:
  - Remove `buildContainerConfig` and `buildHostConfig`.
  - Remove `ParsedRunArgs` and the `runargs.go` parser that maps CLI flags to Moby API structs. `runArgs` from the spec are appended verbatim to the `docker create` command line.
  - Resolve mounts into the existing string format (already supported by `internal/mounts/`) and pass as `--mount` flags.
  - Resolve workspace mount, `containerEnv`, labels, user, workdir, entrypoint, and `overrideCommand` logic into flat string slices / maps for the CLI helper.
- Preserve existing logic that already works well via the Moby library:
  - Discovery via `docker.FindDevcontainers` (label filter).
  - Stale mount checking via `docker.CheckStaleMounts`.
  - Stop / remove on rebuild via existing `docker.Stop` + `docker.Remove` calls.
- Error handling:
  - Parse `docker create` stdout for the container ID.
  - On failure, return CLI stderr so the user sees the exact Docker error.

## Acceptance Criteria

- [ ] `dcx up` on a non-compose image project successfully creates and starts the container via `docker create` / `docker start`.
- [ ] `runArgs` containing `--publish`, `--network`, `--cap-add`, etc. are passed through to `docker create` without any intermediate struct mapping.
- [ ] `dcx up --rebuild` still stops, removes, and recreates the container.
- [ ] All mounts (workspace, SSH agent socket, git config, terminfo) are present inside the created container.
- [ ] `containerEnv` variables are visible via `env` inside the container.
- [ ] The container carries the correct `devcontainer.local_folder`, `devcontainer.config_file`, `devcontainer.metadata`, and `dcx.managed` labels.
- [ ] `dcx exec` connects to the CLI-created container.
- [ ] `dcx stop` and `dcx down` still work (they already use the Moby API).
- [ ] Unit tests verify the constructed `docker create` flag list rather than Moby struct fields.
- [ ] `internal/devcontainer/runargs.go` and `ParsedRunArgs` are deleted.
