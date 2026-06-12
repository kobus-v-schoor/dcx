# Part 5: Container Creation (Non-Compose `dcx up`)

## Goal

Replace the `devcontainer up` delegation for non-Docker-Compose projects by directly creating and starting the container via the Docker Engine API. At this stage features are not yet supported (test with a simple image-only project).

## Architectural Approach

- Create `internal/devcontainer/up.go`:
  - `UpNative(ctx context.Context, cfg *config.Config, spec *spec.Config, imageRef string, rebuild bool) (containerID string, err error)`
  - Steps:
    1. Check for existing container by label `devcontainer.local_folder=<workspaceFolder>`.
    2. If exists, running, and not `rebuild`: return its ID (no-op).
    3. If exists and `rebuild` or stale mounts: stop and remove it (reusing existing `docker.Stop`/`docker.Down` logic).
    4. Resolve workspace mount and all user/config mounts into `[]mount.Mount` structs.
    5. Resolve `containerEnv` into `[]string` key=value pairs.
    6. Parse `runArgs` into Docker container config fields (e.g. `--publish`, `--network`, `--cap-add`). Start with a minimal whitelist: port forwarding and network mode. Document unsupported args.
    7. Build `container.Config` (Image, Env, User, Labels, WorkingDir) and `container.HostConfig` (Mounts, PortBindings, NetworkMode).
    8. Set labels: `devcontainer.local_folder=<absWorkspaceFolder>` and `dcx.managed=true`.
    9. Call `docker.ContainerCreate`, then `docker.ContainerStart`.
    10. Return the new container ID.
- Extend `internal/docker/docker.go`:
  - Add `ContainerCreate` and `ContainerStart` to `DockerClient`.
- Modify `internal/cli/up.go`:
  - After building the override config and resolving the merged spec, call `devcontainer.BuildImage` (Part 4) to get the image ref.
  - Then call `devcontainer.UpNative(...)` instead of `runner.Run(...)`.
  - Keep the `--rebuild` flag; pass it through.
- `ensureDevcontainerRunning` in `exec.go` must also use the new native path. Since `runUp` is the common helper, updating `runUp` covers both.
- Add a feature-flag environment variable `DCX_NATIVE_UP=1` for gradual roll-out during development, but remove it before the final PR.

## Acceptance Criteria

- [ ] `dcx up` on a non-compose project with a simple `image` property successfully creates and starts the container without the `devcontainer` binary.
- [ ] `dcx up --rebuild` removes and recreates the container.
- [ ] `dcx stop` and `dcx down` still find the container by the `devcontainer.local_folder` label.
- [ ] `dcx exec` can connect to the newly created container.
- [ ] Bind mounts from `dcx.yaml`, SSH agent, git config, and the workspace mount are all present inside the running container.
- [ ] `containerEnv` variables are visible inside the container via `env`.
- [ ] The container has the correct `remoteUser` (or image default if absent).
- [ ] Unit tests for `UpNative` mock `DockerClient` and assert correct create/start parameters.
