# Nodex CLI Reference

## Global Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--profile <name>` | Override current profile | — |
| `--output <format>` | Output format: table, json, yaml | table (TTY), json (non-TTY) |
| `--timeout <duration>` | Request timeout | 30s |
| `--no-color` | Disable color output | false |
| `--non-interactive` | Disable interactive prompts | false |
| `--quiet` | Suppress non-essential output | false |
| `--verbose` | Info-level stderr output | false |
| `--debug` | Debug-level stderr output (redacted) | false |

## Commands

### nodex init

Initialize nodex configuration interactively.

```
nodex init
nodex init --non-interactive
```

### nodex profile

Manage connection profiles.

```
nodex profile add <name>
nodex profile list
nodex profile show <name>
nodex profile set-credentials <name> [--backend file|keyring] [--credential-name name]
nodex profile use <name>
nodex profile current
nodex profile test [name]
nodex profile remove <name>
```

### nodex completion

Generate shell completion scripts.

```
nodex completion bash
nodex completion zsh
nodex completion fish
```

### nodex provider

Manage providers.

```
nodex provider list
nodex provider capabilities <name>
```

### nodex node

Manage nodes.

```
nodex node list
nodex node show <name>
```

### nodex vm

Manage virtual machines.

```
nodex vm list
nodex vm show <id>
```

### nodex container

Manage containers.

```
nodex container list
nodex container show <id>
```

### nodex storage

Manage storage.

```
nodex storage list
nodex storage show <name>
```

### nodex doctor

Check system health and connectivity.

```
nodex doctor
```

### nodex version

Print version information.

```
nodex version
```

## Credential Resolution

Credentials are resolved in this order:

1. **credential_ref** in profile (`keyring:myprofile`, `file:default`)
2. **Environment variables** (`NODEX_PROFILE_TOKEN`)
3. **Credential files** in `~/.nodex/credentials/`

Credential references must be either `backend:name` (`keyring`, `file`, `env`, or `stdin`) or a bare file-backend name. Names are validated and cannot contain paths, separators, traversal components, drive-letter paths, UNC paths, or Unicode characters outside the supported profile-name set. Incomplete token or username/password credential pairs are rejected.

Use `nodex profile set-credentials <name>` to prompt for a Proxmox API token ID and secret, store the credentials, and update the profile's `credential_ref`. The command stores file-backed credentials by default; pass `--backend keyring` to use the OS keyring or `--credential-name <name>` to store under a credential name different from the profile name.

### Keyring Backend

Uses the OS keyring (macOS Keychain, Linux Secret Service, Windows Credential Manager).

```
nodex profile add myserver
nodex profile set-credentials myserver --backend keyring
```

### File Backend

Stores credentials as JSON files in `~/.nodex/credentials/`.

```
nodex profile set-credentials myserver

# ~/.nodex/credentials/myserver.json
{
  "type": "token",
  "token_id": "...",
  "token_secret": "..."
}
```

### Environment Backend

Reads from environment variables:

- `NODEX_<PROFILE>_TOKEN`
- `NODEX_<PROFILE>_TOKEN_ID`
- `NODEX_<PROFILE>_TOKEN_SECRET`
- `NODEX_<PROFILE>_USERNAME`
- `NODEX_<PROFILE>_PASSWORD`

## Configuration

Configuration is stored in `~/.config/nodex/config.yaml`:

```yaml
version: 1
current_profile: default
profiles:
  default:
    provider: proxmox
    endpoint: https://pve.example.com:8006
    credential_ref: keyring:default
```

Endpoints must use HTTPS and must not contain URL user info, query strings, fragments, or extra path components. `ca_file` may be used for an additional trusted CA while preserving hostname verification. There is no exposed insecure TLS mode.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Usage error |
| 3 | Configuration error |
| 4 | Credential unavailable |
| 5 | Authentication failed |
| 6 | Authorization denied |
| 7 | Network error |
| 8 | TLS error |
| 9 | Incompatibility |
| 10 | Unsupported capability |
| 11 | Partial failure |
| 12 | Provider error |
| 130 | Interrupted |
| 143 | Terminated |
