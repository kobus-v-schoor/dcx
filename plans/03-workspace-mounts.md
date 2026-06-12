# Part 3: Workspace Mount Resolution

## Goal

Implement standalone resolution of `workspaceMount` and `workspaceFolder` so that later container-creation logic knows exactly how the project code is bound into the container without relying on the `devcontainer` CLI defaults.

## Architectural Approach

- Add to `internal/devcontainer/spec/`:
  - `ResolveWorkspaceMount(cfg *Config, hostWorkspaceFolder string) (string, error)`
  - `ResolveWorkspaceFolder(cfg *Config, hostWorkspaceFolder string) string`
- Rules (matching the devcontainer spec):
  - `workspaceFolder` defaults to the host workspace folder absolute path if absent or empty.
  - `workspaceMount` defaults to `type=bind,source=<hostWorkspaceFolder>,target=<workspaceFolder>,consistency=cached` (platform adjusts: `consistency` is macOS, Linux omits it).
  - If `workspaceMount` is present but `""`, treat as no workspace mount (advanced override).
  - If `workspaceMount` is a non-empty string, return it verbatim after validating it is a valid Docker bind mount string.
- Add a small validation helper `internal/devcontainer/spec/mount.go` that splits a mount string and ensures `source=` exists and points to an existing directory (warn, don't error, on missing source because it might be created later).
- Update `internal/override/override.go` to delegate workspace-folder extraction to `ResolveWorkspaceFolder` instead of manual raw-JSON inspection. Keep the override dir's `ContainerWorkspaceFolder` field so existing consumers are unaffected.

## Acceptance Criteria

- [ ] Unit tests cover default workspace mount, custom workspace mount, and empty workspace mount.
- [ ] On macOS the default mount string includes `consistency=cached`; on Linux it does not.
- [ ] `ResolveWorkspaceFolder` returns the container-side path when set in config, otherwise the host absolute path.
- [ ] No regression: `dcx up` on the `test/` project still works (still delegates to `devcontainer up` at this stage).
