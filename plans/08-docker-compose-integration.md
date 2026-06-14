# Part 8: Docker Compose Integration

## Goal

Re-implement the devcontainer CLI's Docker Compose path so that `dcx up` works for projects specifying `dockerComposeFile` + `service`. This is one of the critical parts listed in the issue.

## Architectural Approach

- Create `internal/devcontainer/compose.go`:
  - `UpCompose(ctx, cfg *Config, spec *spec.Config, rebuild bool) (containerID string, err error)`
  - Steps:
    1. Resolve the compose file path(s). Support both single string and array of strings (merged in sequence).
    2. Determine the compose project name. Prefer `spec.Name`, else derive from workspace folder directory name.
    3. Bring up the compose project by invoking `docker compose up -d <service>` on the CLI. A pure-Go implementation is out of scope; the Docker Compose CLI is the source of truth for networking, volume creation, `depends_on`, and service profiles.
    4. After the service container is created, resolve its ID.
    5. Apply devcontainer-specific augmentations to the service container that the compose spec does not handle natively:
       - Generate a temporary compose override file (`dcx.compose.override.yml`) that injects:
         - The `devcontainer.local_folder` label and other `devcontainer.*` / `dcx.managed` labels.
         - `containerEnv` environment variables.
         - Additional bind mounts from `dcx.yaml` and auto-detected mounts (SSH agent, git config, terminfo).
       - Ensure the workspace mount is correct (compose may already declare it; keep the project's definition).
       - Run `docker compose -f <original> -f dcx.compose.override.yml up -d --force-recreate <service>` so Compose recreates the container with the augmentations without modifying the user's original compose file on disk.
       - After the container is created, verify via `docker inspect` that all augmentations are present.
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
