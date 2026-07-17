# Product Requirements

This document records the implemented product scope reflected by the current repository. It is not a roadmap. All claims are verified against the source code at the current commit.

## Product Identity

- **Name:** Nodex
- **Command:** `nodex`
- **Go module:** `github.com/geoffmcc/nodex`
- **Interface:** Local CLI, single binary, no daemon

Nodex is a secure, predictable, all-in-one CLI for understanding and operating self-hosted infrastructure—Proxmox-first, inspection-led, management-capable, automation-friendly, and designed to support additional providers without sacrificing provider-native depth.

## Implemented Scope

Nodex is a local, single-user CLI for inspecting and operating Proxmox VE infrastructure. The built-in Proxmox provider supports both read-only inspection commands and mutation commands across 31 capabilities with a five-tier safety model. Nodex has no daemon, no background agent, no telemetry, and no mandatory server component.

### Read-only inspection commands

- Node listing and detail (status, services, network, DNS, time, disks, certificates, subscription, updates)
- VM/container listing, detail, configuration, and snapshots
- Storage listing, detail, and content enumeration
- Task listing and detail
- Cluster status, quorum, and log
- Event listing
- Syslog
- Backup tasks and content listing
- Firewall rules (cluster, node, VM levels), aliases, IP sets, security groups, options
- HA resources, groups, status, and current state
- SDN zones and VNets
- Resource pools
- Ceph status, OSDs, monitors, pools
- Replication job listing
- Access control: users, groups, roles, ACLs, domains, tokens

### Mutation commands

All mutations are gated by the five-tier safety model:

- **VM lifecycle:** start, stop, shutdown, reset, reboot, suspend, resume, pause, unpause
- **Container lifecycle:** start, stop, shutdown, reboot, suspend, resume
- **Configuration updates** for VMs and containers
- **Snapshot management:** create, delete, rollback for VMs and containers
- **Delete** VMs and containers (destructive)
- **Template** conversion for VMs and containers
- **Cloud-init** regeneration for VMs
- **Backup** creation and restore
- **Backup schedule** management (list, create, update, delete)
- **Storage** upload, download, and delete
- **Migration** for VMs and containers
- **Clone** for VMs and containers
- **Disk** resize and move for VMs
- **Network** apply and revert
- **Firewall** rule, alias, IP set, security group, and options mutations
- **SDN** zone, VNet, subnet, and controller mutations
- **Ceph** OSD create/destroy/in/out and pool create/destroy
- **Replication** job create, update, delete, schedule
- **Access** user create and delete, ACL add (expert mode)

## Safety Contracts

### Safety tiers

| Tier | Name | Confirmation |
|------|------|-------------|
| 0 | Observation | None |
| 1 | Reversible | `--yes` or interactive prompt |
| 2 | Disruptive | `--yes --force` or double confirmation |
| 3 | Destructive | Type-in target verification |
| 4 | Security Admin | `--expert` flag |

Non-interactive sessions fail closed when confirmation is required and flags are not provided.

### Mutation result envelope

All mutation commands emit an `OperationResult` (schema version 1) with:

| Field | Type | Description |
|-------|------|-------------|
| `schema` | int | Schema version (1) |
| `operation` | string | Command name (e.g., "vm start") |
| `profile` | string | Profile name (omitted when empty) |
| `provider` | string | Provider backend (e.g., "proxmox") |
| `target` | string | Resource identifier |
| `safety` | string | Safety tier label |
| `upid` | string | Provider task ID (omitted when empty) |
| `submitted` | bool | Whether the request was accepted |
| `waited` | bool | Whether Nodex waited for task completion |
| `success` | bool | Overall success |
| `changed` | bool|null | Whether state was modified |
| `status` | string | Provider status text |
| `warnings` | [string] | Human-readable warnings |
| `error` | object | Error details (omitted on success) |

### Task polling

When `--wait` is used, Nodex polls the provider task with:
- Initial interval: 500ms
- Max interval: 5s (exponential backoff, 2.0x)
- Max wait: 30 minutes
- Context cancellation stops polling and returns the UPID

## Output Contracts

- Table output is intended for humans. Byte values use IEC units.
- JSON output is indented with two spaces. Empty lists are `[]` not `null`.
- YAML output uses native YAML serialization mirroring the JSON shape.
- Structured output (JSON/YAML) never mixes human-readable text.
- Error output is formatted as `Error: <message>` and is redacted and terminal-sanitized.
- When `--output json` is used, errors are also written as structured JSON.

## Provider Model

- **Proxmox-first.** The built-in `proxmox` provider is the initial and primary implementation.
- **Extensible.** New providers are registered through `internal/provider.Register()`.
- **Capability-driven.** Each provider advertises capabilities. Commands check capability support before executing.

## Operational Footprint

- **Local binary.** No daemon, no agent, no background service.
- **No telemetry.** No usage data is collected or transmitted.
- **No server.** Nodex connects directly to infrastructure endpoints.
- **Explicit connections.** Every remote connection is declared through a named profile.
- **Least privilege.** Works with read-only tokens for inspection. Documents minimum permissions for management operations.

## Configuration

### Paths

| Platform | Config path |
|----------|-------------|
| Linux | `$XDG_CONFIG_HOME/nodex/config.yaml` or `~/.config/nodex/config.yaml` |
| macOS | `~/Library/Application Support/Nodex/config.yaml` |
| Windows | `%AppData%\Nodex\config.yaml` |

### Schema

Versions 1 and 2 are read; new configurations are written as version 2. A
file's declared version is preserved by config-modifying commands (no silent
migration). Known providers are `proxmox` (Proxmox VE) and `pbs` (reserved
for Proxmox Backup Server; see `docs/roadmap.md`).

```yaml
version: 2
current_profile: lab
profiles:
  lab:
    provider: proxmox
    endpoint: https://pve.example.com:8006
    credential_ref: file:lab
    ca_file: /path/to/ca.pem
```

### Credential backends

| Backend | Read | Write |
|---------|------|-------|
| file | yes | yes |
| keyring | yes | yes |
| env | yes | no |
| stdin | yes | no |

### Environment variables

For profile `lab`: `NODEX_LAB_TOKEN_ID`, `NODEX_LAB_TOKEN_SECRET`, `NODEX_LAB_TOKEN`, `NODEX_LAB_USERNAME`, `NODEX_LAB_PASSWORD`.

## TLS

- Certificate validation: enabled, cannot be disabled
- Hostname verification: enabled
- Minimum TLS version: 1.2
- Custom CA file: supported via `ca_file` in profile
- Endpoints must use `https://` scheme

## HTTP Transport

- Timeout: 30s default, configurable via `--timeout`
- Max response body: 50 MiB
- Max error body: 256 KiB
- Read retries: up to 2 for transport errors and 5xx
- Mutation retries: none (DoMutation executes exactly once)
- Retry delay: 200ms base, 500ms max, ±25% jitter
- TLS errors not retried

## Exit Codes

| Code | Name | Meaning |
|------|------|---------|
| 0 | Success | Operation completed successfully |
| 1 | General | Unspecified error |
| 2 | Usage | Invalid command arguments |
| 3 | Config | Configuration problem |
| 4 | Credential | Credential unavailable or invalid |
| 5 | Auth | Authentication failed |
| 6 | Authorization | Authorization denied (safety check) |
| 7 | Network | Network connectivity error |
| 8 | TLS | TLS/certificate error |
| 9 | Incompatibility | Provider/API version incompatibility |
| 10 | UnsupportedCap | Capability not supported by provider |
| 11 | PartialFailure | Partial failure in multi-profile `--all` |
| 12 | Provider | Provider-specific error |
| 13 | NotFound | Resource not found |
| 14 | Timeout | Request or task timed out |
| 15 | Cancellation | Operation cancelled (context) |
| 16 | TaskFailure | Provider task completed with failure |
| 17 | ValidationError | Request validation failure |
| 18 | AmbiguousOutcome | Outcome uncertain (UPID returned, status unknown) |
| 19 | RateLimit | Rate limited by provider |
| 20 | OutputError | Error writing output |
| 21 | Conflict | Resource conflict |
| 130 | Interrupted | SIGINT (Ctrl+C) |
| 143 | SIGTERM | SIGTERM |

## Build Platforms

| OS runner | Go version |
|-----------|-----------|
| `ubuntu-latest` | 1.25.12 |
| `macos-15` (ARM) | 1.25.12 |
| `macos-15-intel` | 1.25.12 |
| `windows-latest` | 1.25.12 |

## Current Limitations

- No release artifact matrix is defined
- No backward-compatibility policy for structured output fields beyond what the compatibility document defines
- Some exit codes are reserved but not emitted by every provider path
- Golden test files are test artifacts and may change
- Internal Go package APIs (everything under `internal/`) may change before 1.0
- Provider interface signatures may change before 1.0

## Non-Goals

Nodex is explicitly NOT:
- A daemon or background service
- A dashboard or web UI
- A monitoring server
- A remote control plane
- An agent installed on managed nodes
- A GitOps reconciler
- A raw API executor

## Roadmap

### Completed — Phases 1-3
- Safety and execution integrity (safety tiers, confirmation gates)
- Output and automation contracts (OperationResult envelope, structured JSON/YAML, exit codes)
- Secret and transfer hardening (password-stdin, redaction, streaming uploads, temp-file downloads)

### Phase 4 (current)
- Documentation and command truth — reconciling all docs against implemented code

### Phase 5 (future)
- Provider boundary refinement — interface cleanup, capability contracts

Refer to the [compatibility policy](compatibility.md) for stability commitments.
