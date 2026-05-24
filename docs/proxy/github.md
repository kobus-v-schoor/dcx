# GitHub Proxy

The GitHub proxy injects your host's GitHub token into API requests from the devcontainer, allowing tools like `git` and `gh` to authenticate without the token ever being present inside the container.

## How It Works

1. On `dcx exec`, the proxy intercepts all HTTPS traffic to GitHub domains
2. It replaces the dummy `GH_TOKEN` with your **real** GitHub token as an `Authorization: Bearer <token>` header
3. Inside the container, `GH_TOKEN=dummy` is set - this tells the `gh` CLI to make API requests, but the real token is only injected at the network layer

## Enabling the GitHub Proxy

```yaml
# ~/.config/dcx/config.yaml
proxy:
  github:
    enabled: true
```

Or via environment variable:

```bash
export DCX_PROXY_GITHUB_ENABLED=true
```

## Token Detection

dcx detects your GitHub token from three sources, checked in order:

| Priority | Source | Description |
|----------|--------|-------------|
| 1 | `GH_TOKEN` env var | Standard GitHub token env var |
| 2 | `GITHUB_TOKEN` env var | Alternative GitHub token env var |
| 3 | `gh auth token` | Reads from the `gh` CLI's credential store |

If no token is found, the proxy logs a warning and requests to GitHub are forwarded without authentication.

## Intercepted Domains

By default, the proxy intercepts these domains:

- `github.com`
- `api.github.com`
- `uploads.github.com`
- `raw.githubusercontent.com`
- `gist.github.com`

### GitHub Enterprise Server

For GitHub Enterprise Server deployments, configure custom domains:

```yaml
proxy:
  github:
    enabled: true
    domains:
      - github.example.com
      - api.github.example.com
```

## Limitations

### No `gh auth token` inside the container

The `gh auth token` command will return `dummy` inside the container. This is expected - the real token is only available at the network layer. Tools that use `gh` for API access will work correctly because the proxy injects the real token, but tools that try to read `GH_TOKEN` directly (rather than making HTTP requests) will see the dummy value.

### No repository-level scoping

The proxy does **not** enforce repository-level access control. It forwards all requests to GitHub with the host token, regardless of which repository the request targets. The proxy's purpose is to keep the token on the host side and inject it at the network layer - it is not an access control mechanism.

### CA certificate requirement

The proxy requires a CA certificate to be injected into the container's trust store. This works automatically on Debian-derived and Alpine images (which have `update-ca-certificates`). Containers based on other distributions may need manual setup.

## Security Best Practices

### Use a fine-grained PAT

> [!TIP]
> Use a [fine-grained personal access token](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens) limited to only the repositories and permissions you need.

Fine-grained PATs provide:

- **Repository-level access** - limit the token to only the repos you're working on
- **Permission scoping** - grant only the permissions needed (e.g. read/write contents, but not admin)
- **Expiration** - set an automatic expiration date
- **Revocability** - revoke the token instantly if compromised

Since the proxy injects the token at the network layer, even if the container is compromised, the attacker only has the permissions granted to the PAT - not your full GitHub account access.

### Avoid using your OAuth/GitHub App token

The `gh auth token` command returns the token from your `gh` CLI login, which may have broad permissions. Consider setting `GH_TOKEN` explicitly to a fine-grained PAT instead:

```bash
export GH_TOKEN=github_pat_...
dcx exec
```

This gives you full control over what access the container has to your GitHub account.
