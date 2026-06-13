# Part 1: devcontainer.json Config Engine

## Goal

Replace the ad-hoc `map[string]json.RawMessage` parsing of `devcontainer.json` with a strongly-typed Go representation of the devcontainer spec. This engine must be able to load the project `devcontainer.json`, overlay the temporary override config, and resolve defaults so that downstream logic (image building, container creation, exec) can read properties directly from structs rather than repeatedly unmarshalling raw JSON.

## Architectural Approach

- Create `internal/devcontainer/spec/types.go` containing Go structs for the standard devcontainer properties that `dcx` cares about:
  - `name`
  - `image`
  - `build` (`dockerfile`, `context`, `args`, `target`)
  - `dockerfile` (legacy string)
  - `dockerComposeFile` (`string` or `[]string`)
  - `service`
  - `runServices`
  - `workspaceFolder`
  - `workspaceMount`
  - `remoteUser`, `user`
  - `containerEnv`, `remoteEnv`
  - `mounts`
  - `features`
  - `postCreateCommand`, `postStartCommand`, `postAttachCommand`, `initializeCommand`
  - `runArgs`
  - `shutdownAction`
  - `overrideCommand`, `updateRemoteUserUID`
  - `forwardPorts`, `portsAttributes`
  - Use `json.RawMessage` or pointer types for rare/optional fields so absent fields are distinguishable from zero values.
- Create `internal/devcontainer/spec/merge.go` with a `Merge(base, override *Config) *Config` function that performs a deep merge. Override values win on conflict. Arrays should generally be replaced, not merged, unless the spec semantics demand otherwise (e.g. `mounts` might be concatenated—document the choice).
- Create `internal/devcontainer/spec/parse.go` with `Load(workspaceFolder string, overrideDir string) (*Config, error)`:
  1. Read `.devcontainer/devcontainer.json`.
  2. If missing and `defaultImage` is set, generate a minimal `Config{Image: defaultImage}`.
  3. Parse into `Config`.
  4. If `overrideDir/devcontainer.json` exists, parse it as a second `Config` and merge on top.
  5. Resolve defaults (e.g. `workspaceFolder` → `workspaceFolder` if empty).
- Convert `internal/override.OverrideDir` so that its `Inject*` helpers operate on a spec `*Config` instead of a raw map. This unifies the production and test paths.
- Update `internal/override/override_test.go` to assert against the new typed fields rather than raw JSON.

## Acceptance Criteria

- [ ] `go test ./internal/devcontainer/spec/...` passes with >80 % coverage for merge and default-resolution logic.
- [ ] All existing `override` tests continue to pass after migrating to typed config.
- [ ] `dcx up` on the `test/` project still works (no regression) because `flags.Build` and `runner.Run` are still used at this stage.
- [ ] The parser correctly handles both a single `dockerComposeFile` string and an array of strings.
- [ ] The parser correctly handles both `build` object and legacy top-level `dockerfile` string.
- [ ] Doc comments on every exported/unexported type and function explain the spec field and how `dcx` uses it.
