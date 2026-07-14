# Nodex CLI Reference

This reference describes the commands implemented by the `nodex` CLI as verified against the current source code (`internal/cli/root.go`).

## Syntax

```text
nodex [global-flags] <command> [command-args]
```

Global flags must appear before the command name:

```bash
nodex --output json node list
```

This does NOT work because the flag appears after the command:

```bash
nodex node list --output json
```

Use `nodex help` for top-level help and `nodex help <command>` for per-command help. The CLI does not provide detailed `--help` output for subcommands.

## Global Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--profile <name>` | string | "" | Override the configured current profile |
| `--output <format>` | string | table (TTY), json (non-TTY) | Output format: `table`, `json`, or `yaml` |
| `--timeout <duration>` | duration | 30s | Provider request timeout |
| `--limit <n>` | int | 0 | Limit output items (0 = no limit) |
| `--all` | bool | false | Aggregate across all configured profiles |
| `--no-color` | bool | false | Disable color output |
| `--non-interactive` | bool | false | Disable interactive prompts |
| `--quiet` | bool | false | Suppress non-essential output |
| `--verbose` | bool | false | Info-level stderr output |
| `--debug` | bool | false | Debug-level stderr output (redacted) |

## Mutation Flags

| Flag | Description |
|------|-------------|
| `--yes` | Confirm reversible operations (Tier 1) |
| `--force` | Confirm disruptive operations (Tier 2, requires `--yes`) |
| `--wait` | Wait for provider task to complete before exiting |
| `--expert` | Enable expert-mode operations (Tier 4: identity, ACL changes) |
| `--password-stdin` | Read password from stdin instead of interactive prompt |

`--debug` takes precedence over `--verbose`. `--quiet` suppresses logger output unless a more verbose level is selected.

## Safety Tiers

Every mutation command is classified into one of these tiers:

| Tier | Name | Confirmation Required | Examples |
|------|------|----------------------|----------|
| 0 | Observation | None | `node list`, `vm show` |
| 1 | Reversible | `--yes` or interactive prompt | `vm start`, `vm shutdown` |
| 2 | Disruptive | `--yes --force` or double confirmation | `vm reset`, `vm migrate` |
| 3 | Destructive | Type-in target verification | `vm delete`, `storage delete` |
| 4 | Security Admin | `--expert` flag | `access user create` |

Non-interactive sessions fail closed when confirmation is required and flags are not provided.

## Commands

### `nodex version`

Print version metadata.

```bash
nodex version
```

Subcommands: `version compare <v1> <v2>` (semver comparison), `version parse <v>` (semver parsing).

Output fields: `Nodex <version>`, `Go: <go-version>`, `Commit: <commit>`, `Built: <build-date>`, `Dirty: true` when build metadata reports modified source state.

### `nodex init`

Create the configuration file.

```bash
nodex init
nodex --non-interactive init
```

Interactive mode prompts for provider, endpoint, credential reference, and profile name. Non-interactive mode creates a minimal configuration with a `default` profile using provider `proxmox` and no endpoint. If the configuration file already exists, interactive mode asks before overwriting.

### `nodex completion`

Generate shell completion scripts.

```bash
nodex completion bash
nodex completion zsh
nodex completion fish
```

Writes the completion script to stdout.

### `nodex provider list`

List registered providers.

```bash
nodex provider list
```

### `nodex provider capabilities <name>`

Show capabilities reported by a provider.

```bash
nodex provider capabilities proxmox
```

### `nodex profile`

Manage connection profiles.

Subcommands:

| Command | Description |
|---------|-------------|
| `profile add <name>` | Add a new profile |
| `profile list` | List all configured profiles |
| `profile show <name>` | Show profile details |
| `profile set-credentials <name>` | Set profile credentials (prompts for token) |
| `profile use <name>` | Set the current active profile |
| `profile current` | Show the current active profile |
| `profile test [name]` | Test profile connectivity |
| `profile remove <name> [--remove-credential]` | Remove a profile |
| `profile export <name>` | Export a sanitized profile (no credentials) |
| `profile import` | Import a profile from stdin |

`set-credentials` stores token credentials in the `file` backend by default. Use `--backend keyring` for OS keyring. Use `--credential-name <name>` for a different credential name. This command requires interactive input; rejected with `--non-interactive`.

### `nodex node`

Inspect nodes. Safety: Tier 0 (Observation).

| Command | Description |
|---------|-------------|
| `node list` | List all nodes |
| `node show <name>` | Show node details |
| `node status <name>` | Show detailed node status (CPU, memory, disk, uptime) |
| `node services <name>` | List node services |
| `node network <name>` | Show node network interfaces |
| `node dns <name>` | Show node DNS configuration |
| `node time <name>` | Show node time configuration |
| `node disks <name>` | List node disks |
| `node certificates <name>` | List node TLS certificates |
| `node subscription <name>` | Show node subscription status |
| `node updates <name>` | List available updates |

### `nodex vm`

Inspect and operate virtual machines.

**Read-only commands** (Tier 0):

| Command | Description |
|---------|-------------|
| `vm list` | List all VMs |
| `vm show <id>` | Show VM details (e.g., `pve-a/100`) |
| `vm config <id>` | Show VM configuration |
| `vm snapshots <id>` | List VM snapshots |
| `vm snapshot-config <id> <name>` | Show VM snapshot configuration |

**Lifecycle commands** (Tier 1 — Reversible, requires `--yes`):

| Command | Description |
|---------|-------------|
| `vm start <id>` | Start a VM |
| `vm shutdown <id>` | Graceful VM shutdown (60s timeout) |
| `vm reboot <id>` | Reboot a VM |
| `vm suspend <id>` | Suspend a VM to disk |
| `vm resume <id>` | Resume a suspended VM |
| `vm pause <id>` | Pause (freeze) a VM |
| `vm unpause <id>` | Unpause a frozen VM |

**Disruptive commands** (Tier 2, requires `--yes --force`):

| Command | Description |
|---------|-------------|
| `vm stop <id>` | Stop a VM (force) |
| `vm reset <id>` | Hard reset a VM |
| `vm migrate <id> --target <node>` | Migrate VM to another node |

**Destructive commands** (Tier 3, requires type-in confirmation):

| Command | Description |
|---------|-------------|
| `vm delete <id>` | Delete a VM |

**Configuration and management** (varies by operation):

| Command | Description |
|---------|-------------|
| `vm update <id> <params...>` | Update VM configuration |
| `vm cloud-init <id>` | Regenerate cloud-init config |
| `vm template <id>` | Convert VM to template |
| `vm snapshot <action> <id> [args]` | Create, delete, or rollback snapshots |
| `vm clone <id> --newid <id> --name <name>` | Clone a VM |
| `vm disk <action> <id> <disk> [args]` | Resize or move VM disks |

### `nodex container`

Inspect and operate containers.

**Read-only commands** (Tier 0):

| Command | Description |
|---------|-------------|
| `container list` | List all containers |
| `container show <id>` | Show container details |
| `container config <id>` | Show container configuration |
| `container snapshots <id>` | List container snapshots |
| `container snapshot-config <id> <name>` | Show container snapshot config |

**Lifecycle commands** (Tier 1 — Reversible, requires `--yes`):

| Command | Description |
|---------|-------------|
| `container start <id>` | Start a container |
| `container shutdown <id>` | Graceful container shutdown |
| `container reboot <id>` | Reboot a container |
| `container suspend <id>` | Suspend a container |
| `container resume <id>` | Resume a suspended container |

**Disruptive commands** (Tier 2, requires `--yes --force`):

| Command | Description |
|---------|-------------|
| `container stop <id>` | Stop a container (force) |
| `container migrate <id> --target <node>` | Migrate container |

**Destructive commands** (Tier 3, requires type-in confirmation):

| Command | Description |
|---------|-------------|
| `container delete <id>` | Delete a container |

**Configuration and management** (varies by operation):

| Command | Description |
|---------|-------------|
| `container update <id> <params...>` | Update container config |
| `container template <id>` | Convert container to template |
| `container snapshot <action> <id> [args]` | Create, delete, or rollback snapshots |
| `container clone <id> --newid <id>` | Clone a container |

### `nodex storage`

Inspect and operate storage.

**Read-only commands** (Tier 0):

| Command | Description |
|---------|-------------|
| `storage list` | List all storage pools |
| `storage show <name>` | Show storage details |
| `storage content <name> --node <node>` | List storage content |

**Mutation commands** (varies by operation):

| Command | Description |
|---------|-------------|
| `storage upload <name> --node <node> <path>` | Upload a file to storage |
| `storage download <name> --node <node> <volume>` | Download a volume |
| `storage delete <name> --node <node> <volume>` | Delete a storage volume (destructive) |

### `nodex task`

Inspect tasks. Safety: Tier 0.

| Command | Description |
|---------|-------------|
| `task list --node <node>` | List all tasks for a node |
| `task show --node <node> <upid>` | Show task details |

### `nodex status`

Show cluster status overview. Safety: Tier 0.

```bash
nodex status
```

### `nodex cluster`

Inspect cluster state. Safety: Tier 0.

| Command | Description |
|---------|-------------|
| `cluster status` | Show cluster quorum and node health |
| `cluster log` | Show cluster log entries |

### `nodex event`

List cluster events. Safety: Tier 0.

```bash
nodex event list
```

### `nodex log`

Show node syslog. Safety: Tier 0.

```bash
nodex log --node <node>
```

### `nodex doctor`

Run local configuration checks and connectivity tests. Safety: Tier 0.

```bash
nodex doctor
```

Table output includes `CHECK`, `STATUS`, and `MESSAGE`, followed by a summary. JSON/YAML modes return a structured report with `pass`, `fail`, `warn`, and `results`.

### `nodex backup`

Inspect and manage backups.

**Read-only commands** (Tier 0):

| Command | Description |
|---------|-------------|
| `backup list --node <node>` | List backup tasks |
| `backup content --node <node> --storage <storage>` | List backup content |

**Mutation commands** (varies by operation):

| Command | Description |
|---------|-------------|
| `backup create <vmid> --node <node> --storage <storage>` | Create a manual backup |
| `backup restore <vmid> --archive <archive> --storage <storage>` | Restore VM from backup |
| `backup job list` | List backup job schedules |
| `backup job show <id>` | Show backup job details |
| `backup job create <params...>` | Create a backup job schedule |
| `backup job update <id> <params...>` | Update a backup job schedule |
| `backup job delete <id>` | Delete a backup job schedule |

### `nodex firewall`

Inspect and manage firewall rules.

**Read-only commands** (Tier 0):

| Command | Description |
|---------|-------------|
| `firewall list` | List cluster firewall rules |
| `firewall aliases` | List firewall aliases |
| `firewall ipsets` | List firewall IP sets |
| `firewall ipset <name>` | Show IP set entries |
| `firewall security-groups` | List firewall security groups |
| `firewall group <name>` | Show security group rules |
| `firewall options` | Show firewall options |
| `firewall node-rules <node>` | List node-level firewall rules |
| `firewall vm-rules <node> <vmid>` | List VM-level firewall rules |

**Mutation commands** (varies, may require `--yes`, `--force`, or `--expert`):

| Command | Description |
|---------|-------------|
| `firewall rule create <params...>` | Create a firewall rule |
| `firewall rule update <pos> <params...>` | Update a firewall rule |
| `firewall rule delete <pos>` | Delete a firewall rule |
| `firewall alias create <name> <cidr>` | Create a firewall alias |
| `firewall alias delete <name>` | Delete a firewall alias |
| `firewall ipset create <name>` | Create an IP set |
| `firewall ipset add <name> <cidr>` | Add IP set entry |
| `firewall ipset remove <name> <cidr>` | Remove IP set entry |
| `firewall ipset delete <name>` | Delete an IP set |
| `firewall group create <name>` | Create a security group |
| `firewall group delete <name>` | Delete a security group |
| `firewall options update <params...>` | Update firewall options |

### `nodex ha`

Inspect HA resources. Safety: Tier 0.

| Command | Description |
|---------|-------------|
| `ha list` | List HA resources |
| `ha groups` | List HA groups |
| `ha status` | Show HA status |
| `ha current` | Show current HA resource state |

### `nodex sdn`

Inspect and manage SDN.

**Read-only commands** (Tier 0):

| Command | Description |
|---------|-------------|
| `sdn zones` | List SDN zones |
| `sdn vnets` | List SDN VNets |

**Mutation commands** (varies by operation):

| Command | Description |
|---------|-------------|
| `sdn zone create <type> <zone>` | Create an SDN zone |
| `sdn zone delete <zone>` | Delete an SDN zone |
| `sdn vnet create <vnet> --zone <zone>` | Create an SDN VNet |
| `sdn vnet delete <vnet>` | Delete an SDN VNet |
| `sdn subnet create <vnet> <cidr> <gateway>` | Create an SDN subnet |
| `sdn subnet delete <vnet> <subnet>` | Delete an SDN subnet |
| `sdn controller create <ctrl>` | Create an SDN controller |
| `sdn controller delete <ctrl>` | Delete an SDN controller |

### `nodex pools`

List resource pools. Safety: Tier 0.

```bash
nodex pools list
```

### `nodex network`

Inspect and manage network configuration.

| Command | Description | Tier |
|---------|-------------|------|
| `network show --node <node>` | Show node network interfaces | 0 |
| `network apply --node <node> <config>` | Apply network configuration | varies |
| `network revert --node <node>` | Revert pending network changes | varies |

### `nodex access`

Inspect and manage identity and access control.

**Read-only commands** (Tier 0):

| Command | Description |
|---------|-------------|
| `access users` | List users |
| `access groups` | List groups |
| `access roles` | List roles |
| `access acl` | List ACL entries |
| `access domains` | List authentication domains |
| `access tokens <user>` | List API tokens for a user |

**Mutation commands** (Tier 4 — Security Admin, requires `--expert`):

| Command | Description |
|---------|-------------|
| `access user create <userid> [--password-stdin]` | Create a user |
| `access user delete <userid>` | Delete a user |
| `access acl add <params...>` | Add an ACL entry |

### `nodex ceph`

Inspect and manage Ceph storage.

**Read-only commands** (Tier 0):

| Command | Description |
|---------|-------------|
| `ceph status --node <node>` | Show Ceph cluster status |
| `ceph osd list --node <node>` | List Ceph OSDs |
| `ceph mon list --node <node>` | List Ceph monitors |
| `ceph pool list --node <node>` | List Ceph pools |

**Mutation commands** (varies by operation):

| Command | Description |
|---------|-------------|
| `ceph osd create --node <node> <dev>` | Create a new OSD |
| `ceph osd out --node <node> <osdid>` | Mark OSD as out |
| `ceph osd in --node <node> <osdid>` | Mark OSD as in |
| `ceph osd destroy --node <node> <osdid>` | Destroy an OSD |
| `ceph pool create --node <node> <name> <params...>` | Create a Ceph pool |
| `ceph pool destroy --node <node> <name>` | Destroy a Ceph pool |

### `nodex replication`

Manage replication jobs.

| Command | Description |
|---------|-------------|
| `replication list` | List replication jobs |
| `replication show <id>` | Show replication job details |
| `replication create <params...>` | Create a replication job |
| `replication update <id> <params...>` | Update a replication job |
| `replication delete <id>` | Delete a replication job |
| `replication schedule <id> --node <node>` | Schedule replication now |

## Output Formats

### Table

Table output is intended for terminal use. Byte values use IEC units (KiB, MiB, GiB). Column layout, width, and formatting may change without notice. Scripts should use `--output json` or `--output yaml`.

### JSON

JSON output is indented with two spaces. Empty lists are emitted as `[]` not `null`. Structured output streams never contain human-readable text.

### YAML

YAML output uses native YAML serialization mirroring the JSON shape. Field names and semantics match the JSON contract.

### Operation Result

Mutation commands emit an `OperationResult` envelope:

| Field | Description |
|-------|-------------|
| `schema` | Schema version (currently 1) |
| `operation` | Command name |
| `profile` | Profile name |
| `provider` | Provider backend |
| `target` | Resource identifier |
| `safety` | Safety tier label |
| `upid` | Provider task ID |
| `submitted` | Whether request was accepted |
| `waited` | Whether `--wait` was used |
| `success` | Overall success |
| `changed` | Whether state changed (null = unknown) |
| `status` | Provider status text |
| `warnings` | Human-readable warnings |
| `error.class` | Error classification |
| `error.exit` | Recommended exit code |
| `error.detail` | Error detail message |

## Credential Resolution

See the [configuration reference](configuration.md) for credential backends, environment variables, file paths, and TLS settings.

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
| 11 | Partial failure (`--all`) |
| 12 | Provider error |
| 130 | Interrupted (SIGINT) |
| 143 | Terminated (SIGTERM) |

## Signals and Cancellation

The entry point listens for SIGINT and SIGTERM. On receipt, Nodex cancels the command context. If an error is returned after cancellation, the process exits with 130 for SIGINT or 143 for SIGTERM.

## Error Output

Errors are printed to stderr as `Error: <message>`. The message is passed through redaction and terminal sanitization before printing. When `--output json` is requested, errors are written as structured JSON.
