# CLI Guide

This guide covers the basic usage of each dcx command.

## Global Flags

These flags apply to all commands (run `dcx --help` for an up-to-date list of flags):

| Flag | Description |
|------|-------------|
| `--workspace-folder` | Path to the workspace folder (default: `.`) |
| `--log-level` | Log verbosity: `debug`, `info`, `warn`, `error` |
| `--version` | Print the dcx version |
---

## `dcx up`

Start a devcontainer using dcx configuration. This delegates to the `devcontainer` CLI with dcx-assembled flags.

```bash
# Start the devcontainer in the current directory
dcx up

# Start with verbose logging
dcx up --log-level info

# Start a devcontainer in a different workspace
dcx up --workspace-folder /path/to/project

# Rebuild the container (picks up config changes)
dcx up --rebuild

# Pass extra flags to devcontainer up
dcx up -- --skip-post-create
```

### What happens when you run `dcx up`

1. dcx loads and merges your user + project config
2. It reads your `.devcontainer/devcontainer.json` and creates an **override** in a temp directory
3. It resolves SSH agent, git configs, features, mounts, and env vars
4. It injects everything into the override config
5. It calls `devcontainer up` with the assembled flags

Your original `devcontainer.json` is **not modified**.

### `--rebuild`

Use `--rebuild` when you've changed dcx configuration (env vars, mounts, features) and need the container recreated so the changes take effect. Without it, the devcontainer CLI will reuse the existing container.

---

## `dcx exec`

Open an interactive shell or execute a command inside the running devcontainer. When proxy services are enabled in the config, dcx starts them before opening the shell.

```bash
# Open an interactive bash shell
dcx exec

# Run a specific command
dcx exec -- make test

# Run with proxy logging
dcx exec --log-level debug
```

### Auto-start

If the devcontainer is not running, `dcx exec` will automatically run `dcx up` to start it first. This means `dcx exec` works as a single command to both start and connect to your devcontainer.

### Proxy services

When you run `dcx exec`, dcx:

1. Detects enabled proxy providers (e.g. GitHub)
2. Starts a transparent MITM proxy on the host
3. Injects the proxy's CA certificate into the container's trust store
4. Sets `HTTP_PROXY` / `HTTPS_PROXY` in the container to route traffic through the proxy
5. Sets provider-specific env vars (e.g. `GH_TOKEN=dummy` so the `gh` CLI makes API requests)

When you exit the shell, dcx stops the proxy and removes the CA certificate from the container.

---

## `dcx stop`

Stop the devcontainer for the current project **without removing it**. The container can be restarted later.

```bash
dcx stop
```

---

## `dcx down`

Stop, remove, and clean up the devcontainer for the current project. This also attempts to remove the associated container image.

```bash
dcx down
```

Image removal is best-effort - if another container still references the image, Docker will refuse and dcx logs a debug message.

---

## Common Workflows

### First-time setup

```bash
# 1. Create your user config
mkdir -p ~/.config/dcx
cat > ~/.config/dcx/config.yaml <<EOF
ssh:
  forward_agent: true
git:
  inject_configs: true
proxy:
  github:
    enabled: true
EOF

# 2. Navigate to your project
cd my-project

# 3. Start and connect
dcx exec
```

### Day-to-day development

```bash
# Start your container
dcx up

# Connect to it
dcx exec

# If you made dcx config changes, then rebuild
dcx up --rebuild

# Done for the day
dcx stop
```

### Using a project without devcontainer.json

If a project doesn't have a `.devcontainer/devcontainer.json`, dcx will use the `default_image` config option to create a minimal container:

```yaml
# ~/.config/dcx/config.yaml
default_image: mcr.microsoft.com/devcontainers/base:debian
```

Then simply run `dcx up` - dcx generates a temporary spec on the fly.
