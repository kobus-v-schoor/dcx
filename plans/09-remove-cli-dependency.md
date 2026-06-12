# Part 9: Remove Devcontainer CLI Dependency

## Goal

Complete the decoupling from the `devcontainer` CLI by deleting all references to the binary, removing prerequisite checks, and updating documentation. After this part, `dcx` is a fully standalone Go binary.

## Architectural Approach

- Delete `internal/runner/runner.go` and `internal/runner/runner_test.go`.
- Delete `internal/flags/flags.go` and `internal/flags/flags_test.go` (these packages only existed to assemble arguments for the `devcontainer` CLI).
- Update `internal/cli/root.go`:
  - Remove any `devcontainer` binary PATH checks from `PersistentPreRunE`.
  - Change root command short/long descriptions to remove "wraps devcontainer CLI" language.
- Update `internal/cli/up.go`:
  - Remove all fallback paths that call `runner.Run`.
  - Remove `--` passthrough flag documentation if it only applied to `devcontainer up` flags.
- Update `internal/cli/exec.go`:
  - Remove the `overrideConfigPath` special-case for missing `devcontainer.json` (the spec engine already handles this).
  - Remove any remaining `devcontainer exec` argument-building helpers.
- Update `README.md`:
  - Remove the `devcontainer CLI` prerequisite.
  - Update the "How It Works" and "Why not just use devcontainers directly?" sections to explain that `dcx` implements the devcontainer spec natively.
- Update `docs/cli.md`:
  - Remove references to "delegates to the devcontainer CLI".
- Update `AGENTS.md` if it mentions the `devcontainer` CLI dependency.
- Update `.github/workflows/ci.yml` if it installs the devcontainer CLI in the test environment; remove that installation step.
- Verify `go.mod`: remove any indirect dependencies that were only needed for flag assembly (unlikely, but check).
- Run `go test ./... -race`, `go vet ./...`, and `gofmt -l .` to ensure clean CI.

## Acceptance Criteria

- [ ] `grep -r "devcontainer" --include="*.go" /workspace/internal/` returns no matches to the external CLI binary (devcontainer.local_folder labels are allowed).
- [ ] Running `dcx up` on a fresh machine without the `devcontainer` CLI installed works end-to-end for image, Dockerfile, and compose projects.
- [ ] `dcx exec`, `dcx stop`, `dcx down`, and `dcx ps` all work without the `devcontainer` CLI.
- [ ] `README.md` and `docs/cli.md` do not list the devcontainer CLI as a prerequisite.
- [ ] CI passes lint, unit tests, and integration tests (tested against `test/` project).
- [ ] The binary size or dependency graph does not balloon significantly (features code should be lean; OCI download code should reuse stdlib/net/http where possible).
- [ ] A final commit or PR summary references issue #99 and notes all prior parts that were merged.
