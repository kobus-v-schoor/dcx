# Part 7: Docker Compose Integration

## Goal

Re-implement the devcontainer CLI's Docker Compose path so that `dcx up` works for projects specifying `dockerComposeFile` + `service`. This is one of the critical parts listed in the issue.

## Architectural Approach

- Create `internal/devcontainer/compose.go`:
  - `UpCompose(ctx, cfg *Config, spec *spec.Config, rebuild bool) (containerID string, err error)`
  - Steps:
    1. Resolve the compose file path(s). Support both single string and array of strings (merged in sequence).
    2. Determine the compose project name. Prefer `spec.Name`, else derive from workspace folder directory name.
    3. Bring up the compose project. Use the Docker Compose Go library if available and stable, otherwise shell out to `docker compose up -d <service>` as a pragmatic intermediate step. The plan should note that a pure-Go implementation is preferred, but `docker compose` CLI is acceptable as long as `dcx` does not depend on the `devcontainer` CLI itself.
    4. After the service container is created, resolve its ID.
    5. Apply devcontainer-specific augmentations to the service container that the compose spec does not handle natively:
       - Add the `devcontainer.local_folder` label.
       - Ensure workspace mount is correct (compose may already declare it; inspect and patch if needed).
       - Inject `containerEnv` environment variables.
       - Apply additional bind mounts from `dcx.yaml` and auto-detected mounts (SSH, git, terminfo).
       - Because Docker Compose manages the container config, some changes require recreating the service container. Detect if any augmentations differ from the existing container; if so, call `compose up --force-recreate` for this service.
    6. Return the container ID.
- Reuse existing `internal/compose/` helpers:
  - `compose.FindProjectsAndVolumes` for discovery.
  - `compose.Stop`/`compose.Down` for teardown.
- Modify `internal/cli/up.go`:
  - Branch on whether `spec.DockerComposeFile` is present:
    - If yes → `devcontainer.UpCompose`
    - If no  → `devcontainer.UpNative`
- Keep `runDown` and `runStop` in `down.go`/`stop.go` unchanged; they already handle compose projects by discovering and tearing them down.

## Acceptance Criteria

- [ ] `dcx up` on a project with `dockerComposeFile` + `service` brings up the compose stack and the target service container.
- [ ] The target container has the `devcontainer.local_folder` label.
- [ ] `dcx exec` can open a shell in the compose-managed devcontainer service.
- [ ] `dcx down --volumes` tears down the compose project and removes named volumes.
- [ ] `dcx stop` stops the compose-managed devcontainer service and related services.
- [ ] Additional mounts from `dcx.yaml` are present in the target container.
- [ ] `containerEnv` variables are present in the target container.
