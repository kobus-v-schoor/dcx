# SSH Integration

dcx can automatically forward your SSH agent socket from the host into the devcontainer, so you can use SSH keys for `git push`, `ssh`, `scp`, and other SSH-based operations inside the container without copying keys into it.

## How It Works

When SSH agent forwarding is enabled (the default), dcx:

1. Reads `SSH_AUTH_SOCK` from the host environment
2. Verifies the socket file exists and is a Unix socket
3. Bind-mounts the socket into the container at `/opt/dcx/sockets/ssh-agent.sock`
4. Sets `SSH_AUTH_SOCK` inside the container to point at the mounted path

Your **private keys never enter the container** - only the agent socket is forwarded, and the agent (running on the host) handles all signing operations.

## Configuration

```yaml
# ~/.config/dcx/config.yaml
ssh:
  forward_agent: true                                    # default: true
  agent_socket_target: /opt/dcx/sockets/ssh-agent.sock   # default: /opt/dcx/sockets/ssh-agent.sock
```

If `SSH_AUTH_SOCK` is unset or the socket doesn't exist, forwarding is skipped with a warning - dcx won't fail, but SSH operations inside the container won't use your agent.

## Getting Your Keys Into the Agent

The SSH agent only holds keys that have been **explicitly added** to it. If your keys aren't in the agent, SSH forwarding won't help.

Check what keys are currently in your agent:

```bash
ssh-add -l
```

If the list is empty or missing the key you need, add it:

```bash
ssh-add ~/.ssh/id_ed25519
```

> [!IMPORTANT]
> **Common pitfall:** On macOS, SSH keys stored in the Keychain are not automatically loaded into the agent. You may need to run `ssh-add ~/.ssh/id_ed25519` to load them.

## Recommended Setup: Secretive (macOS)

💡 **Recommendation:** Use [Secretive](https://github.com/maxgoedjen/Secretive) on macOS to store your SSH keys in the Secure Enclave.

Secretive provides:

- **Hardware-backed keys** - keys are stored in the Mac's Secure Enclave and **cannot be extracted**
- **Touch ID / Apple Watch authentication** - every SSH operation requires biometric approval
- **Automatic agent** - Secretive runs an SSH agent that dcx can forward

With Secretive, even if someone gains access to your container or host machine, they **cannot use your SSH keys without your biometric approval**.

## Colima Support

If you're using [Colima](https://github.com/abiosoft/colima) as your Docker runtime on macOS, there's an important quirk: the Docker daemon runs inside a Linux VM, and the host's `SSH_AUTH_SOCK` path is **not valid inside the VM**.

dcx handles this automatically:

1. It detects that Colima is the active Docker runtime (via `~/.docker/config.json`)
2. It reads `SSH_AUTH_SOCK` from **inside the Colima VM** (where the Docker daemon lives)
3. It bind-mounts the VM-resident socket into the devcontainer

### Enabling SSH Agent in Colima

Colima must be started with the `--ssh-agent` flag at least once for SSH forwarding to work:

```bash
colima start --ssh-agent
```

If dcx detects that SSH agent forwarding is not enabled in the Colima VM, it will:

- **Interactive terminal:** Prompt you to restart Colima with `--ssh-agent` and offer to do it automatically
- **Non-interactive terminal:** Log an error with instructions to restart manually

### Colima profiles

If you use named Colima profiles, dcx detects the profile from the Docker context name:

| Docker context | Colima profile |
|---------------|----------------|
| `colima` | `default` |
| `colima-<name>` | `<name>` |
