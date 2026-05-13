# DCX — DevContainer Extended: Project Specification

## 1. Project Overview

### 1.1 What Is This Project

`dcx` is a CLI tool, written in Go, that wraps the existing [Dev Containers CLI](https://github.com/devcontainers/cli) to provide user-level persistence and workflow automation. The `devcontainer` CLI is stateless — every invocation requires explicit configuration. `dcx` adds a user config file so that preferences, credentials, and tooling persist across projects and invocations.

`dcx` does not replace the `devcontainer` CLI. It sits in front of it, pre-processing configuration and injecting user-level customizations before delegating all container lifecycle management to the standard toolchain via its documented CLI flags.

The output of this project is a single static binary called `dcx`.

### 1.2 Why This Exists

The Dev Containers specification and CLI solve the core problem of defining and managing development containers. However, several practical gaps create daily friction for developers who use the CLI directly (as opposed to VS Code's integrated terminal):

- **No user-level persistence.** The CLI has no `~/.devcontainer/config.json` or equivalent. Every preference — features to include, mounts to add, env vars to forward — must be specified per-invocation or per-project. VS Code has `dev.containers.defaultFeatures` for user-level feature defaults, but the standalone CLI does not read VS Code settings. Terminal-centric developers have no equivalent.
- **No SSH or git credential forwarding.** VS Code handles this automatically inside its dev container terminal. The CLI does not. This is confirmed as intentional by the CLI maintainers: "The ssh-agent forwarding is part of the Dev Containers extension and not part of the Dev Containers CLI." Developers using the CLI must manually configure socket mounts and environment variables every time.
- **No password manager integration.** No devcontainer tool reads secrets from 1Password, Bitwarden, or any other password manager and injects them into the container. Developers manually export secrets to files or environment variables, which is friction and encourages insecure practices.
- **Shell integration is heavy.** The CLI supports `--dotfiles-repository` for full shell environment replication (clone a git repo, run an install script). This is powerful but heavyweight — it requires a git repo, an install script, and it runs arbitrary code. There is no lightweight mechanism for "just bring my aliases and prompt into the container."
- **Docker Compose integration is awkward for third-party projects.** The spec requires the dev container to be defined as a service in `docker-compose.yml`. Developers contributing to projects they don't own cannot add a dev container without forking.

### 1.3 What Exists Already

| Capability | VS Code | `devcontainer` CLI | `squirrelsoft-dev/dev` | `dcx` |
|---|---|---|---|---|
| User-level default features | ✅ `defaultFeatures` | ❌ | ✅ base config | ✅ |
| User-level default mounts | ❌ (open request) | ❌ | ✅ base config | ✅ |
| SSH agent forwarding | ✅ automatic | ❌ | ❌ | ✅ |
| Git config forwarding | ✅ automatic | ❌ | ❌ | ✅ |
| Password manager → env vars | ❌ | ❌ | ❌ | ✅ |
| Lightweight shell integration | ✅ dotfiles repo | ✅ dotfiles repo | ❌ | ✅ |
| Docker Compose network join | ❌ | ❌ | ❌ | ✅ |
| `init` scaffolding | ✅ (command palette) | ❌ | ✅ `dev init` | ✅ |
| Delegates to official CLI | — | — | ❌ (own runtime) | ✅ |

`squirrelsoft-dev/dev` is the closest existing tool — it provides layered config merging with a base config. However, it is a full CLI replacement (written in Rust, manages containers directly) rather than a thin wrapper. `dcx` deliberately delegates to the official `devcontainer` CLI, inheriting spec compliance and new features automatically.

### 1.4 Project Goals

1. **User-level persistence for CLI users.** A developer's preferences (features, mounts, env vars, shell configs) are stored in a single config file and automatically applied to every container.
2. **Compatibility by default.** `dcx` must never produce a configuration that the standard `devcontainer` CLI or VS Code cannot consume. Any project that works with `devcontainer up` must work with `dcx up` and vice versa.
3. **Delegation, not reimplementation.** All container lifecycle operations are delegated to the `devcontainer` CLI via its documented flags. `dcx` is a pre-processor, not a runtime.
4. **Graceful degradation.** If `dcx` is not installed, or if a project is opened in VS Code directly, everything must still work. `dcx` enhances the terminal workflow — it is not a dependency.

### 1.5 What This Project Is Not

- It is not a replacement for the `devcontainer` CLI.
- It is not a dev container specification alternative.
- It is not a general-purpose Docker wrapper.
- It is not a container runtime (it does not use the Docker SDK for lifecycle management).

---

## 2. Architecture

### 2.1 High-Level Flow

```
Developer runs: dcx up
        │
        ▼
┌──────────────────────────────────┐
│  1. Load user config             │  ~/.config/dcx/config.yaml
│  2. Load project config          │  .devcontainer/dcx.yaml (if present)
│  3. Resolve password manager     │  Execute op read / bw get on host
│     secrets                      │
│  4. Compute merged config        │  User features + project features
│  5. Compute bind mounts          │  User mounts + auto-detected SSH/git
│  6. Compute env passthrough      │  Explicit env vars + resolved secrets
│  7. Compute shell integration    │  Shell file mounts + postCreateCommand
│  8. Handle Compose integration   │  Network join or overlay as configured
│  9. Build CLI invocation         │  Assemble flags and override config
│ 10. Invoke devcontainer CLI      │  Delegate all lifecycle to CLI
└──────────────────────────────────┘
        │
        ▼
  Standard devcontainer CLI
  (does all the real work)
```

### 2.2 Delegation via CLI Flags

`dcx` communicates with the `devcontainer` CLI exclusively through its documented flags. It does not modify `devcontainer.json` files on disk. The primary flags used are:

| Flag | Purpose |
|---|---|
| `--override-config <path>` | Point the CLI to a merged `devcontainer.json` in a temp directory |
| `--additional-features <json>` | Inject user-level default features at runtime |
| `--mount <spec>` | Add bind mounts for user configs, SSH socket, shell files |
| `--remote-env NAME=VALUE` | Pass through env vars and resolved secrets |
| `--secrets-file <path>` | Pass resolved secrets via a secrets file |

The original project `devcontainer.json` is never modified. `dcx` produces a merged configuration in a temp directory and passes it via `--override-config`.

### 2.3 Why Subprocess, Not SDK

The `devcontainer` CLI is a TypeScript application built on yargs. Its internal modules (`launch`, `createDockerParams`, `ProvisionOptions`, etc.) are not part of a public or stable API — they are internal implementation details that change between releases. There is no documented programmatic interface.

Even if `dcx` were written in TypeScript, coupling to internal APIs would be fragile. The CLI's flags (`--override-config`, `--additional-features`, `--mount`, `--remote-env`) are the **intended extension points**. They are documented, stable, and specifically designed for tools like `dcx` to use.

### 2.4 Command Structure

| `dcx` command | Delegates to | Behavior |
|---|---|---|
| `dcx up` | `devcontainer up` | Merge configs, invoke CLI |
| `dcx exec <cmd>` | `devcontainer exec` | Merge configs, invoke CLI |
| `dcx build` | `devcontainer build` | Merge configs, invoke CLI |
| `dcx down` | `devcontainer down` | Merge configs, invoke CLI |
| `dcx init` | — | Generate a `devcontainer.json` for the current project |

For every command except `init`, the merge-and-delegate flow applies. The wrapper does not skip the merge step because the merged config may contain information needed for correct operation (e.g., which Compose file to reference, which features to install).

### 2.5 Configuration Loading Order

Configuration is loaded in the following order, with later sources overriding earlier ones:

1. **User config**: `~/.config/dcx/config.yaml` — the developer's personal defaults
2. **Project config**: `.devcontainer/dcx.yaml` — project-specific wrapper settings (committed to the repo)
3. **Environment variables**: `DCX_*` prefixed variables for CI/automation overrides
4. **CLI flags**: explicit flags on the command line

---

## 3. User-Level Feature Defaults

### 3.1 Problem

Dev Container features are declared per-project in `devcontainer.json`. VS Code has `dev.containers.defaultFeatures` for user-level defaults, but the standalone `devcontainer` CLI does not read VS Code settings. A developer who wants the same features (e.g., OpenCode, GitHub CLI) across all projects must add them to every project's config or pass `--additional-features` manually on every invocation.

### 3.2 Solution

`dcx` reads a list of default features from the user config and passes them to the CLI via the `--additional-features` flag.

### 3.3 User Config Format

```yaml
# ~/.config/dcx/config.yaml
default_features:
  - id: ghcr.io/devcontainers/features/github-cli
    options:
      version: latest
  - id: ghcr.io/opencode/devcontainer-feature/opencode
    options: {}
```

### 3.4 Merge Semantics

The `--additional-features` flag in the `devcontainer` CLI merges additional features with those in the project's `devcontainer.json`. The CLI's own merge behavior is:

- Features present in `--additional-features` but absent from the project config are added.
- Features present in both use the project's definition (project wins on conflict).

This means `dcx` does not need to implement merge logic itself — it simply passes the user's default features to the CLI and the CLI handles the merge. `dcx` only needs to ensure that the user's feature list is serialized to the JSON format expected by `--additional-features`.

### 3.5 Serialization Format

The `--additional-features` flag accepts a JSON object mapping feature IDs to option objects:

```json
{
  "ghcr.io/devcontainers/features/github-cli:1": { "version": "latest" },
  "ghcr.io/opencode/devcontainer-feature/opencode:latest": {}
}
```

`dcx` converts the user's YAML list into this JSON format. The feature ID from the user config is used as the key. If the user does not specify a version tag in the ID, `dcx` appends `:latest`.

---

## 4. Arbitrary Config Bind Mounts

### 4.1 Problem

Developers frequently need host files and directories available inside the dev container: tool configurations (e.g., OpenCode config directory), credential files, shell configs, development tool binaries. The `devcontainer` CLI supports `--mount` for adding mounts, but there is no persistence mechanism — mounts must be specified per-invocation.

### 4.2 Solution

`dcx` reads a list of bind mount declarations from the user config and passes them to the CLI via `--mount` flags. This is a general-purpose mechanism that subsumes SSH socket mounting, git config mounting, shell config mounting, and any other "I need this file/directory inside the container" use case.

### 4.3 User Config Format

```yaml
# ~/.config/dcx/config.yaml
bind_mounts:
  - source: ~/.config/opencode
    target: /home/vscode/.config/opencode
    read_only: true
  - source: ~/Library/Group Containers/2BUA8C4S.com.1password/t/agent.sock
    target: /opt/dcx/sockets/1password-ssh.sock
    read_only: true
  - source: ~/.config/dcx/shell/bash.sh
    target: /opt/dcx/shell/bash.sh
    read_only: true
```

### 4.4 Field Specification

| Field | Required | Description |
|---|---|---|
| `source` | Yes | Path on the host. Supports `~` expansion and environment variable substitution (`${HOME}`, `${SSH_AUTH_SOCK}`, etc.). |
| `target` | Yes | Absolute path inside the container. |
| `read_only` | No | Whether the mount is read-only. Defaults to `true`. |

### 4.5 Source Path Resolution

The `source` path is resolved at invocation time:

1. `~` is expanded to the user's home directory.
2. Environment variable references (`${VAR}`) are expanded using the host's environment.
3. If the resolved path does not exist on the host, the mount is **skipped** with a warning to stderr. This prevents errors when a config references paths that only exist on certain OSes (e.g., macOS-specific paths).

### 4.6 Target Path Conventions

To avoid conflicts with container-installed software, user bind mounts should target paths under `/opt/dcx/`. The shell integration feature (Section 7) and SSH/git auto-detection (Section 6) follow this convention:

| Content | Target path |
|---|---|
| Shell config files | `/opt/dcx/shell/<shell>.<ext>` |
| SSH agent socket | `/opt/dcx/sockets/ssh-agent.sock` |
| Git config | `/opt/dcx/gitconfig` |
| 1Password SSH socket | `/opt/dcx/sockets/1password-ssh.sock` |
| User-defined mounts | User-specified (e.g., `/home/vscode/.config/opencode`) |

### 4.7 Serialization

Each bind mount entry is converted to a `--mount` flag in Docker mount format:

```
--mount type=bind,source=/home/user/.config/opencode,target=/home/vscode/.config/opencode,readonly
```

If `read_only` is `false`, the `readonly` option is omitted.

---

## 5. Environment Variable Passthrough

### 5.1 Problem

Developers need certain host environment variables available inside the dev container: cloud provider credentials, API tokens, service account tokens. The `devcontainer` CLI supports `--remote-env` for passing through env vars, but there is no persistence mechanism.

### 5.2 Solution

`dcx` reads a list of environment variable names from the user config, resolves their values from the host environment, and passes them to the CLI via `--remote-env` flags.

### 5.3 User Config Format

```yaml
# ~/.config/dcx/config.yaml
env_passthrough:
  - AWS_ACCESS_KEY_ID
  - AWS_SECRET_ACCESS_KEY
  - AWS_DEFAULT_REGION
  - OP_SERVICE_ACCOUNT_TOKEN
```

### 5.4 Resolution Behavior

For each variable name in the list:

1. `dcx` reads the variable's value from the host environment.
2. If the variable is set, `dcx` adds `--remote-env NAME=VALUE` to the CLI invocation.
3. If the variable is not set, it is **silently skipped** (no error, no warning). This allows a developer to declare a superset of variables and only the ones that exist on their current machine will be forwarded.

### 5.5 Security Note

`--remote-env` maps to `remoteEnv` in the devcontainer spec, which keeps variables client-side rather than baking them into the container image. However, the variables are still visible inside the container's environment (`env` command, `/proc/self/environ`). They are **not** visible in `docker inspect` on the host. Developers should be aware of this distinction.

---

## 6. SSH and Git Auto-Detection

### 6.1 Problem

VS Code automatically forwards the SSH agent and git configuration into dev containers. The standalone `devcontainer` CLI does not. Developers who use the CLI directly must manually configure socket mounts and environment variables, and the SSH socket path changes on every login (on macOS, it is a random path under `/var/folders/`).

This is not a "remember my flags" problem — it is a **dynamic detection** problem. The socket path cannot be captured in a static config file because it changes. The wrapper must detect it at invocation time.

### 6.2 Solution

`dcx` auto-detects the SSH agent socket and git config on the host and automatically adds the appropriate bind mounts and environment variables. This is enabled by default and can be disabled in the user config.

### 6.3 SSH Agent Forwarding

**Detection:**

1. `dcx` reads `SSH_AUTH_SOCK` from the host environment.
2. If the variable is set and the socket file exists on the host, `dcx` adds:
   - A bind mount: `--mount type=bind,source=<socket_path>,target=/opt/dcx/sockets/ssh-agent.sock,readonly`
   - An environment variable: `--remote-env SSH_AUTH_SOCK=/opt/dcx/sockets/ssh-agent.sock`
3. If `SSH_AUTH_SOCK` is not set or the socket does not exist, this step is silently skipped.

**1Password SSH agent:**

If the 1Password SSH agent socket exists on the host, `dcx` forwards it instead of (or in addition to) the standard SSH agent. Detection paths:

| OS | Socket path |
|---|---|
| macOS | `~/Library/Group Containers/2BUA8C4S.com.1password/t/agent.sock` |
| Linux | `~/.1password/agent.sock` |

If the 1Password socket exists and `SSH_AUTH_SOCK` is not set, `dcx` uses the 1Password socket as `SSH_AUTH_SOCK` inside the container. If both exist, `dcx` forwards the 1Password socket to a separate path (`/opt/dcx/sockets/1password-ssh.sock`) and the standard SSH agent to `/opt/dcx/sockets/ssh-agent.sock`. The developer can choose which to use by setting `SSH_AUTH_SOCK` inside the container.

**User config override:**

```yaml
ssh_forwarding: true   # default: true
```

Set to `false` to disable all SSH agent forwarding (both standard and 1Password).

### 6.4 Git Configuration Forwarding

**Detection:**

1. `dcx` checks for `~/.gitconfig` on the host.
2. If it exists, `dcx` adds:
   - A bind mount: `--mount type=bind,source=<home>/.gitconfig,target=/opt/dcx/gitconfig,readonly`
   - An environment variable: `--remote-env GIT_CONFIG_GLOBAL=/opt/dcx/gitconfig`

**Edge case:** If `~/.gitconfig` includes `includeIf` directives referencing paths outside the container, those includes will not resolve. This is a known limitation. Developers can adjust their `.gitconfig` to be container-compatible (e.g., by using conditional includes that check for `$DEVCONTAINER`).

**User config override:**

```yaml
git_config_forwarding: true   # default: true
```

---

## 7. Shell Integration

### 7.1 Problem

When a developer `exec`s into a dev container, they get the container's default shell configuration — typically a minimal bash with no aliases, no custom prompt, and no keybindings. The container does not feel like their normal working environment.

The `devcontainer` CLI supports `--dotfiles-repository` for full shell environment replication (clone a git repo, run an install script). This is powerful but heavyweight. `dcx` provides a more targeted alternative: mount specific shell config files and inject source lines into system-wide RC files.

### 7.2 Relationship to `--dotfiles-repository`

These are different tools for different use cases:

| | `--dotfiles-repository` | `dcx` shell integration |
|---|---|---|
| How it works | Clones a git repo, runs install script | Mounts specific files, appends source lines to system RC |
| Requires git repo | Yes | No |
| Requires install script | Yes | No |
| Runs arbitrary code | Yes (install script) | No (read-only mount + one source line) |
| Scope | Full dotfiles (plugins, themes, everything) | Targeted: aliases, functions, prompt, keybindings |
| Predictable | Depends on install script | Yes, by design |
| Works offline | Only if repo is cached | Yes, files are local |

Developers who already have a dotfiles repo with an install script should continue using `--dotfiles-repository`. `dcx` shell integration is for developers who want a lightweight, predictable way to bring their aliases and prompt into every container.

### 7.3 User Config Format

```yaml
# ~/.config/dcx/config.yaml
shell_configs:
  bash: ~/.config/dcx/shell/bash.sh
  zsh: ~/.config/dcx/shell/zsh.sh
  fish: ~/.config/dcx/shell/fish.fish
```

Each key is a shell name. Each value is a path to a shell config file on the host. Not all shells need to be specified — a developer who only uses bash would only declare the bash entry.

### 7.4 Injection Mechanism

The injection follows this sequence:

**Step 1: Bind-mount the config files into the container (read-only).**

`dcx` adds `--mount` flags for each declared shell config. The files are mounted at a well-known path under `/opt/dcx/shell/`:

```
--mount type=bind,source=/home/user/.config/dcx/shell/bash.sh,target=/opt/dcx/shell/bash.sh,readonly
--mount type=bind,source=/home/user/.config/dcx/shell/zsh.sh,target=/opt/dcx/shell/zsh.sh,readonly
```

If a declared shell config file does not exist on the host, the mount is skipped with a warning.

**Step 2: Append a source line to the system-wide RC file for each installed shell.**

This is done via the `postCreateCommand` in the override `devcontainer.json`. `dcx` injects a script that:

1. Checks which shells are installed in the container (`command -v bash`, `command -v zsh`, etc.).
2. For each installed shell that has a corresponding mounted config file, locates the system-wide RC file and appends a guarded source line.
3. Uses `grep -qF` to ensure idempotency — running the command multiple times does not append duplicate source lines.

The injected `postCreateCommand` script:

```bash
# For bash
if command -v bash >/dev/null 2>&1 && [ -f /opt/dcx/shell/bash.sh ]; then
    for rcfile in /etc/bash.bashrc /etc/bashrc; do
        if [ -f "$rcfile" ]; then
            grep -qF 'dcx/shell/bash.sh' "$rcfile" 2>/dev/null || \
                echo '[ -f /opt/dcx/shell/bash.sh ] && . /opt/dcx/shell/bash.sh' >> "$rcfile"
            break
        fi
    done
    # If no system-wide RC file exists, create the standard one
    if [ ! -f /etc/bash.bashrc ] && [ ! -f /etc/bashrc ]; then
        echo '[ -f /opt/dcx/shell/bash.sh ] && . /opt/dcx/shell/bash.sh' > /etc/bash.bashrc
    fi
fi

# For zsh
if command -v zsh >/dev/null 2>&1 && [ -f /opt/dcx/shell/zsh.sh ]; then
    for rcfile in /etc/zsh/zshrc /etc/zshrc; do
        if [ -f "$rcfile" ]; then
            grep -qF 'dcx/shell/zsh.sh' "$rcfile" 2>/dev/null || \
                echo '[ -f /opt/dcx/shell/zsh.sh ] && . /opt/dcx/shell/zsh.sh' >> "$rcfile"
            break
        fi
    done
fi

# For fish
if command -v fish >/dev/null 2>&1 && [ -f /opt/dcx/shell/fish.fish ]; then
    for rcfile in /etc/fish/config.fish; do
        if [ -f "$rcfile" ]; then
            grep -qF 'dcx/shell/fish.fish' "$rcfile" 2>/dev/null || \
                echo 'test -f /opt/dcx/shell/fish.fish && source /opt/dcx/shell/fish.fish' >> "$rcfile"
            break
        fi
    done
fi
```

**Step 3: The source line guard.**

Each source line includes a guard (`[ -f /opt/dcx/shell/bash.sh ] &&`) so that if the config file is not mounted (e.g., the container is started without `dcx`), the source command is a no-op. This ensures graceful degradation.

### 7.5 System-Wide RC File Paths

The following paths are probed for each shell, in order. The first file that exists receives the source line:

| Shell | Candidate paths (checked in order) |
|-------|-------------------------------------|
| bash  | `/etc/bash.bashrc`, `/etc/bashrc`   |
| zsh   | `/etc/zsh/zshrc`, `/etc/zshrc`      |
| fish  | `/etc/fish/config.fish`             |

If none of the candidate paths exist for a given shell and that shell is installed, `dcx` creates `/etc/bash.bashrc` (for bash), `/etc/zsh/zshrc` (for zsh), or `/etc/fish/config.fish` (for fish) and writes the source line into it.

### 7.6 Conditional Execution in Shell Configs

Dev containers set the environment variable `DEVCONTAINER=true` (and `REMOTE_CONTAINERS=true` for VS Code compatibility). Developers are encouraged to use this in their shell config files to conditionally enable or adjust behavior:

```bash
# ~/.config/dcx/shell/bash.sh
if [ -n "$DEVCONTAINER" ]; then
    alias gs='git status -sb'
    alias gp='git push'
    alias k='kubectl'

    if [ -z "$STARSHIP_SHELL" ]; then
        PS1='\[\033[01;36m\]dev:\[\033[01;34m\]\w\[\033[00m\]\$ '
    fi

    bind '"\e[1;5D": backward-word'
    bind '"\e[1;5C": forward-word'
fi
```

This allows the developer to share the same shell config file between host and container, with different branches activating depending on the environment.

### 7.7 Interaction With Project Shell Customization

System-wide RC files are sourced **before** user-level RC files (`~/.bashrc`, `~/.zshrc`). This means:

- `dcx`'s injected aliases, functions, and keybindings are defined first.
- The project's own shell initialization (e.g., via a feature that installs oh-my-zsh or starship) runs later and can override them.
- This is the correct layering: `dcx` provides personal defaults, and the project's tooling takes precedence if there is a conflict.

### 7.8 Scope

The shell integration feature is scoped to:

- Aliases
- Functions
- Prompt (`PS1`, `PROMPT`, `fish_prompt`)
- Key bindings (`bind` in bash, `bindkey` in zsh, `bind` in fish)

It is explicitly **not** scoped to:

- Shell plugin frameworks (oh-my-zsh, bash-it, fisher, etc.)
- Shell theme engines (starship, powerlevel10k, etc.)
- Terminal multiplexers (tmux, zellij, screen)
- PATH modifications that affect tool resolution

### 7.9 Merging `postCreateCommand`

The `postCreateCommand` field requires special handling because `dcx` needs to inject its shell integration script while preserving any `postCreateCommand` already defined in the project's `devcontainer.json`.

Since `dcx` writes an override `devcontainer.json` to a temp directory (it never modifies the original), the merge happens in the override config:

- If the project has no `postCreateCommand`, `dcx` sets it to its shell integration script.
- If the project has a `postCreateCommand` (string), `dcx` prepends its shell integration script, joined by ` && `:
  ```
  <dcx-shell-integration-script> && <original-postCreateCommand>
  ```
  `dcx`'s commands run first so that shell configs are available when the project's command executes.
- If the project has a `postCreateCommand` (array of strings), `dcx` prepends its script as the first element.
- If the project has a `postCreateCommand` (object with per-command-type keys), `dcx` prepends to the `postCreateCommand` key specifically.

---

## 8. Password Manager Integration

### 8.1 Problem

No devcontainer tool integrates with password managers. Developers manually export secrets to files or environment variables, which is friction and encourages insecure practices (secrets in shell history, secrets in files that get committed, secrets visible in `docker inspect`).

### 8.2 Solution

`dcx` provides bespoke integrations with specific password managers. Each integration reads secrets from the password manager on the host (at container startup) and injects them as environment variables into the dev container via `--remote-env`.

This is not socket forwarding — `dcx` resolves the secret values on the host and passes them into the container. The password manager CLI does not need to be installed inside the container.

### 8.3 Supported Password Managers

For the initial release, `dcx` supports:

- **1Password** — via the `op` CLI
- **Bitwarden** — via the `bw` CLI

Additional password managers (Vault, `pass`, AWS Secrets Manager, etc.) may be added later. The integration architecture (Section 8.5) is designed to be extensible.

### 8.4 User Config Format

```yaml
# ~/.config/dcx/config.yaml
secrets:
  - name: DATABASE_URL
    provider: 1password
    reference: "op://Engineering/db-host/url"
  - name: AWS_ACCESS_KEY_ID
    provider: 1password
    reference: "op://AWS/access-key-id"
  - name: API_KEY
    provider: bitwarden
    reference: "bw://item-id/field-name"
```

| Field | Required | Description |
|---|---|---|
| `name` | Yes | The environment variable name to set inside the container. |
| `provider` | Yes | The password manager to use. Currently `1password` or `bitwarden`. |
| `reference` | Yes | A provider-specific reference string identifying the secret. |

### 8.5 Provider Architecture

Each password provider implements the following interface:

```go
type SecretProvider interface {
    // Name returns the provider identifier (e.g. "1password", "bitwarden")
    Name() string
    
    // IsAvailable checks whether the provider's CLI is installed
    // and the user is authenticated
    IsAvailable() bool
    
    // Resolve reads a secret from the provider and returns its value
    Resolve(reference string) (string, error)
}
```

`dcx` iterates through the user's `secrets` list at invocation time. For each secret:

1. `dcx` calls `provider.IsAvailable()`. If the provider is not available (CLI not installed or not authenticated), `dcx` prints a warning and skips all secrets for that provider.
2. `dcx` calls `provider.Resolve(reference)`. If resolution fails (secret not found, network error), `dcx` prints a warning and skips that specific secret.
3. If resolution succeeds, `dcx` adds `--remote-env NAME=VALUE` to the CLI invocation.

### 8.6 1Password Integration

**CLI:** `op` (1Password CLI)

**Availability check:**

1. `dcx` runs `op account list` (or `op whoami` on newer versions).
2. If the command exits with code 0, the CLI is installed and the user is authenticated.
3. If the command exits with a non-zero code, the provider is unavailable.

**Secret resolution:**

1. `dcx` runs `op read "<reference>"` on the host.
2. The `reference` uses 1Password's secret reference format: `op://<vault>/<item>/<field>`.
3. `dcx` captures stdout as the secret value.
4. stderr is suppressed unless `--verbose` is set.

**Authentication:** The 1Password CLI uses the user's existing authentication session (established via `op signin` or the 1Password app integration). `dcx` does not handle authentication — it relies on the user being signed in.

**1Password SSH agent:** This is handled separately via the SSH auto-detection feature (Section 6.3), not via the secrets mechanism. The SSH agent socket is forwarded; individual secrets are resolved via `op read`.

### 8.7 Bitwarden Integration

**CLI:** `bw` (Bitwarden CLI)

**Availability check:**

1. `dcx` runs `bw status`.
2. If the output indicates the user is unlocked (status `"unlocked"`), the provider is available.
3. If the output indicates the user is locked or unauthenticated, the provider is unavailable.

**Secret resolution:**

1. `dcx` runs `bw get item <item-id>` on the host, where `<item-id>` is extracted from the reference string.
2. The output is JSON. `dcx` parses it to extract the specified field.
3. The `reference` format for Bitwarden is: `bw://<item-id>/<field-name>`. For example:
   - `bw://abc123def456/password` — get the password field of the item
   - `bw://abc123def456/username` — get the username field
   - `bw://abc123def456/login.password` — explicitly access the login sub-object's password
4. `dcx` extracts the field value from the JSON response.
5. If the field is not found, `dcx` prints a warning and skips the secret.

**Authentication:** The Bitwarden CLI requires the user to be logged in (`bw login`) and unlocked (`bw unlock`). The `BW_SESSION` environment variable must be set. `dcx` does not handle login or unlock — it relies on the user having an active session.

### 8.8 Security Considerations

**Environment variable visibility:** Secrets passed via `--remote-env` are visible inside the container's environment. They are **not** visible in `docker inspect` on the host (unlike `containerEnv`). However, any process running inside the container can read them via `/proc/self/environ` or the `env` command.

**No secret caching:** `dcx` resolves secrets fresh on every invocation. It does not cache secret values on disk. If a developer wants to avoid re-resolving secrets on every `dcx up`, they can use `env_passthrough` (Section 5) with environment variables that they've already exported to their host shell.

**No secret logging:** `dcx` never logs secret values, even with `--verbose`. Log messages reference the secret by name only (e.g., "Resolved secret: DATABASE_URL").

### 8.9 Adding New Providers

To add a new password manager provider:

1. Implement the `SecretProvider` interface (Section 8.5).
2. Register the provider in the provider registry.
3. Document the reference format for the new provider.

No changes to the core `dcx` logic are required. The provider is responsible for its own availability check, authentication state, and secret resolution.

---

## 9. Docker Compose Integration

### 9.1 Problem

Projects that use Docker Compose for integration services (Redis, Postgres, etc.) present two challenges:

**Challenge A: Third-party projects with existing Compose files.** A developer contributing to a project that has a `docker-compose.yml` but no `devcontainer.json` cannot add a dev container without modifying the compose file. This is not possible for open-source contributions without forking.

**Challenge B: Projects where the dev container is already defined in Compose.** Some projects have a `docker-compose.yml` that includes both integration services and the main application service. The developer may want the `devcontainer` CLI to manage the dev service while Compose manages only the integration services.

### 9.2 Strategy: Network Join (Primary)

The dev container is defined independently of the compose file and joins the Compose project's network at runtime.

**How it works:**

1. `dcx` detects the project's `docker-compose.yml` (or the developer specifies it in config).
2. `dcx` starts the integration services by running `docker-compose up -d` (or detects that they are already running).
3. `dcx` determines the Compose project's network name. By convention, this is `<project_name>_default`, where `<project_name>` is derived from:
   - The `COMPOSE_PROJECT_NAME` environment variable (highest priority).
   - The `name` field in the compose file (if present).
   - The directory name containing the compose file (lowest priority).
4. `dcx` adds `--mount` and run args to connect the dev container to the Compose network. Specifically, `dcx` adds `--network=<project_name>_default` to the override config's `runArgs`.
5. Inside the dev container, integration services are reachable by their Compose service names (e.g., `redis:6379`, `postgres:5432`).

**When to use:**

- The project has a `docker-compose.yml` but no `devcontainer.json`.
- The developer does not want to modify the compose file.
- The developer wants the dev container lifecycle to be independent of the Compose lifecycle.

**Config:**

```yaml
# In .devcontainer/dcx.yaml or ~/.config/dcx/config.yaml
compose_integration:
  compose_file: ../docker-compose.yml
  strategy: network_join
```

If `compose_file` is not specified, `dcx` searches the project root and parent directories for `docker-compose.yml` or `compose.yml`.

**Integration service lifecycle:**

When `dcx` starts integration services via `docker-compose up -d`:
- It uses the same `docker-compose.yml` that the project defines.
- It does not modify or override any service definitions.
- It only starts services that are not already running (idempotent).
- It does **not** stop services on `dcx down` by default — integration services may be shared across multiple dev containers. The developer can stop them manually via `docker-compose down` or by passing `--with-services` to `dcx down`.

### 9.3 Strategy: Compose Override (Secondary)

This strategy applies when the project's `docker-compose.yml` already defines a service that is intended to be the dev container (e.g., an `app` service), and the developer wants the `devcontainer` CLI to manage that service.

**How it works:**

1. `dcx` reads the project's `docker-compose.yml` and identifies the dev service (specified by the `dev_service` config key).
2. `dcx` generates an overlay compose file (`.devcontainer/docker-compose.dcx.yml`) that modifies only the dev service:
   - Changes the command to `sleep infinity` (to keep the container running for interactive development).
   - Adds the workspace bind mount.
   - Adds any `dcx`-injected mounts (SSH socket, git config, shell files).
   - Adds any `dcx`-injected environment variables.
3. `dcx` writes an override `devcontainer.json` that references both the original compose file and the overlay:

```json
{
  "dockerComposeFile": [
    "../docker-compose.yml",
    "docker-compose.dcx.yml"
  ],
  "service": "app",
  "workspaceFolder": "/workspace"
}
```

4. The `devcontainer` CLI manages the `app` service using the merged compose configuration. Integration services are started by Compose as dependencies.

**When to use:**

- The project has a `docker-compose.yml` that defines the main application service alongside integration services.
- The developer wants the devcontainer CLI to manage the entire lifecycle via Compose.
- The developer wants to override the dev service's command and configuration for interactive development without modifying the original compose file.

**Config:**

```yaml
# In .devcontainer/dcx.yaml
compose_integration:
  compose_file: ../docker-compose.yml
  strategy: overlay
  dev_service: app
```

**Overlay file generation:**

The generated overlay file is written to `.devcontainer/docker-compose.dcx.yml`. This file is automatically added to `.gitignore` if `dcx` creates it. `dcx` checks for an existing `.gitignore` and appends the entry only if it's not already present.

The overlay file only modifies the dev service — it does not touch integration service definitions. Compose merge semantics ensure that the overlay's service definition is merged with the original, with the overlay taking precedence for any keys specified in both.

**Conflict detection:**

If the `dev_service` is not found in the compose file, `dcx` prints an error and exits. The developer must specify a valid service name.

### 9.4 Strategy Selection

`dcx` selects a strategy based on the following logic:

1. If the project has a `devcontainer.json` that already references a Docker Compose file, `dcx` does **nothing** for Compose integration — the existing config is used as-is. `dcx` only adds user-level customizations (features, shell, credentials) on top.
2. If the project has a `docker-compose.yml` but no `devcontainer.json`, and the `dcx.yaml` specifies `strategy: overlay` with a `dev_service`, `dcx` uses the overlay strategy.
3. If the project has a `docker-compose.yml` but no `devcontainer.json`, and no overlay strategy is configured, `dcx` uses the network join strategy.
4. If the project has neither, `dcx` uses the existing `devcontainer.json` (if present) without any Compose integration.

This logic ensures that `dcx` never interferes with an existing, working devcontainer + Compose setup.

---

## 10. Project Initialization (`dcx init`)

### 10.1 Purpose

Many projects — especially those that already have a `docker-compose.yml` — do not have a `devcontainer.json`. `dcx init` generates one, reducing the boilerplate and decision-making required to get started.

### 10.2 Behavior

**Step 1: Detect the project stack.**

`dcx` scans the project root for indicator files:

| Indicator file | Detected stack | Base image |
|---|---|---|
| `go.mod` | Go | `mcr.microsoft.com/devcontainers/go` |
| `package.json` | Node.js | `mcr.microsoft.com/devcontainers/javascript-node` |
| `requirements.txt` or `pyproject.toml` | Python | `mcr.microsoft.com/devcontainers/python` |
| `Cargo.toml` | Rust | `mcr.microsoft.com/devcontainers/rust` |
| `pom.xml` or `build.gradle` | Java | `mcr.microsoft.com/devcontainers/java` |
| None of the above | Generic | `mcr.microsoft.com/devcontainers/base:ubuntu` |

If multiple indicator files are present, `dcx` prompts the developer to choose (or uses the first match). The prompt can be skipped with a `--stack` flag.

**Step 2: Detect Docker Compose.**

If the project has a `docker-compose.yml` or `compose.yml`, `dcx` asks which strategy to use (network join or overlay) and which service (if any) is the dev container. Based on the answer, it generates the appropriate `devcontainer.json` and `dcx.yaml`.

**Step 3: Generate the config.**

For a Go project with a compose file (network join):

```json
{
  "name": "my-project",
  "image": "mcr.microsoft.com/devcontainers/go:1",
  "features": {},
  "workspaceFolder": "/workspace",
  "workspaceMount": "source=${localWorkspaceFolder},target=/workspace,type=bind",
  "runArgs": ["--network=myproject_default"],
  "postCreateCommand": "go mod download"
}
```

For a Go project with a compose file (overlay):

```json
{
  "name": "my-project",
  "dockerComposeFile": [
    "../docker-compose.yml",
    "docker-compose.dcx.yml"
  ],
  "service": "app",
  "workspaceFolder": "/workspace"
}
```

For a Go project without a compose file:

```json
{
  "name": "my-project",
  "image": "mcr.microsoft.com/devcontainers/go:1",
  "features": {},
  "workspaceFolder": "/workspace",
  "workspaceMount": "source=${localWorkspaceFolder},target=/workspace,type=bind",
  "postCreateCommand": "go mod download"
}
```

**Step 4: Prompt for confirmation.**

`dcx` prints the generated configuration and asks the developer to confirm before writing files. A `--yes` flag skips the confirmation.

**Step 5: Write files.**

`dcx` creates:
- `.devcontainer/devcontainer.json` (always)
- `.devcontainer/dcx.yaml` (if Compose integration is configured)
- `.devcontainer/docker-compose.dcx.yml` (if overlay strategy is selected)
- Appends `.devcontainer/docker-compose.dcx.yml` to `.gitignore` (if overlay strategy and the entry doesn't already exist)

### 10.3 Non-Interactive Mode

For scripting and CI:

```bash
dcx init --stack go --compose-strategy network_join --yes
```

---

## 11. User Configuration Reference

### 11.1 Config File Location

`~/.config/dcx/config.yaml`

This file is optional. If it does not exist, `dcx` uses sensible defaults. `dcx` does not create this file automatically — the developer creates it when they want to customize behavior.

### 11.2 Full Schema

```yaml
# ~/.config/dcx/config.yaml

# --- Default Features ---
# Features to add to every dev container via --additional-features.
# Project features take precedence on conflict.
default_features:
  - id: ghcr.io/devcontainers/features/github-cli
    options:
      version: latest

# --- Bind Mounts ---
# Host paths to bind-mount into every dev container via --mount.
bind_mounts:
  - source: ~/.config/opencode
    target: /home/vscode/.config/opencode
    read_only: true

# --- Environment Variable Passthrough ---
# Host environment variables to forward into every dev container via --remote-env.
env_passthrough:
  - AWS_ACCESS_KEY_ID
  - AWS_SECRET_ACCESS_KEY
  - AWS_DEFAULT_REGION

# --- SSH and Git Auto-Detection ---
# Automatically detect and forward SSH agent socket and git config.
ssh_forwarding: true
git_config_forwarding: true

# --- Shell Integration ---
# Shell config files to mount and inject into system-wide RC files.
shell_configs:
  bash: ~/.config/dcx/shell/bash.sh
  zsh: ~/.config/dcx/shell/zsh.sh
  fish: ~/.config/dcx/shell/fish.fish

# --- Password Manager Secrets ---
# Secrets to resolve from password managers and inject as env vars.
secrets:
  - name: DATABASE_URL
    provider: 1password
    reference: "op://Engineering/db-host/url"
  - name: AWS_ACCESS_KEY_ID
    provider: 1password
    reference: "op://AWS/access-key-id"
  - name: API_KEY
    provider: bitwarden
    reference: "bw://item-id/field-name"

# --- Docker Compose Integration ---
# Auto-detect docker-compose.yml and connect to its network.
compose_auto_detect: true
```

### 11.3 Project-Level Config

A project can include a `.devcontainer/dcx.yaml` file with project-specific wrapper settings. This file is committed to the repo and shared by all developers on the project.

Its schema is a subset of the user config — it supports `compose_integration` and project-specific overrides but does not override user-level preferences (features, shell configs, secrets are personal).

```yaml
# .devcontainer/dcx.yaml
compose_integration:
  compose_file: ../docker-compose.yml
  strategy: network_join
  # strategy: overlay
  # dev_service: app
```

---

## 12. Merged Configuration Lifecycle

### 12.1 Temporary Directory

`dcx` writes merged configurations to a temporary directory under the system temp folder (e.g., `/tmp/dcx-<hash>/`). This directory is created on each `dcx` invocation and is not persisted between runs.

The directory contains:
- `devcontainer.json` — the override config (if needed)
- `docker-compose.dcx.yml` — the generated overlay (if applicable)

### 12.2 Override Config Construction

When `dcx` needs to modify the project's `devcontainer.json` (e.g., to add `postCreateCommand` for shell integration, or to add `runArgs` for Compose network join), it:

1. Reads the project's `devcontainer.json`.
2. Applies modifications (appending to `postCreateCommand`, adding to `runArgs`, etc.).
3. Writes the modified config to the temp directory.
4. Passes the temp config to the CLI via `--override-config`.

The original `devcontainer.json` is never modified.

### 12.3 CLI Invocation Assembly

`dcx` assembles the full CLI invocation by combining:

1. The base command: `devcontainer up`
2. The override config: `--override-config /tmp/dcx-<hash>/devcontainer.json`
3. Default features: `--additional-features '{"ghcr.io/...":{...}}'`
4. Bind mounts: `--mount type=bind,source=...,target=...,readonly` (one per mount)
5. Environment variables: `--remote-env NAME=VALUE` (one per variable)
6. Resolved secrets: `--remote-env NAME=VALUE` (one per secret, same mechanism)
7. User-specified CLI flags: passed through unchanged

The invocation is assembled and logged (with `--verbose`) before execution. Secret values are redacted in the log output.

---

## 13. Implementation Requirements

### 13.1 Language and Distribution

- Written in Go.
- Distributed as a single static binary with no runtime dependencies (other than the `devcontainer` CLI and Docker being installed on the host).
- The binary is named `dcx`.

### 13.2 External Dependencies

- **`devcontainer` CLI** (`@devcontainers/cli`) must be installed and available on `$PATH`. `dcx` detects its absence and prints a clear error message with installation instructions.
- **Docker** must be running and accessible. `dcx` does not start Docker — it assumes the daemon is already running.
- **Password manager CLIs** (`op`, `bw`) are optional. If a provider's CLI is not installed, `dcx` skips secrets for that provider with a warning.

### 13.3 Error Handling

- If the `devcontainer` CLI returns an error, `dcx` exits with the same error code and forwards the error output unchanged.
- If `dcx` encounters a configuration error (invalid `dcx.yaml`, missing referenced file), it prints a descriptive error message and exits with code 1.
- If a non-critical injection fails (e.g., SSH socket not found, shell config file missing, password manager unavailable), `dcx` logs a warning to stderr but continues. These are enhancements, not requirements.

### 13.4 Logging

- `dcx` supports a `--verbose` flag that prints detailed information about what it's doing (config paths, merge decisions, injected mounts, detected networks, resolved secrets by name only).
- Without `--verbose`, `dcx` only prints warnings and errors.
- `dcx` never prints secret values, even with `--verbose`.
- `dcx` never prints anything to stdout that isn't the output of the `devcontainer` CLI (so that piping and scripting work correctly).

### 13.5 Testing

- Unit tests for all merge logic (`postCreateCommand`, `runArgs`, feature serialization, mount generation).
- Unit tests for config loading and validation.
- Unit tests for each password provider's availability check and resolution logic (mocked CLI invocations).
- Integration tests that run `dcx up` against a test project and verify that the container has the expected configuration (SSH socket, shell configs, network connectivity to a Compose service).
- Integration tests require Docker and the `devcontainer` CLI to be installed.

---

## 14. Out of Scope

The following are explicitly not part of this project:

- **VS Code integration.** `dcx` is a terminal tool. VS Code's dev container support handles its own configuration. Projects that use `dcx` for terminal development and VS Code for editor-based development should work identically — `dcx` does not conflict with VS Code.
- **UID mapping.** The devcontainer spec already handles this via `updateRemoteUserUID` (default `true`). `dcx` does not duplicate this.
- **Shell plugin/framework installation.** oh-my-zsh, starship, tmux, etc. are not injected by `dcx`. They can be installed via dev container features or project Dockerfiles.
- **Custom Dockerfile composition.** `dcx` does not generate or merge Dockerfiles. It works with whatever image or Dockerfile the project specifies.
- **Container runtime abstraction.** `dcx` targets Docker and Docker Compose. Podman, containerd, or other runtimes are not supported.
- **Remote host support.** `dcx` assumes Docker is running on the local machine. Docker-over-SSH or remote Docker hosts are not supported.
- **Secret storage or encryption.** `dcx` resolves secrets from password managers and passes them through. It does not store, encrypt, or cache secret values.
- **Dotfiles repository management.** `dcx` provides targeted shell integration (Section 7). Full dotfiles management is the `--dotfiles-repository` flag's job.

---

## 15. Success Criteria

This project is successful when:

1. A developer can run `dcx up` in a project with a `devcontainer.json` and have their SSH agent, git config, default features, and shell aliases automatically available inside the container — with no per-project configuration beyond what the project already defines.
2. A developer can run `dcx init` in a project with a `docker-compose.yml` and get a working `devcontainer.json` that connects to the project's integration services — without modifying the compose file.
3. A developer can declare secrets in `~/.config/dcx/config.yaml` and have them automatically resolved from 1Password or Bitwarden and injected into every dev container — without manually exporting them to environment variables or files.
4. A developer can open the same project in VS Code and have it work identically — `dcx` does not break compatibility.
5. The merged configuration is inspectable (`--verbose`) and understandable — there is no magic, only predictable composition.
6. Removing `dcx` and running `devcontainer up` directly produces the original, unwrapped behavior — `dcx` is fully reversible.
