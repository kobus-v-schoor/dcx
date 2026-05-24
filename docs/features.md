# Features

dcx can inject [devcontainer features](https://containers.dev/features) into your devcontainer via the `default_features` config option. Features are additional tools and packages that are installed into the container at build time.

> [!TIP]
> For the list of Microsoft-supported features and their options, see the [devcontainer features registry](https://github.com/devcontainers/features). Or, develop your own using the [devcontainer features specification](https://containers.dev/features).

## Configuration

```yaml
# ~/.config/dcx/config.yaml or .devcontainer/dcx.yaml
default_features:
  - id: ghcr.io/devcontainers/features/github-cli
    options:
      version: latest
  - id: ghcr.io/devcontainers/features/docker-outside-of-docker
    options: {}
  - id: ghcr.io/devcontainers/features/node:20
    options:
      version: "20"
```

### Feature ID

Each feature is identified by its registry ID (e.g. `ghcr.io/devcontainers/features/github-cli`). If the ID doesn't include a version tag (a colon followed by a non-empty segment), dcx automatically appends `:latest`.

| ID in config | Effective ID |
|-------------|-------------|
| `ghcr.io/devcontainers/features/github-cli` | `ghcr.io/devcontainers/features/github-cli:latest` |
| `ghcr.io/devcontainers/features/node:20` | `ghcr.io/devcontainers/features/node:20` |

### Feature Options

Each feature accepts an `options` map with feature-specific settings. When no options are needed, use an empty map (`options: {}` or omit the key entirely).

Refer to the [devcontainer features registry](https://github.com/devcontainers/features) for the available options for each feature.

## Merge Behavior

Features from user config and project config are **union-merged**:

- All user features are included first (in their listed order)
- Project-only features are appended (in their listed order)
- If the same feature ID appears in both, the **project entry wins** (its options override the user entry)

This lets you set personal defaults in your user config while allowing projects to customize or override them.

```yaml
# ~/.config/dcx/config.yaml (user config)
default_features:
  - id: ghcr.io/devcontainers/features/github-cli
    options:
      version: "2"

# .devcontainer/dcx.yaml (project config)
default_features:
  - id: ghcr.io/devcontainers/features/github-cli
    options:
      version: "2.42"   # project overrides version
  - id: ghcr.io/devcontainers/features/python
    options:
      version: "3.12"   # project-only feature
```

Result: both `github-cli` (with `version: "2.42"`) and `python` are installed.

## How It Works

dcx serializes the feature list into a JSON object and passes it to the `devcontainer` CLI via the `--additional-features` flag. This is the standard mechanism for injecting features without modifying the original `devcontainer.json`.
