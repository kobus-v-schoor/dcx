# GitLab Proxy

The GitLab proxy injects your host's GitLab token into API requests from the devcontainer, allowing tools like `git` and `glab` to authenticate without the token ever being present inside the container.

## How It Works

1. On `dcx exec`, the proxy intercepts all HTTPS traffic to GitLab domains
2. It replaces the dummy `GITLAB_TOKEN` / `GLAB_TOKEN` with your **real** GitLab token as an `Authorization: Bearer <token>` header
3. Inside the container, `GITLAB_TOKEN=dummy` and `GLAB_TOKEN=dummy` are set — this tells the `glab` CLI to make API requests, but the real token is only injected at the network layer

## Enabling the GitLab Proxy

```yaml
# ~/.config/dcx/config.yaml
proxy:
  gitlab:
    enabled: true
```

Or via environment variable:

```bash
export DCX_PROXY_GITLAB_ENABLED=true
```

## Token Detection

dcx detects your GitLab token from three sources, checked in order:

| Priority | Source | Description |
|----------|--------|-------------|
| 1 | `GITLAB_TOKEN` env var | Standard GitLab token env var |
| 2 | `GLAB_TOKEN` env var | glab-specific token env var |
| 3 | `glab config get token` | Reads from the `glab` CLI's credential store |

If no token is found, the proxy logs a warning and requests to GitLab are forwarded without authentication.

## Intercepted Domains

By default, the proxy intercepts these domains:

- `gitlab.com`
- `registry.gitlab.com`

### GitLab Self-Managed

For GitLab self-managed deployments, configure custom domains:

```yaml
proxy:
  gitlab:
    enabled: true
    domains:
      - gitlab.example.com
      - registry.gitlab.example.com
```

## Limitations

### No `glab config get token` inside the container

The `glab config get token` command will return an empty value inside the container. This is expected — the real token is only available at the network layer. Tools that use `glab` for API access will work correctly because the proxy injects the real token, but tools that try to read `GITLAB_TOKEN` or `GLAB_TOKEN` directly (rather than making HTTP requests) will see the dummy value.

### No repository-level scoping

The proxy does **not** enforce repository-level access control. It forwards all requests to GitLab with the host token, regardless of which repository the request targets. The proxy's purpose is to keep the token on the host side and inject it at the network layer — it is not an access control mechanism.

### CA certificate requirement

The proxy requires a CA certificate to be injected into the container's trust store. This works automatically on Debian-derived and Alpine images (which have `update-ca-certificates`). Containers based on other distributions may need manual setup.

## Security Best Practices

### Use a project or group access token

> [!TIP]
> Use a [project access token](https://docs.gitlab.com/user/project/settings/project_access_tokens/) or [group access token](https://docs.gitlab.com/user/group/settings/group_access_tokens/) limited to only the repositories and permissions you need.

Project and group access tokens provide:

- **Repository-level access** - limit the token to only the repos you're working on
- **Permission scoping** - grant only the permissions needed (e.g. read/write repository, but not admin)
- **Expiration** - set an automatic expiration date
- **Revocability** - revoke the token instantly if compromised

Since the proxy injects the token at the network layer, even if the container is compromised, the attacker only has the permissions granted to the token — not your full GitLab account access.

### Avoid using your OAuth token

The `glab auth login` command may store an OAuth token with broad permissions. Consider setting `GITLAB_TOKEN` explicitly to a scoped access token instead:

```bash
export GITLAB_TOKEN=glpat-...
dcx exec
```

This gives you full control over what access the container has to your GitLab account.
