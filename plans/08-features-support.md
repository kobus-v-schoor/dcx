# Part 8: Devcontainer Features Support

## Goal

Re-implement devcontainer features installation so that `dcx` can resolve, download, and apply features to the container image without relying on the `devcontainer` CLI's feature engine. This is the most complex critical part and is intentionally scheduled after the simpler `up` paths are stable.

## Architectural Approach

- Create `internal/devcontainer/features/` (note potential package name clash with existing `internal/features`; rename existing to `internal/dcxfeatures` or nest new code under `internal/devcontainer/features/` and alias as needed).
- Feature resolution:
  - Features are specified either in `devcontainer.json` (`"features": { "<id>": { ...opts } }`) or via dcx user config `default_features` (which currently serializes to `--additional-features`).
  - Merge project features and dcx `default_features`; dcx defaults should be appended (project features take precedence on option conflict—document the precedence).
- Feature download:
  - Features are distributed as OCI artifacts or GitHub release tarballs. Implement a downloader that:
    1. Parses the feature ID (e.g. `ghcr.io/devcontainers/features/github-cli:1`).
    2. If the registry is `ghcr.io` or another OCI registry, pull the manifest and layers using `oras` or the Docker registry HTTP API, then extract the tarball layer.
    3. Cache downloaded feature tarballs in a user-level cache directory (e.g. `~/.cache/dcx/features/`) so repeated builds are fast.
    4. Extract to a temp build context directory.
  - Implement `internal/devcontainer/features/download.go` with `Resolve(featureID string) (localPath string, err error)`.
- Feature installation:
  - Each feature contains an `install.sh` (or `install` script) and a `devcontainer-feature.json` describing its options and lifecycle.
  - Generate a Dockerfile on-the-fly:
    - `FROM <base-image-ref>` (the image built in Part 4).
    - For each feature in order: `COPY <feature-dir> /tmp/feature-<n>`, then `RUN cd /tmp/feature-<n> && ./install.sh` (with options injected as env vars or via the feature's option-handling convention).
    - Tag the final image with a stable name that encodes the base image digest + sorted feature IDs + hash of options.
  - Build this generated Dockerfile using `docker.ImageBuild` (Part 4).
- Modify `internal/devcontainer/image.go`:
  - `BuildImage` should accept a list of resolved features. If features are present, it invokes the feature builder to produce the final image instead of using the raw base image.
- Start with a whitelist of well-known registries (`ghcr.io`) and log a clear unsupported-feature-registry error for others, reducing scope.

## Acceptance Criteria

- [ ] `dcx up` on the `test/` project (which specifies `ghcr.io/devcontainers/features/github-cli:1`) successfully installs `gh` inside the container.
- [ ] Features from `default_features` in `~/.config/dcx/config.yaml` are also installed.
- [ ] Re-running `dcx up` without config changes reuses the cached final image (fast path).
- [ ] Adding a new feature or changing a feature option triggers a rebuild of the feature layer.
- [ ] The feature cache is stored under `~/.cache/dcx/features/` and respects `XDG_CACHE_HOME`.
- [ ] Unit tests for the downloader mock the registry HTTP responses. Integration tests verify `gh` (or a minimal test feature) is present in the container.
- [ ] Doc comments explain the OCI layer extraction and the generated Dockerfile structure.
