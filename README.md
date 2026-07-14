# Nodex

Nodex is a secure, predictable, all-in-one CLI for understanding and operating self-hosted infrastructure—Proxmox-first, inspection-led, management-capable, and automation-friendly.

Nodex runs on Linux, macOS, and Windows as a single local binary. It connects directly to your Proxmox VE endpoints over HTTPS. There is no daemon, no agent, no telemetry, and no hidden network connections.

## What Nodex does

- **Inspect.** List and show nodes, VMs, containers, storage, tasks, events, snapshots, firewall rules, HA resources, backup content, SDN zones, Ceph state, pools, cluster logs, and more.
- **Diagnose.** Run `nodex doctor` to check configuration and connectivity across all your profiles.
- **Operate.** Start, stop, shutdown, reboot, suspend, resume, pause, and unpause VMs and containers. Create and manage snapshots. Update VM and container configurations. Create backups. Upload and download storage content. Migrate and clone guests.
- **Administer** (expert mode). Manage users, ACL entries, firewall rules, SDN topology, Ceph OSDs and pools, backup schedules, and replication jobs.

Every management command is protected by a five-tier safety model. Read-only commands need no confirmation. Reversible operations need `--yes`. Disruptive operations need `--yes --force`. Destructive operations require typing the target identifier. Security administration requires `--expert`.

## Quick start

Install with Go:

```bash
go install github.com/geoffmcc/nodex/cmd/nodex@latest
```

Create a minimal configuration:

```bash
nodex init --non-interactive
nodex provider list
nodex --output json provider capabilities proxmox
```

Connect to Proxmox by editing the configuration file and adding a profile:

```yaml
version: 1
current_profile: lab
profiles:
  lab:
    provider: proxmox
    endpoint: https://pve.example.com:8006
    credential_ref: env:lab
```

Set credentials and test:

```bash
export NODEX_LAB_TOKEN_ID='root@pam!nodex'
export NODEX_LAB_TOKEN_SECRET='example-token-secret'
nodex profile test lab
nodex node list
```

Use fictional or test credentials in examples. Do not paste real tokens into shell history.

## Safety at a glance

| Operation type | Example | Gate |
|---------------|---------|------|
| Read-only | `node list`, `vm show` | None |
| Reversible | `vm start`, `vm shutdown` | `--yes` |
| Disruptive | `vm reset`, `vm migrate` | `--yes --force` |
| Destructive | `vm delete`, `storage delete` | Type target ID |
| Security admin | `access user create` | `--expert` |

Non-interactive sessions fail closed when confirmation is required.

## Credentials

Nodex supports four credential backends:

- **Environment variables** (`env:profilename`) — good for CI and scripts
- **JSON files** (`file:name`) — stored under `~/.nodex/credentials/`
- **OS keyring** (`keyring:name`) — macOS Keychain, Linux Secret Service, Windows Credential Manager
- **Stdin** — read at prompt time, not stored

Proxmox API tokens are the recommended and supported credential type. Passwords may be used through `--password-stdin` for commands like `access user create` but are not supported for Proxmox provider authentication.

## Output modes

- `--output table` (default for terminals) — human-readable tables
- `--output json` (default for non-terminals) — structured JSON, indented, parseable
- `--output yaml` — YAML mirroring the JSON shape

Structured output never contains human-readable text mixed in. Empty lists are `[]`.

## Commands

```text
nodex version            Show version information
nodex init               Initialize configuration
nodex profile            Manage connection profiles
nodex provider           List providers and capabilities
nodex node               Inspect nodes
nodex vm                 Inspect and operate VMs
nodex container          Inspect and operate containers
nodex storage            Inspect and operate storage
nodex task               Inspect tasks
nodex status             Show cluster status overview
nodex cluster            Cluster status and log
nodex event              List cluster events
nodex log                Show node syslog
nodex doctor             Check system health
nodex backup             List, create, and restore backups
nodex firewall           Inspect and manage firewall rules
nodex ha                 Inspect HA resources
nodex sdn                Inspect and manage SDN
nodex pools              List resource pools
nodex network            Inspect and manage network config
nodex access             Inspect and manage identity (expert)
nodex ceph               Inspect and manage Ceph
nodex replication        Manage replication jobs
nodex completion         Generate shell completions
```

Global flags go before the command name: `nodex --output json node list`.

## Documentation

- [Product principles](docs/product-principles.md) — what Nodex is and how capability decisions are made
- [CLI reference](docs/cli-reference.md) — every command, flag, exit code, and safety classification
- [Configuration reference](docs/configuration.md) — profiles, credentials, TLS, paths
- [Architecture](docs/architecture.md) — package layout, provider model, transport, task lifecycle
- [Product requirements](docs/product_requirements.md) — implemented scope, contracts, limitations
- [Compatibility policy](docs/compatibility.md) — what is stable and what may change
- [Security policy](SECURITY.md) — threat model, reporting, protections
- [Support](SUPPORT.md) — what is supported, how to get help
- [Contributing](CONTRIBUTING.md) — development setup, PR requirements

## Requirements

- Go 1.25.12 for building from source
- A Proxmox VE endpoint reachable over HTTPS
- A Proxmox API token (or username/password) with appropriate permissions

CI builds and tests on Ubuntu, macOS (Apple Silicon and Intel), and Windows.

## License

Nodex is licensed under the [GNU General Public License v3.0](LICENSE).
