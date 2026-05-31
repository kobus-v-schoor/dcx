# Environment Passthrough

dcx can forward environment variables from the host into the devcontainer. This is useful for passing API keys, configuration flags, and other runtime values without hardcoding them into the container image or `devcontainer.json`.

## How It Works

Environment variables declared in the `environment` config are resolved on the host at `dcx up` time and injected into the override `devcontainer.json`'s `containerEnv` property. This makes them **Docker-level environment variables** - they are persistent in the running container and visible via `env`, `docker exec`, etc.

## Configuration

Two formats are supported:

### Shorthand: `"NAME"`

Reads the host env var `NAME` and sets `NAME` in the container with the same value.

```yaml
environment:
  - AWS_ACCESS_KEY_ID
  - AWS_SECRET_ACCESS_KEY
```

If the host variable is not set, a warning is logged and the value is set to an empty string.

### Explicit: `"CONTAINER_NAME=${HOST_VAR}"`

Reads `HOST_VAR` from the host environment and sets `CONTAINER_NAME` in the container.

```yaml
environment:
  - AWS_KEY=${AWS_ACCESS_KEY_ID}
```

### Composite expressions

The value part supports mixing substitutions and literal text:

```yaml
environment:
  - PATH=${PATH}:/opt/bin
  - NODE_PATH=${HOME}/.npm-global/lib
```

### Literal values

If the value contains no `${...}` references, it is treated as a plain literal string:

```yaml
environment:
  - MY_FLAG=true
  - RUST_BACKTRACE=full
```

## Auto-Forwarded Variables

Some environment variables are automatically forwarded without requiring configuration:

| Variable | Why |
|----------|-----|
| `TERM` | Ensures TUI applications work correctly inside the container |
| `COLORTERM` | Enables true-color support in terminal applications |
| `TERMINFO` | Forwards the host's terminfo database for terminal emulators not present in the container |

Auto-forwarded variables have the **lowest precedence** - they can be overridden by entries in your `environment` config.

When `TERMINFO` is forwarded, dcx also creates a read-only bind mount of the host terminfo directory to `/opt/dcx/terminfo` inside the container, and sets `TERMINFO` to point at that path. This ensures terminal emulators not present in the container's default terminfo database (e.g. `xterm-kitty`) continue to work correctly.

## Merge Behavior

Environment lists from user config and project config are **concatenated** - project entries appear after user entries. If both define the same container-side name, the **project entry wins** (it appears later and takes precedence).

## Security Considerations

> ⚠️ **Warning:** Environment variables passed through this mechanism are **visible inside the container**. They are stored in the `containerEnv` property of the override config and are accessible to any process running in the container.

### Prefer proxy-based credential injection

For sensitive credentials (GitHub tokens, API keys), **prefer using the proxy system** instead of environment variable passthrough. The proxy injects credentials at the network layer - they are never exposed inside the container.

```yaml
# ✅ Recommended: proxy-based injection (token never in container)
proxy:
  github:
    enabled: true

# ⚠️ Use with caution: env passthrough (token visible in container)
environment:
  - GITHUB_TOKEN
```

### When env passthrough is appropriate

Environment variable passthrough is appropriate for:

- Non-secret configuration (`RUST_BACKTRACE=full`, `MY_FLAG=true`)
- Infrastructure identifiers (`AWS_DEFAULT_REGION=us-east-1`)
- Values that are needed by non-HTTP tools (the proxy only intercepts HTTP/HTTPS traffic)

For secrets, always prefer proxy-based injection where available.
