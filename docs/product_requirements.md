# Product Requirements

This document records the implemented product scope reflected by the current repository. It is not a roadmap. All claims are verified against the source code at the current commit.

## Product Identity

- **Name:** Nodex
- **Command:** `nodex`
- **Go module:** `github.com/geoffmcc/nodex`
- **Interface:** Local CLI, single binary, no daemon

Nodex is a secure, predictable, all-in-one CLI for understanding and operating self-hosted infrastructure—Proxmox-first, inspection-led, management-capable, automation-friendly, and designed to support additional providers without sacrificing provider-native depth.

## Implemented Scope

Nodex is a local CLI for inspecting and operating Proxmox VE and Proxmox Backup Server infrastructure, with optional explicitly enrolled Linux-host maintenance and one-shot monitoring. The built-in Proxmox VE provider advertises 37 capabilities; the separate PBS provider advertises six inspection and four guarded maintenance capabilities. Nodex has no daemon, no agent installed on managed nodes, no telemetry, and no mandatory server component. Its opt-in `--agent` flag is a structured local CLI interface, not a resident agent or isolation boundary.

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
- Proxmox Backup Server (`provider: pbs`): host status, version,
  subscription, certificates, datastore configuration and usage, backup
  snapshots (with namespace/type/ID filters and verification state), tasks
  (list/status/log), verify/prune/sync job configurations, and
  garbage-collection status. Guarded maintenance mutations: verify run
  (reversible), sync run (disruptive; typed confirmation when the job has
  remove-vanished), prune run (destructive, typed confirmation),
  garbage-collection run (disruptive) — all with conflicting-task preflight
  and `--wait` task polling
- Fleet maintenance: `maintenance inventory|status|plan|apply|verify|report|resume|reconcile|abandon` over
  explicitly enrolled hosts, with `--environment/--group/--role/--host`
  filters. Status runs the allowlisted read-only `check-updates` preflight
  (Ansible required, otherwise clearly reported); plan emits an immutable,
  expiring, tamper-evident (SHA-256 digest) plan with per-host package
  intent, conservative ordering, `never` reboot policy, and explicit
  backup-requirement blockers. Apply requires the existing verified plan,
  exact typed plan-ID confirmation, and writes atomic secret-free receipts;
  verification and reporting refuse missing or tampered state.
- Unified environments (`environments` config section, schema v2):
  `environment list|health|backup-health` combining PVE and PBS state —
  reachability, datastore availability/capacity thresholds, active and
  recently failed backup-chain tasks, per-guest backup coverage/age/
  verification with configurable thresholds, maintenance-safety blockers,
  and honest partial-failure reporting (missing data is never "healthy")
- One-shot monitoring (`monitor targets|check`): explicitly configured generic
  HTTP(S), TCP, TLS and DNS targets, plus provider-backed PVE/PBS API, task,
  datastore, and backup coverage/age/verification checks. No target discovery,
  persistent history, scheduling service, or alert delivery.
- Versioned operation and agent discovery: `operation list|describe`,
  `agent contract`, opt-in `--agent` JSON execution and durable local mutation
  receipts with request-ID deduplication and read-only refresh/reconciliation.
  Each operation's agent support and exclusions are available from the live
  operation contract.

### Mutation commands

All mutations are gated by the five-tier safety model:

- **VM lifecycle:** start, stop, shutdown, reset, reboot, suspend, resume
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
- **Guest creation controls:** set VM cores, memory, and optional boot disk, or container cores, memory, swap, and rootfs size before first startup; new containers are unprivileged by default and waited creates verify requested settings by provider readback.
- **Disk** resize and move for VMs
- **Network** apply and revert
- **Firewall** rule, alias, IP set, security group, and options mutations
- **SDN** zone, VNet, subnet, and controller mutations
- **Ceph** OSD create/destroy/in/out and pool create/destroy
- **Replication** job create, update, delete, schedule
- **Access** user create and delete, ACL add (expert mode)
- **Cluster administration:** guarded cluster initialization. Cluster join is
  preflight-only and refuses execution because the required peer credential is
  not safely available to Nodex.
- **Linux guest OS updates:** `container os-update` supports only the fixed
  `approved-full-upgrade` procedure on a running, explicitly identified,
  enrolled LXC; it does not reboot the guest.

## Safety Contracts

### Safety tiers

| Tier | Name | Confirmation |
|------|------|-------------|
| 0 | Observation | None |
| 1 | Reversible | `--yes` or interactive prompt |
| 2 | Disruptive | `--yes --force` or the operation's interactive equivalent |
| 3 | Destructive | `--yes --force` plus exact target confirmation (or interactive equivalent), unless a documented specialized workflow defines its own gate |
| 4 | Security Admin | `--expert` plus any operation-specific requirements |

Non-interactive sessions fail closed when confirmation is required and flags are not provided.

### Mutation result envelope

Provider-backed mutations that use the standard result contract emit an
`OperationResult` (schema version 1) with:

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
| `synchronous` | bool | Whether the provider applied the request inline without a task ID |
| `success` | bool | Request acceptance when not waiting; provider task result when waiting. Independent of postcondition verification. |
| `changed` | bool|null | Whether state was modified |
| `status` | string | Provider status text |
| `verification` | string | Postcondition result when checked (`verified`, `failed`, or `unsupported`) |
| `warnings` | [string] | Human-readable warnings |
| `error` | object | Error details (omitted on success) |

### Task polling

Without `--wait`, an asynchronous mutation's success means provider acceptance,
not task completion. When `--wait` is used, Nodex polls the provider task with:
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
migration). The implemented providers are `proxmox` (Proxmox VE) and `pbs`
(Proxmox Backup Server).

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
| 0 | Success | Command's documented success condition; asynchronous mutation without `--wait` means provider acceptance, not task completion |
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
| 11 | PartialFailure | Partial retrieval or operation failure; used by supported `--all`, environment, maintenance and monitor workflows |
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
| `ubuntu-latest` | 1.27.1 |
| `macos-15` (ARM) | 1.27.1 |
| `macos-15-intel` | 1.27.1 |
| `windows-latest` | 1.27.1 |

## Current Limitations

- Only the PVE and PBS providers are implemented; other registered-provider
  concepts do not imply additional live backends.
- Some operation postconditions cannot be verified from provider evidence; the
  command/agent contract reports verification as unavailable or unsupported.
- `--all` is restricted to `status`, `node list`, `vm list`, and
  `container list`.
- Fleet maintenance and enrolled-host service checks require Ansible;
  maintenance plans currently never reboot hosts.
- The `service` monitoring type is accepted in configuration but currently
  reports unsupported.
- Nodex is pre-1.0. Internal Go package APIs and provider interface signatures
  may change; public CLI/config/output compatibility is defined in
  `compatibility.md`.

## Non-Goals

Nodex is explicitly NOT:
- A daemon or background service
- A dashboard or web UI
- A monitoring server
- A remote control plane
- An agent installed on managed nodes
- A GitOps reconciler
- A raw API executor

## Roadmap and compatibility

The delivered fleet-operations phases and remaining work are tracked in the
[roadmap](roadmap.md). For supported interfaces and pre-1.0 stability
commitments, see the [compatibility policy](compatibility.md).
