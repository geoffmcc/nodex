# Product Requirements

This document records the implemented product scope reflected by the current repository. It is not a roadmap.

## Product identity

- Name: Nodex
- Command: `nodex`
- Go module: `github.com/geoffmcc/nodex`
- Current interface: local CLI

## Implemented scope

Nodex is a local, single-user CLI for inspecting Proxmox VE infrastructure. The built-in Proxmox provider is read-only and issues reviewed HTTP `GET` requests. Nodex has no daemon, no background agent, no telemetry, and no implemented resource mutation commands.

## Supported build platforms

The GitHub Actions workflow builds, vets, tests, and checks formatting on:

| OS runner | Go version |
| --- | --- |
| `ubuntu-latest` | `1.25.12` |
| `macos-15` | `1.25.12` |
| `macos-15-intel` | `1.25.12` |
| `windows-latest` | `1.25.12` |

No release artifact matrix is currently defined in the repository.

## Toolchain

- Go module version: `go 1.25.12`
- CI Go version: `1.25.12`
- Build system: `Makefile` wrapping Go commands

## Implemented commands

```text
nodex version
nodex init
nodex completion bash|zsh|fish
nodex profile add <name>
nodex profile list
nodex profile show <name>
nodex profile set-credentials <name> [--backend file|keyring] [--credential-name name]
nodex profile use <name>
nodex profile current
nodex profile test [name]
nodex profile remove <name> [--remove-credential]
nodex provider list
nodex provider capabilities <name>
nodex doctor
nodex node list
nodex node show <name>
nodex vm list
nodex vm show <id>
nodex container list
nodex container show <id>
nodex storage list
nodex storage show <name>
```

Global flags are parsed before the command name: `nodex --output json node list`.

## Global flags

| Flag | Description | Default |
| --- | --- | --- |
| `--profile <name>` | Override current profile. | empty |
| `--output table\|json\|yaml` | Output format. | `table` for TTY stdout, `json` otherwise |
| `--timeout <duration>` | Provider request timeout. | `30s` |
| `--no-color` | Disable color output. | `false` |
| `--non-interactive` | Disable prompts. | `false` |
| `--quiet` | Suppress non-essential success output. | `false` |
| `--verbose` | Info-level stderr output. | `false` |
| `--debug` | Debug-level stderr output with redaction. | `false` |

## Configuration paths

| Platform | Path |
| --- | --- |
| Linux | `$XDG_CONFIG_HOME/nodex/config.yaml` or `~/.config/nodex/config.yaml` |
| macOS | `~/Library/Application Support/Nodex/config.yaml` |
| Windows | `%AppData%\Nodex\config.yaml` |

## Config schema version 1

```yaml
version: 1
current_profile: lab
profiles:
  lab:
    provider: proxmox
    endpoint: https://pve.example.com:8006
    credential_ref: file:lab
    ca_file: /home/alex/.config/nodex/lab-ca.pem
```

Profiles require provider names. Endpoint values are required for live provider commands and must be HTTPS URLs without user info, query strings, fragments, or extra path components.

## Credential behavior

Credential references use `backend:name`; a bare name uses the file backend. Resolver backends are `file`, `keyring`, `env`, and `stdin`.

When a profile has an explicit `credential_ref`, Nodex resolves that reference. Otherwise it tries environment credentials first and then a same-name file credential.

`nodex profile set-credentials` writes only `file` or `keyring` credentials. It prompts for a Proxmox API token ID and token secret and updates the profile's `credential_ref`.

## Proxmox provider contract

The current Proxmox provider uses:

- `GET /api2/json/version`
- `GET /api2/json/nodes`
- `GET /api2/json/cluster/resources`

It maps API data into domain resources for nodes, VMs, containers, storage, and cluster information. It authenticates with `PVEAPIToken` when token ID and token secret credentials are available.

## Output contracts

- Table output is intended for humans.
- JSON and YAML output are structured from the current domain models.
- Empty VM, container, and storage lists are emitted as `[]` in JSON and YAML.
- Error output is formatted as `Error: <message>` and is redacted and terminal-sanitized.

The project has not declared a backward-compatibility policy for structured output fields.

## Exit codes

| Code | Meaning |
| ---: | --- |
| 0 | Success |
| 1 | General/internal error |
| 2 | Usage/validation error |
| 3 | Configuration error |
| 4 | Credential unavailable |
| 5 | Authentication failed |
| 6 | Authorization denied |
| 7 | Network/timeout error |
| 8 | TLS error |
| 9 | Provider incompatibility |
| 10 | Unsupported capability |
| 11 | Partial failure |
| 12 | Provider error |
| 130 | Interrupted by SIGINT |
| 143 | Terminated by SIGTERM |

Some codes are reserved by the application package and are not currently emitted by every provider path.

## TLS defaults

- Certificate validation: enabled
- Hostname verification: enabled
- Minimum TLS version: 1.2
- Custom CA file: supported with `ca_file`
- Insecure TLS mode: not exposed

## HTTP retry and body limits

- Maximum retries: 2
- Base retry delay: 200 ms
- Maximum retry delay: 500 ms
- Jitter: ±25%
- Retried cases: non-TLS transport errors and HTTP 5xx responses
- Non-retried cases: TLS/certificate errors
- Maximum successful API response body: 50 MiB
- Maximum API error body: 256 KiB
