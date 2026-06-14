# Part 4: Image Building (Base Image or Dockerfile)

## Goal

Re-implement the `devcontainer` CLI's image-building logic so `dcx` can pull a named image or build one from a Dockerfile without delegating to the external binary.

## Architectural Approach

- Extend `internal/docker/docker.go`:
  - Add `ImagePull(ctx, imageRef string) error` that pulls an image and streams progress to `slog.Debug`.
  - Add `ImageBuild(ctx, buildContextDir string, dockerfile string, buildArgs map[string]string, target string) (imageID string, err error)` that builds an image using the Docker SDK `ImageBuild` API and returns the resulting image ID or a predictable tag.
- Create `internal/devcontainer/image.go`:
  - `BuildImage(cfg *Config, workspaceFolder string) (imageRef string, err error)`
  - Algorithm:
    1. If `cfg.Image` is non-empty: `docker.ImagePull` and return the resolved digest/tag.
    2. If `cfg.Build` is present or `cfg.Dockerfile` is present:
       - Determine context path: `cfg.Build.Context` or `workspaceFolder`.
       - Dockerfile path: `cfg.Build.Dockerfile` or `cfg.Dockerfile` or `"Dockerfile"`.
       - Call `docker.ImageBuild` with args and target.
       - Tag the resulting image with a stable name so rebuilds are cache-friendly, e.g. `dcx-<workspace-name>:<hash-of-dockerfile-and-args>`.
    3. If neither image nor build is specified, return an error.
- Update the `DockerClient` interface with the necessary methods (`ImageBuild` proxy is tricky because the moby client returns a reader and we may not have that exact type in the docker/go-sdk abstraction). In the plan we note: if the docker/go-sdk does not expose `ImageBuild` directly, add a thin wrapper that uses the underlying moby client via type assertion, or contribute the missing wrapper locally.
- Keep image cleanup in `docker.Down` intact; it should now also match the stable dcx tag.

## Acceptance Criteria

- [ ] `dcx up` on a project with only `"image": "mcr.microsoft.com/devcontainers/base:debian"` pulls the image and starts the container using our native path (even if container creation itself is not yet native, the image-building helper is unit-testable).
- [ ] `dcx up` on a project with a `build` block builds the Dockerfile and tags it predictably.
- [ ] Re-running `dcx up` on an image-based project reuses the already-pulled image (no redundant pull).
- [ ] Re-running `dcx up` on a `build` project with no Dockerfile changes reuses the cached image layer.
- [ ] Unit tests mock `DockerClient` and verify the correct pull/build parameters.
- [ ] Doc comments explain the stable tagging scheme.
