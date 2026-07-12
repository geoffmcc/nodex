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
nodex profile use <name>
nodex profile current
nodex profile test [name]
nodex profile remove <name>
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
```

### nodex vm

Manage virtual machines.

```
nodex vm list
```

### nodex container

Manage containers.

```
nodex container list
```

### nodex storage

Manage storage.

```
nodex storage list
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

### Keyring Backend

Uses the OS keyring (macOS Keychain, Linux Secret Service, Windows Credential Manager).

```
nodex profile add myserver
# Then set credential_ref to "keyring:myserver" in config.yaml
```

### File Backend

Stores credentials as JSON files in `~/.nodex/credentials/`.

```
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
