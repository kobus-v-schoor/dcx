# Git Integration

dcx automatically forwards your git configuration from the host into the devcontainer, so commits, pushes, and other git operations work seamlessly inside the container.

## How It Works

When git config forwarding is enabled (the default), dcx:

1. Reads the list of git config files from `git.configs` (default: `~/.gitconfig`)
2. For each file that exists on the host, bind-mounts it into the container at `/opt/dcx/git/<number>-<basename>` (read-only)
3. Sets `GIT_CONFIG_GLOBAL` to the first mounted file's container path
4. Configures git to trust the workspace directory (preventing "dubious ownership" errors)

Your original git config files on the host are not modified.

## Configuration

```yaml
# ~/.config/dcx/config.yaml
git:
  inject_configs: true              # default: true
  configs:                          # default: [~/.gitconfig]
    - ~/.gitconfig
    - ~/.config/git/ignore
  mount_base: /opt/dcx/git          # default: /opt/dcx/git
```

### `inject_configs`

Set to `false` to disable git config forwarding entirely. When disabled, the container will use whatever git config is built into the container image.

### `configs`

List of git config files to forward. Supports `~` for home directory expansion. Missing files are skipped with a warning.

The **first file in the list that exists** on the host gets `GIT_CONFIG_GLOBAL` set to its container path - regardless of its filename. This means you can list multiple files and dcx will use whichever one is available.

```yaml
git:
  configs:
    - ~/.gitconfig           # primary config
    - ~/.config/git/config   # alternative XDG location
    - ~/.config/git/ignore   # global gitignore
```

### `mount_base`

Container directory where git config files are mounted. Each file is mounted as `<mount_base>/<index>-<basename>`. The index prefix prevents collisions when multiple files share the same basename.

## Safe Directory

When git config injection is enabled, dcx automatically configures git to **trust the workspace directory** inside the container. This prevents the "detected dubious ownership" error that occurs when the container's filesystem UID doesn't match the host's.

This is done via the `GIT_CONFIG_COUNT`, `GIT_CONFIG_KEY_0`, and `GIT_CONFIG_VALUE_0` environment variables (supported by git 2.31+), setting:

```
safe.directory = <container workspace folder>
```

This is injected automatically - no configuration needed.

## What Gets Forwarded

Your `~/.gitconfig` typically contains:

| Setting | Example | Forwarded? |
|---------|---------|-----------|
| `user.name` | `user.name=Jane Doe` | ✅ Yes |
| `user.email` | `user.email=jane@example.com` | ✅ Yes |
| `core.editor` | `core.editor=vim` | ✅ Yes |
| `alias.*` | Custom git aliases | ✅ Yes |
| `credential.*` | Credential helpers | ⚠️ Forwarded, but may not work (paths differ) |
| `includeIf.*` | Conditional includes | ⚠️ Forwarded, but paths may not exist in container |

### Credential helpers

If your `~/.gitconfig` references a credential helper (e.g. `osxkeychain`, `store`), the path may not be valid inside the container. Consider using the **GitHub proxy** instead - it injects your token at the network layer and works with any git remote over HTTPS:

```yaml
proxy:
  github:
    enabled: true
```

See [GitHub Proxy](proxy/github.md) for details.
