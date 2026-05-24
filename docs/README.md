# dcx Documentation

Welcome to the dcx documentation. This page will help you find the right guide for what you need.

## New to devcontainers?

dcx is a thin wrapper around the official devcontainer CLI - it does not replace it. If you are new to devcontainers, start with the [devcontainer specification](https://containers.dev/) to understand how `devcontainer.json`, features, and lifecycle scripts work. Once you are comfortable with that, dcx adds user-level persistence, credential injection, and workflow automation on top.

## Where to start

- **First time using dcx?** - Read the [CLI Guide](cli.md) to learn the basic commands and a recommended first-time setup.
- **Configuring dcx for your workflow?** - See [Configuration](configuration.md) for how config files, environment variables, and CLI flags work together. You can also browse the fully-commented [`config-reference.yaml`](config-reference.yaml).
- **Using dcx day-to-day?** - The [CLI Guide](cli.md) covers common workflows like starting containers, connecting to them, and rebuilding after config changes.

## Full documentation index

### Getting started

- [`cli.md`](cli.md) - CLI Guide: commands (`up`, `exec`, `stop`, `down`), global flags, and common day-to-day workflows.

### Configuration

- [`configuration.md`](configuration.md) - Configuration Reference: config file locations, precedence rules, and environment variable overrides.
- [`config-reference.yaml`](config-reference.yaml) - A fully-commented example config file showing every available option.

### Integrations

- [`env.md`](env.md) - Environment Passthrough: forward host environment variables into the container, with security guidance on when to prefer proxy-based injection.
- [`features.md`](features.md) - Features: inject devcontainer features automatically via config, with merge behavior between user and project settings.
- [`git.md`](git.md) - Git Integration: forward your host git configs (name, email, aliases) into the container, plus safe-directory handling.
- [`ssh.md`](ssh.md) - SSH Integration: forward your SSH agent socket so git push and SSH operations work inside the container without copying keys.

### Proxy / Secrets injection

- [`proxy/README.md`](proxy/README.md) - Proxy & Secrets Injection: how the transparent MITM proxy works, security properties, and provider architecture.
- [`proxy/github.md`](proxy/github.md) - GitHub Proxy: inject your GitHub token at the network layer so it is never exposed inside the container.
