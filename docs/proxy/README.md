# Proxy & Secrets Injection

dcx uses a **transparent MITM (man-in-the-middle) proxy** to inject credentials into API requests from the devcontainer - without ever exposing those credentials inside the container itself.

## How It Works

When you run `dcx exec`, dcx starts a proxy server on the host machine. The proxy:

1. **Intercepts HTTPS traffic** to configured domains (e.g. `github.com`, `api.github.com`)
2. **Decrypts the request** using a dynamically-generated, per-host TLS certificate signed by an ephemeral CA
3. **Injects credentials** (e.g. your GitHub token as an `Authorization: Bearer <token>` header)
4. **Re-encrypts and forwards** the request to the real destination
5. **Returns the response** to the container

Non-matching domains are tunneled transparently without decryption - the proxy only intercepts traffic to domains registered by enabled providers.

## Architecture

```
┌───────────────────────────────────────────────────────────┐
│  Container                                                │
│                                                           │
│  git / gh / curl -> HTTPS_PROXY -> dcx proxy              │
│                                           │               │
└───────────────────────────────────────────┼───────────────┘
                                            │
                              ┌─────────────▼──────────────┐
                              │  Host: dcx proxy           │
                              │                            │
                              │  1. Decrypt (MITM)         │
                              │  2. Inject credentials     │
                              │  3. Re-encrypt             │
                              │  4. Forward to real server │
                              │                            │
                              └─────────────┬──────────────┘
                                            │
                              ┌─────────────▼──────────────┐
                              │  Real API server           │
                              │  (e.g. api.github.com)     │
                              └────────────────────────────┘
```

## CA Certificate Trust

For the MITM proxy to work, the container must trust the proxy's dynamically-generated TLS certificates. dcx handles this automatically:

1. On proxy startup, dcx generates an **ephemeral CA certificate** (valid for 24 hours by default)
2. The CA cert is copied into the container at `/usr/local/share/ca-certificates/`
3. dcx runs `update-ca-certificates` inside the container to add it to the system trust store
4. When the `dcx exec` session ends, dcx removes the CA cert and runs `update-ca-certificates --fresh`

The CA certificate is unique per session.

## Security Properties

| Property | How dcx enforces it |
|----------|-------------------|
| Credentials never in container | The real token only exists in host process memory. A dummy value (e.g. `GH_TOKEN=dummy`) is set inside the container to trigger API calls, which the proxy then replaces. |
| Request destinations are controlled by dcx | A malicious process inside the container cannot request dcx to forward credentials to other hosts |
| Credentials never on disk | Tokens are read from env vars or the `gh auth token` CLI at runtime. They are never cached. |
| Credentials never logged | Even with `--log-level debug`, dcx logs token presence by name only. |
| Proxy not on public network | By default, the proxy binds to the Docker gateway IP - it is only reachable from the container's network, not from the internet or other containers. |
| Ephemeral CA | The CA certificate is generated per-session and cleaned up on exit. |

## Provider System

dcx uses a provider interface for different credential injection services. Each provider:

- Declares which **domains** it handles
- Provides a **credential injection** function (modifies the request in-place)
- Declares **container env vars** needed for tools to make API requests (e.g. `GH_TOKEN=dummy`)

Currently available providers:

| Provider | Config key | Domains | Injects |
|----------|-----------|---------|---------|
| [GitHub](github.md) | `proxy.github.enabled` | `github.com`, `api.github.com`, etc. | `Authorization: Bearer <token>` header |

## Configuration

```yaml
proxy:
  # Address the proxy listens on. Default: Docker gateway IP (container-only).
  # Set to "0.0.0.0" if the gateway IP is not routable from the container.
  bind_addr: ""

  # CA certificate validity. Default: 24h (ephemeral, per-session)
  cert_expiry: 24h

  # provider-specific config
  # ...
```
