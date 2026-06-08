# Configuration Reference

dcx loads configuration from two YAML files, environment variables, and CLI flags.

> [!TIP]
> A fully-commented config file is available at [`config-reference.yaml`](config-reference.yaml)

## Config File Locations

| Source | Location | When to use |
|--------|----------|-------------|
| **User config** | `~/.config/dcx/config.yaml` | Personal settings that apply to all projects |
| **Project config** | `.devcontainer/dcx.yaml` | Project-specific settings committed to the repo |

If you run `dcx` from a subdirectory of a project, it will automatically traverse up the directory tree to find the `.devcontainer` folder and use the corresponding directory as the project root.

If `XDG_CONFIG_HOME` is set, the user config is read from `$XDG_CONFIG_HOME/dcx/config.yaml`.

## Precedence

Configuration values are resolved in the following order (low → high):

1. **User config** (`~/.config/dcx/config.yaml`)
2. **Project config** (`.devcontainer/dcx.yaml`)
3. **Environment variables** (`DCX_*`)
4. **CLI flags** (`--log-level`, etc.)

Higher-precedence sources override lower ones. For list values (features, mounts, environment), user and project lists are **concatenated** (not replaced).

## Environment Variables

Every config key can be overridden with an environment variable using the `DCX_` prefix and underscores for nesting:

```bash
# proxy.github.enabled → DCX_PROXY_GITHUB_ENABLED
export DCX_PROXY_GITHUB_ENABLED=true

# ssh.forward_agent → DCX_SSH_FORWARD_AGENT
export DCX_SSH_FORWARD_AGENT=true

# default_image → DCX_DEFAULT_IMAGE
export DCX_DEFAULT_IMAGE=mcr.microsoft.com/devcontainers/base:debian

# default_shell → DCX_DEFAULT_SHELL
export DCX_DEFAULT_SHELL=zsh
```
