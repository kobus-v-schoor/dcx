# Part 12: Switch Image Builds to Docker CLI

## Goal

Move all Docker image builds from the Moby SDK `ImageBuild` API to the Docker CLI (`docker build`), as originally described in the architecture section of `plans/README.md`. This aligns the implementation with the documented hybrid strategy and fixes the SDK's "no active sessions" BuildKit failure.

The two affected build paths are:

1. **Devcontainer Dockerfile builds** — `internal/devcontainer/image.go` (`buildFromDockerfile`).
2. **Feature-augmented image builds** — `internal/devcontainer/features/build.go` (`BuildFeatureImage`).

Both currently call `docker.ImageBuildFromDir`, which uses the Moby client directly.

## Architectural Approach

1. **Add a CLI image build helper**  
   Create `internal/docker/build.go` with:
   - A plain `ImageBuildOptions` struct holding `Tags`, `Dockerfile`, `Target`, `BuildArgs`, and `Labels`.
   - `ImageBuildFromDirCLI(ctx, buildContextDir, opts)` that constructs and runs `docker build` via `exec.CommandContext`.
   - Stdout and stderr are wired directly to `os.Stdout`/`os.Stderr` so build progress is visible in real time. No manual tar archive creation or JSON stream parsing is required.

2. **Delete the SDK-based build path**  
   Remove from `internal/docker/docker.go`:
   - `ImageBuildFromDir` (the old Moby wrapper).
   - `tarBuildContext` (manual tar creation).
   - `consumeBuildStream` (JSON stream parsing).
   - `ImageBuild` from the `DockerClient` interface.

3. **Update production callers**  
   - In `internal/devcontainer/image.go`, replace the `client.ImageBuildOptions` construction and `docker.ImageBuildFromDir` call with `docker.ImageBuildFromDirCLI`. Introduce a package-level variable (e.g. `imageBuildFromDirCLI`) so tests can substitute the builder without shelling out.
   - In `internal/devcontainer/features/build.go`, do the same: swap the `client.ImageBuildOptions` + `docker.ImageBuildFromDir` call for `docker.ImageBuildFromDirCLI`.

4. **Clean up imports**  
   Remove Moby SDK `client` and `build` imports from `image.go` and `features/build.go` where they are no longer needed.

5. **Update tests**  
   - Remove `ImageBuild` from all mock `DockerClient` implementations (at least `docker_test.go`, `image_test.go`, `up_test.go`, `compose_test.go`, `discovery_test.go`, `features/build_test.go`).
   - Rewrite `image_test.go` cases that previously verified `capturedBuildOpts` to instead verify the options passed to the overridable `imageBuildFromDirCLI` variable.

## Acceptance Criteria

- [ ] No production code calls `ImageBuild` on the Moby client.
- [ ] `ImageBuild` is removed from the `DockerClient` interface.
- [ ] `docker.ImageBuildFromDir`, `tarBuildContext`, and `consumeBuildStream` are deleted.
- [ ] Feature images and Dockerfile-based images are both built via the `docker build` CLI.
- [ ] Build output streams to the user's terminal in real time.
- [ ] `.dockerignore` is respected automatically (handled by the CLI).
- [ ] BuildKit works without the "no active sessions" error.
- [ ] Unit tests pass after updating mocks and overrides.
- [ ] `go test ./... -race`, `go vet ./...`, and `gofmt -l .` pass.
- [ ] The test project in `test/` continues to work for both image-based and Dockerfile-based configs.
