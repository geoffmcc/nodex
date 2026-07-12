# Product Requirements

## Product Identity

- **Name:** Nodex
- **Command:** `nodex`
- **Module:** `github.com/geoffmcc/nodex`

## Version 0.1 Scope

Local, single-user, read-only CLI for Proxmox VE. No daemon, no plugins, no mutations, no telemetry.

## Supported Platforms

| OS | Arch | Status |
|----|------|--------|
| Linux | amd64 | Supported |
| Linux | arm64 | Supported |
| macOS | amd64 | Supported |
| macOS | arm64 | Supported |
| Windows | amd64 | Supported |

## Toolchain

- Minimum Go: 1.25
- CI toolchain: 1.26

## Commands

```
nodex version
nodex init
nodex profile add <name>
nodex profile list
nodex profile show <name>
nodex profile use <name>
nodex profile current
nodex profile test [name]
nodex profile remove <name> [--remove-credential]
nodex provider list
nodex provider capabilities
nodex doctor
nodex node list
nodex vm list
nodex container list
nodex storage list
```

## Global Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--profile <name>` | Override current profile | — |
| `--output table\|json\|yaml` | Output format | `table` (TTY), `json` (non-TTY) |
| `--timeout <duration>` | Request timeout | 30s |
| `--no-color` | Disable color | — |
| `--non-interactive` | No prompts | false |
| `--quiet` | Suppress non-essential output | false |
| `--verbose` | Info-level stderr | false |
| `--debug` | Debug-level stderr (redacted) | false |

## Configuration Paths

| Platform | Path |
|----------|------|
| Linux | `$XDG_CONFIG_HOME/nodex/config.yaml` or `~/.config/nodex/config.yaml` |
| macOS | `~/Library/Application Support/Nodex/config.yaml` |
| Windows | `%AppData%\Nodex\config.yaml` |

Credentials stored in sibling `credentials` file.

## Config Schema (v1)

```yaml
version: 1
current_profile: home
profiles:
  home:
    provider: proxmox
    endpoint: https://proxmox.example.com:8006
    credential_ref: file:home
    ca_file: /optional/path/to/custom-ca.pem
```

## Credential Reference Format

`backend:name` — prefixes: `keyring:`, `file:`, `env:`

Backend priority: OS keyring > headless file > env vars > stdin.

## Exit Codes

| Code | Meaning |
|-----:|---------|
| 0 | Success |
| 1 | General/internal |
| 2 | Usage/validation |
| 3 | Configuration |
| 4 | Credential unavailable |
| 5 | Authentication |
| 6 | Authorization |
| 7 | Network/timeout |
| 8 | TLS |
| 9 | Provider incompatibility |
| 10 | Unsupported capability |
| 11 | Partial failure |
| 130 | Interrupted (Ctrl+C) |
| 143 | SIGTERM (Unix) |

## Error Format

```
Error: <human summary>

<explanation>

To fix this:
  - <remediation>

Code: NODEX_<CATEGORY>_<DETAIL>
```

## Output Contracts

- JSON and YAML are public interfaces with stable field names.
- Timestamps: RFC 3339.
- Dedicated output models per resource type.
- Native YAML serialization (not JSON round-trip).
- No secrets in output.

## TLS Defaults

- Certificate validation: enabled
- Hostname verification: enabled
- Custom CA: supported, preserves hostname checks
- Insecure override: `--insecure` flag, warns to stderr, never persisted

## Retry Policy

- Max 2 retries, base delays 200ms/500ms, jitter ±25%
- Retry on: temp network failures, HTTP 502/503/504
- No retry on: TLS, auth, authorization failures
- Max response body: 50 MiB
