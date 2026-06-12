# Part 2: Replace `devcontainer exec` with Direct `docker exec`

## Goal

Remove `dcx exec`'s dependency on the `devcontainer` CLI binary by executing the user command directly inside the running container using the Docker Engine API. This is a high-value, low-risk early win because `dcx exec` is used many times per session and the Docker SDK already provides everything needed.

## Architectural Approach

- Extend `internal/docker/docker.go`:
  - Add `ContainerInspect` to the `DockerClient` interface if not already present (already present).
  - Add helper `ExecInteractive(ctx, cli, containerID, user, workdir, envVars, cmd)` that creates an exec with `AttachStdin`, `AttachStdout`, `AttachStderr`, and `Tty` when appropriate, then attaches the caller's stdio streams.
- Modify `internal/cli/exec.go`:
  - Delete the `runner.Find()` check.
  - Delete `buildExecArgs` and `runner.Run` call.
  - After finding `containerID` with `findContainerID`, read the merged devcontainer config (from Part 1) to obtain `remoteUser`, `workspaceFolder`, and `remoteEnv`.
  - If no config is available (no devcontainer.json and no default image), fall back to running as root with the host workspace folder as workdir.
  - Combine proxy `remoteEnv` with config `remoteEnv` (proxy values win on key conflict).
  - Call `docker.ExecInteractive(...)` with the resolved user, working directory, merged env vars, and the command/shell.
- Keep proxy setup unchanged—it already runs before exec and collects `remoteEnv`.
- Keep the auto-start logic (`ensureDevcontainerRunning`) unchanged; it still invokes `runUp` which still delegates to `devcontainer up` for now.

## Acceptance Criteria

- [ ] `dcx exec` opens an interactive shell inside a running devcontainer without the `devcontainer` binary on PATH.
- [ ] `dcx exec -- make test` runs a non-interactive command correctly.
- [ ] The shell/command runs as `remoteUser` when specified in `devcontainer.json`, otherwise as the image's default user.
- [ ] Environment variables from `remoteEnv` and active proxies are injected.
- [ ] Working directory is `workspaceFolder` from the config, falling back to the host workspace path.
- [ ] Stdin, stdout, stderr, and TTY are forwarded bidirectionally (fixing any terminal size issues is out of scope unless trivial).
- [ ] Existing proxy tests (`internal/proxy/proxy_test.go`) still pass.
