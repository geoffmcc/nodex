# Nodex CLI Reference

This reference describes the commands implemented by the `nodex` CLI as verified against the canonical operation registry (`internal/cli/operations.go`) and the command tree (`internal/cli/root.go`).

## Syntax

```text
nodex [global-flags] <command> [command-args]
```

Global flags may appear anywhere on the command line — before, between, or after the command tokens:

```bash
nodex --output json node list
nodex node --output json list
nodex node list --output json
```

Each of these is equivalent. Global flags are extracted wherever they appear; flags a subcommand owns itself (such as `--provider` for `nodex profile add`) are passed through to that handler untouched.

Help is available at any depth with `--help`, `-h`, `-help`, or the `help` command:

```bash
nodex --help
nodex help <command>
nodex <command> --help
nodex help <command> <subcommand> [<operation>]
nodex <command> <subcommand> --help
```

`nodex help` prints the top-level command list, `nodex help version` shows a single command, `nodex vm snapshot --help` shows a subcommand's operations, and `nodex help pbs datastore show` reaches a dispatch operation. Help paths are forgiving: trailing tokens beyond the deepest resolvable command are ignored.

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
| `--agent` | bool | false | Opt in to the versioned structured agent interface; use with a supported operation |
| `--request-id <id>` | string | generated in agent mode | Stable agent request ID for mutation deduplication |

## Mutation Flags

| Flag | Description |
|------|-------------|
| `--yes` | Confirm reversible operations (Tier 1) |
| `--force` | Confirm disruptive operations (Tier 2, requires `--yes`) |
| `--wait` | Wait for provider task to complete before exiting |
| `--expert` | Enable expert-mode operations (Tier 4: identity, ACL changes); Tier 4 also requires `--yes --force`, plus `--confirm-target <text>` for removals |
| `--password-stdin` | Read password from stdin instead of interactive prompt |
| `--confirm-target <text>` | Exact target text for non-interactive destructive confirmation |

`--debug` takes precedence over `--verbose`. `--quiet` suppresses logger output unless a more verbose level is selected. Agent-mode constraints and support are described in the [`nodex agent` section](#nodex-agent) and operation discovery contract.

`--all` is supported only for the read-only `status`, `node list`, `vm list`, and `container list` commands. It is not a general fan-out switch and is unavailable for mutations.

## Safety Tiers

Every mutation command is classified into one of these tiers:

| Tier | Name | Confirmation Required | Examples |
|------|------|----------------------|----------|
| 0 | Observation | None | `node list`, `vm show` |
| 1 | Reversible | `--yes` or interactive prompt | `vm start`, `vm shutdown` |
| 2 | Disruptive | `--yes --force` or double confirmation | `vm reset`, `vm migrate` |
| 3 | Destructive | `--yes --force` plus exact target confirmation (or equivalent interactive confirmation) | `vm delete`, `storage delete` |
| 4 | Security Admin | `--yes --force --expert` (removals also require `--confirm-target <text>`) | `access user create` |

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

### `nodex setup`

Run guided secure setup for a provider profile. Interactive mode prompts only
for non-secret configuration values. Secrets are never accepted as command-line
arguments; use a credential reference to an existing `file`, `keyring`, or
environment credential.

```bash
nodex setup
nodex --non-interactive setup --provider proxmox --profile production \
  --endpoint https://pve.example.com:8006 \
  --credential-ref keyring:production --ca-file /path/to/ca.pem --check
```

`--endpoint` must use HTTPS. `--ca-file`, when supplied, must be a readable
PEM certificate. Non-interactive setup fails closed unless provider, profile,
and endpoint are explicitly supplied. Configuration is written atomically.
`--check` runs read-only connectivity, API-version, capability, and supported
permission diagnostics before the profile is written; a failed preflight does
not modify the configuration. Use `--force` to replace an existing profile.

### `nodex certification`

Run an explicitly selected disposable-environment certification transaction.
Certification is restricted to the configured `nodex-test-admin` profile and
never falls back to the current profile.

```bash
nodex --profile nodex-test-admin --yes --confirm-target nodex-cert-smoke \
  certification run --environment <environment> --suite <readonly|disposable-mutations> \
  --node <node> --vmid <vmid> --name nodex-cert-smoke --storage <storage>
nodex --profile nodex-test-admin --yes --confirm-target <ledger-entry-id> certification cleanup
nodex certification report [--ledger <path>]
```

Run records cleanup intent before creating a VM, verifies the configured
provider/endpoint/leaf certificate and CA identity, waits for task completion,
and verifies both creation and cleanup. A failed or interrupted run remains in
the ledger for recovery; cleanup leases are fenced so stale workers cannot
overlap a recovered cleanup. Names must begin with `nodex-cert-`; reports
contain no credentials or provider response bodies. Do not use this command
against production-looking targets.

### `nodex completion`

Generate shell completion scripts.

```bash
nodex completion bash
nodex completion zsh
nodex completion fish
```

Writes the completion script to stdout.

Supported shells: `bash`, `zsh`, `fish`, and `powershell`.

### `nodex operation`

Discover registered operations without connecting to a provider. Static
capability metadata does not check live permissions or readiness.

```bash
nodex operation list --output json
nodex operation describe "vm migrate" --output json
```

`list` returns the canonical operation paths and concise metadata. `describe`
returns one operation's arguments, flags, constraints, safety tier, target
requirements, provider interface, verification and recovery notes, and
agent-mode support. The runtime registry is the most current command catalog.

### `nodex agent`

Opt in to the versioned JSON agent interface by passing `--agent` before an
operation. It wraps existing handlers and does not bypass safety checks.

```bash
nodex agent contract --output json
nodex agent receipt list --output json
nodex agent receipt show <request-id> --output json
nodex --profile lab agent receipt refresh <request-id> --output json
nodex --profile lab agent receipt reconcile <request-id> --output json
```

For executing an operation, use `nodex --agent --profile <name>
--request-id <id> ... <existing-operation-and-arguments>`. Remote operations
require an explicit profile. Agent mode uses JSON, is non-interactive, and
never inserts confirmation flags. A repeated request ID with identical input
returns the existing receipt; a conflicting request fails. Refresh/reconcile
are read-only with respect to the provider and never resubmit. See the full
[agent-interface contract](agent-interface.md) for result semantics, supported
operations, exclusions, schemas, retention, and recovery rules.

### `nodex monitor`

Run checks against explicitly configured targets once. Nodex does not discover
targets, schedule checks, retain history, or send alerts.

```bash
nodex monitor targets
nodex monitor check
nodex monitor check --target pve-api
nodex monitor check --environment homelab
```

Configured checks include HTTP, HTTPS, TCP, TLS, DNS and ICMP; PVE/PBS API and
task checks; datastore capacity; backup age, verification and coverage; and
systemd service state through enrolled-host Ansible facts. ICMP is `unsupported`
when the local OS or permissions do not provide ping sockets. Reports include
per-target state, latency and detail. Overall non-healthy results return exit
code 11.
Service targets use the inventory host name as `address`, a `.service` unit
name, and require Ansible.
See [Configuration](configuration.md#monitoring-target-types) for target types and fields.

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
| `profile add <name> [--provider proxmox\|pbs]` | Add a new profile (default provider: `proxmox`) |
| `profile list` | List all configured profiles |
| `profile show <name>` | Show profile details |
| `profile set-credentials <name>` | Set profile credentials (prompts for token) |
| `profile use <name>` | Set the current active profile |
| `profile current` | Show the current active profile |
| `profile test [name]` | Test profile connectivity |
| `profile diagnose-permissions <name>` | Report confirmed, missing, unsupported, and unknown setup/permission checks without exposing credentials |
| `profile remove <name> [--remove-credential]` | Remove a profile |
| `profile export <name>` | Export a sanitized profile (no credentials) |
| `profile import <name>` | Import a profile from stdin under the given name |

`set-credentials` stores token credentials in the `file` backend by default. Use `--backend keyring` for OS keyring. Use `--credential-name <name>` for a different credential name. This command requires interactive input; rejected with `--non-interactive`.

### `nodex node`

Inspect nodes. Safety: Tier 0 (Observation), except `node updates` (Tier 1; requires `--yes`).

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
| `node updates <name>` | List available updates; refreshes Proxmox's package-status cache |

`node updates` calls Proxmox's `GET /nodes/<name>/apt/update` endpoint. Proxmox
requires `Sys.Modify` on `/nodes/<name>` for this endpoint, even though Nodex
does not install packages. Because Proxmox may refresh its package-status cache,
Nodex treats the command as Tier 1 and requires confirmation:

```bash
nodex --yes node updates <name>
```

An audit-only API token cannot run this command; grant `Sys.Modify` only if that
additional node-level privilege is acceptable for the token.

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
| `vm stop <id>` | Stop a VM (force) |
| `vm shutdown <id>` | Graceful VM shutdown (60s timeout) |
| `vm suspend <id>` | Suspend a VM in memory |
| `vm resume <id>` | Resume a suspended VM |
| `vm console <node>/<vmid>` | Open an interactive VM serial console (text mode) |

The Proxmox QEMU API has no separate VM `pause` or `unpause` action. `vm pause`
and `vm unpause` return an unsupported-capability error with guidance to use
`vm suspend` and `vm resume`, respectively. Nodex's `vm suspend` leaves
Proxmox's `todisk` option unset, so it suspends the running QEMU process in
memory; it does not save the VM state to disk.

**Disruptive commands** (Tier 2, requires `--yes --force`):

| Command | Description |
|---------|-------------|
| `vm reset <id>` | Hard reset a VM |
| `vm reboot <id>` | Reboot a VM |
| `vm migrate <node>/<vmid> <target> [online]` | Migrate a VM to another node |
| `vm create <node> <vmid> [name] [iso] [disk-storage] [--disk-storage ID] [--cores N] [--memory MiB] [--disk-size GiB]` | Create a VM with bounded CPU, RAM, and optional boot-disk size |

VM creation defaults to 1 core and 512 MiB of RAM. Supplying `disk-storage`
creates a 4 GiB boot disk unless `--disk-size` is specified; without disk
storage, no boot disk is created. Storage may be supplied in the legacy
positional slot or with `--disk-storage`, but not both. `--cores` accepts
1–8192, `--memory` accepts 16–1048576 MiB, and `--disk-size` accepts 1–65536
GiB. Disk size requires an explicit disk storage.
With `--wait`, Nodex reads the created VM configuration back and verifies the
requested CPU, memory, name/ISO, and boot-disk settings. Without `--wait`, the
result reports provider acceptance only.

```bash
nodex vm create pve-a 100 web local:iso/debian.iso --disk-storage local-lvm \
  --cores 4 --memory 8192 --disk-size 64 --yes --force
```

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
| `vm snapshot create <id> <name> [description]` | Create a VM snapshot (`--yes`) |
| `vm snapshot delete <id> <name>` | Delete a VM snapshot (destructive; `--yes --force --confirm-target <name>`) |
| `vm snapshot rollback <id> <name>` | Roll back to a VM snapshot (`--yes --force`) |
| `vm clone <node>/<vmid> <new-vmid> <name> [storage]` | Clone a VM |
| `vm disk move <node>/<vmid> <disk> <storage>` | Move a VM disk to another storage (`--yes --force`) |
| `vm disk resize <node>/<vmid> <disk> <size>` | Resize a VM disk (`--yes --force`) |

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
| `container stop <id>` | Stop a container (force) |
| `container shutdown <id>` | Graceful container shutdown |
| `container suspend <id>` | Suspend a container |
| `container resume <id>` | Resume a suspended container |
| `container console <node>/<vmid>` | Open an interactive container console (text mode) |

**Disruptive commands** (Tier 2, requires `--yes --force`):

| Command | Description |
|---------|-------------|
| `container reboot <id>` | Reboot a container |
| `container migrate <node>/<vmid> <target>` | Migrate a container to another node |
| `container create <node> <vmid> <ostemplate> [hostname] [storage] [--rootfs-storage ID] [--cores N] [--memory MiB] [--swap MiB] [--rootfs-size GiB]` | Create an unprivileged container with bounded resources |
| `container restore <node> <vmid> <archive> [storage]` | Restore a container from a backup archive |

Container creation always requests an unprivileged container. Memory and swap
default to 512 MiB each; when `--cores` is omitted, Proxmox's default CPU
allocation is retained. `--cores` accepts 1–8192, `--memory` accepts 16–1048576
MiB, `--swap` accepts 0–1048576 MiB, and `--rootfs-size` accepts 1–65536 GiB.
Rootfs storage may be supplied in the legacy positional slot or with
`--rootfs-storage`, but not both. A rootfs size requires explicit storage;
without `--rootfs-size`, Proxmox applies its create-time rootfs sizing. If no
storage is supplied, Nodex leaves storage resolution to Proxmox rather than
choosing a storage from the inventory.
With `--wait`, Nodex reads the created container configuration back and verifies
the requested CPU, memory, swap, unprivileged setting, hostname, and rootfs
storage/size. Without `--wait`, the result reports provider acceptance only.

```bash
nodex container create pve-a 200 local:vztmpl/debian-12.tar.zst app-200 \
  --rootfs-storage local-lvm --cores 2 --memory 2048 --swap 512 \
  --rootfs-size 20 --yes --force
```

**Destructive commands** (Tier 3, requires type-in confirmation):

| Command | Description |
|---------|-------------|
| `container delete <id>` | Delete a container |

**Configuration and management** (varies by operation):

| Command | Description |
|---------|-------------|
| `container update <id> <params...>` | Update container config |
| `container os-update <node>/<vmid> --policy approved-full-upgrade` | Update a running LXC guest OS through the enrolled PVE host |
| `container template <id>` | Convert container to template |
| `container snapshot create <id> <name> [description]` | Create a container snapshot (`--yes`) |
| `container snapshot delete <id> <name>` | Delete a container snapshot (destructive; `--yes --force --confirm-target <name>`) |
| `container snapshot rollback <id> <name>` | Roll back to a container snapshot (`--yes --force`) |
| `container clone <node>/<vmid> <new-vmid> <name> [storage]` | Clone a container |

Taskless VM/container template conversion verifies the config flag with up to seven reads and bounded backoff (250 ms initially, capped at 2 s), honoring cancellation. A persistent readable mismatch returns exit 11. `agent receipt refresh` and `reconcile` can check the bound guest again after a completed conversion, even without a task ID. Earlier errors remain in warnings; a matching current flag does not establish historical causation. These checks never resubmit conversion.

`container os-update` is a disruptive operation and requires `--yes --force`.
It accepts only `--policy approved-full-upgrade`, targets an explicitly
identified running LXC, and executes the fixed APT full-upgrade procedure
through its uniquely enrolled PVE host's Ansible connection. It never reboots
the guest. Nodex verifies that the guest remains running and has no remaining
APT or package-database issues after the update. VM operating system updates
are not provided by this command.

### `nodex storage`

Inspect and operate storage.

**Read-only commands** (Tier 0):

| Command | Description |
|---------|-------------|
| `storage list` | List all storage pools |
| `storage show <name>` | Show storage details |
| `storage content <node> <storage>` | List storage content |

**Mutation commands** (varies by operation):

| Command | Description |
|---------|-------------|
| `storage upload <node> <storage> <local-file>` | Upload a local file to storage (`--yes --force`) |
| `storage download <node> <storage> <volume-id> <local-path>` | Download a volume to a local path (`--yes`) |
| `storage delete <node> <storage> <volume-id>` | Delete a storage volume (destructive; `--yes --force --confirm-target <volume-id>`) |

### `nodex task`

Inspect tasks. Safety: Tier 0.

| Command | Description |
|---------|-------------|
| `task list <node>` | List all tasks for a node |
| `task show <node> <upid>` | Show task details |
| `task log <node> <upid>` | Show bounded provider task log lines; supports table, JSON, and YAML |

Proxmox task logs default to 200 lines and accept `--limit`, capped at 2000.
They return the beginning of the log; this is not a complete-log or pagination
guarantee. Access requires task-read permission (normally `Sys.Audit` on the
node for tasks owned by other users); the provider enforces permissions.
Successful tasks with `WARNINGS: N` retain that status and expose a warning in
waited results and refreshed agent receipts.

### `nodex status`

Show cluster status overview. Safety: Tier 0.

```bash
nodex status
nodex status --output json
nodex status --profile homelab
nodex status --watch --profile homelab
nodex status --watch --interval 10s
```

`--watch` runs a read-only dashboard in an alternate terminal screen. It requires
terminal stdin/stdout, table output and one profile; it rejects `--all`, `--agent`,
`--non-interactive`, JSON/YAML, redirected streams and `TERM=dumb`. The selected
profile configuration is pinned for the session. `q` exits normally; Ctrl+C exits
with code 130. Both restore the terminal screen, cursor and input mode.

`--interval` requires `--watch`, defaults to `5s`, and accepts `1s` through `5m`.
Each delay starts after collection finishes, so slow requests never overlap or
queue refreshes. Failed/incomplete refreshes double the delay up to 60 seconds
(configured intervals above 60 seconds stay unchanged). Recovery resets it.
`--timeout` bounds each collection cycle, defaults to 30 seconds and is capped
at five minutes in watch mode. Terminal dimensions are checked each refresh;
small screens clip content. Display text uses ASCII with other characters
replaced by `?`; one-shot structured output preserves full values.

Watch and one-shot status use the same storage and overview collector. Watch also
requests a task sample: the first eight nodes sorted by name, at most five tasks
per node, running tasks first then most recently started. Offline nodes are
skipped with a warning. This is not a cluster-wide task total and does not fetch
task logs. Every cycle repeats the read-only inventory requests plus up to eight
task-list requests; choose a longer interval for large or busy installations.

The dashboard distinguishes healthy, degraded, unreachable and unknown states.
Unavailable sections never appear as successful zero counts. Last successful
refresh means all core queries and supported optional queries completed (HA is
not required on standalone hosts); it does not mean the infrastructure is healthy.
Completely failed refreshes retain the previous snapshot marked stale. Partial
results show current section failures. Collection time does not establish source
freshness. Changes in guest counts, node connectivity and storage are highlighted
for one completed refresh relative to the previous successful refresh.

One-shot JSON/YAML add `sections` with `available`, `unavailable` or `unsupported`
states and optional `warnings`. Guest counts with other/unknown states appear in
`vms_unknown` / `cts_unknown` when nonzero, rather than being counted as
stopped. Consumers must check section availability before interpreting counts.
`status --all` retains its compact cross-profile format and does not support watch.

Storage capacity is reported with evidence and uncertainty:

- Configured PBS entries with the same normalized server, port and datastore
  share a capacity group. Namespaces remain distinct backup views; capacity is
  datastore-wide. This verifies the configured relationship, not physical disks
  or an independently probed server UUID. Credentials are not exposed or used
  as distinct capacities. Different endpoints (including DNS aliases) and
  different datastores are never merged by matching byte counts. Node-relative
  names, loopback/link-local addresses and malformed endpoints stay unresolved.
- An explicit PVE `shared` flag identifies repeated node views of the same
  configured non-PBS storage ID. Separate IDs are not assumed independent.
- All IDs and node views remain visible. Unknown identity, forbidden config
  access, unavailable metrics and conflicting views are explicit. Group metrics
  are omitted when any member is unavailable/disabled or readings disagree;
  per-entry values remain available in JSON/YAML. These are cached provider
  readings, not an atomic or guaranteed-current filesystem measurement.
- Physical disk totals are not established. Do not sum entries or groups:
  distinct datastores, thin pools and local directories may share backing.

The existing `storage[]` numeric fields retain their per-entry meanings. Additive
fields are `id`, `node`, `status`, `identity` and (when established)
`capacity_group`. Identity includes `scope`, `evidence`, an opaque profile-local
`key`, `namespace` for non-root PBS views, `enabled` when config reports it, and
`reason` for unresolved identity. A missing `enabled` value is unknown, not true.
Reported `status` is the provider's accessibility state; it is not a separate
probe or a claim that the storage is enabled.

The additive [`storage_capacity` schema v1](schemas/status-storage-capacity-v1.schema.json)
contains groups, zero-based indexes into `storage[]` for members/unresolved
entries, collection-attempt time, source freshness (`unknown`), and the reason a
physical total is omitted. Group metrics are one consistent reported view,
never a sum. Keys are scoped to this profile and cannot prove relationships
across profiles. `status --all` retains its existing compact profile summary.

Storage uses read-only `/cluster/resources` and, for backup/PBS entries,
`/storage` requests. Config is listed once per snapshot when needed.
PVE filters resources by the token's permissions; the config listing accepts
`Datastore.Audit` or `Datastore.AllocateSpace` for each storage. Missing permission
leaves an unresolved entry instead of failing the entire overview. NodeX does not connect
directly to PBS or request its credentials to establish these relationships.

### `nodex cluster`

Inspect cluster state. Safety: Tier 0.

| Command | Description |
|---------|-------------|
| `cluster status` | Show cluster quorum and node health |
| `cluster log` | Show cluster log entries |
| `cluster init <name> <bind-address>` | Initialize a new PVE cluster; requires `--expert --yes --force` and typing the cluster name, or `--non-interactive --expert --yes --force --confirm-target <name>` |
| `cluster join <node-address> <fingerprint>` | Validate a join target, then refuse execution because PVE requires the peer root password; Nodex never accepts or transports that password |

Cluster initialization posts `clustername` and `link0` to `/cluster/config` and returns the provider worker UPID. It is a destructive cluster and corosync operation, not a reversible configuration change. The join endpoint `/cluster/config/join` also requires a peer root password; the existing profile API token is not equivalent, so Nodex fails closed before making a join request.

#### Standalone hosts

A Proxmox VE host that is not a cluster member reports no `type: "cluster"`
entry from `/cluster/status`. Nodex treats a successful cluster-status query
that returns no cluster entry as **standalone** and reports it explicitly
instead of presenting a zero quorum as a fault.

`nodex status`, `nodex cluster status`, and `nodex ha status` all distinguish
three states, and a failed or unsupported cluster-status query is always
reported as *unavailable* — never as *standalone*:

| State | Meaning |
|-------|---------|
| `standalone` | The host was confirmed not to be a cluster member; a zero quorum is expected. |
| `n/a — standalone host` | Quorum and HA are not applicable on a standalone host. |
| `unavailable` | The cluster state could not be determined; the reason is reported. |

`nodex node services <node>` also annotates `corosync` and `pmxcfs` on a
standalone host. These services are not part of a standalone deployment, so they
are reported as not applicable with an explanatory note while the raw provider
state and `active` flag are preserved.

### `nodex event`

List cluster events. Safety: Tier 0.

```bash
nodex event list
```

### `nodex log`

Show the most recent node syslog entries. Safety: Tier 0.

```bash
nodex log <node> [--last <n>] [--grep <regexp>] [--follow]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--last <n>` | 50 | Show only the `n` most recent entries. Proxmox caps one syslog request at 5000 lines, so `0` (or any value above 5000) requests the maximum rather than an uncapped log. |
| `--grep <regexp>` | none | Show only entries whose text matches this Go regular expression. Matching is case-sensitive. |
| `--follow` | off | Stream matching entries as they arrive until interrupted. Text output only. |

The global `--limit` flag is also accepted and behaves like `--last` for this
command, because a log limit conventionally means the most recent N entries.

Proxmox's `/nodes/{node}/syslog` returns only the line number and message text,
with no timestamps, so time-window filters such as `--since`/`--until` are not
available. Use `--last` and `--grep` to narrow the output instead.

```bash
# last 200 lines
nodex log proxmox --last 200

# only corosync or pveproxy messages, streaming live
nodex log proxmox --grep 'corosync|pveproxy' --follow
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
| `backup list <node>` | List backup tasks |
| `backup content <node> <storage>` | List backup content |

**Mutation commands** (varies by operation):

| Command | Description |
|---------|-------------|
| `backup create <node>/<vmid> <storage> <mode>` | Create a manual backup (`--yes --force`) |
| `backup restore <node> <new-vmid> <archive> [storage]` | Restore a VM from a backup archive (`--yes --force`) |
| `backup job list` | List backup job schedules |
| `backup job show <id>` | Show backup job details |
| `backup job create <params...>` | Create a backup job schedule |
| `backup job update <id> <params...>` | Update a backup job schedule |
| `backup job delete <id>` | Delete a backup job schedule |

### `nodex firewall`

Inspect and manage firewall rules.

**Command naming**: the plural noun is always the read-only list and the
singular noun is always the mutate verb. `firewall security-groups` lists and
`firewall security-group` creates/deletes; `firewall group` remains as a legacy
alias for `firewall security-group`.

**Rule scopes**: firewall rules exist at three scopes, each with its own command
so the scope is always explicit in the command name. `firewall cluster-rules`
reads `/cluster/firewall/rules`; `firewall node-rules <node>` and
`firewall vm-rules <node>/<vmid>` read the node and guest scopes. `firewall list`
and `firewall rules` are documented aliases for `firewall cluster-rules`.

**Read-only commands** (Tier 0):

| Command | Description |
|---------|-------------|
| `firewall cluster-rules` | List cluster-wide firewall rules |
| `firewall list` | Alias for `firewall cluster-rules` |
| `firewall rules` | Alias for `firewall cluster-rules` |
| `firewall aliases` | List firewall aliases |
| `firewall ipsets` | List firewall IP sets |
| `firewall ipset <name>` | Show IP set entries |
| `firewall security-groups` | List firewall security groups |
| `firewall groups` | Alias for `firewall security-groups` |
| `firewall options` | Show firewall options |
| `firewall node-rules <node>` | List node-level firewall rules |
| `firewall vm-rules <node>/<vmid>` | List VM-level firewall rules |

**Mutation commands** (varies, may require `--yes` or `--force`; destructive operations add `--confirm-target`):

| Command | Description |
|---------|-------------|
| `firewall rule create <params...>` | Create a firewall rule |
| `firewall rule update <pos> <params...>` | Update a firewall rule |
| `firewall rule delete <pos>` | Delete a firewall rule |
| `firewall alias create <name> <cidr>` | Create a firewall alias |
| `firewall alias delete <name>` | Delete a firewall alias |
| `firewall ipset create <name>` | Create an IP set |
| `firewall ipset entry add <name> <cidr>` | Add IP set entry |
| `firewall ipset entry remove <name> <cidr>` | Remove IP set entry |
| `firewall ipset delete <name>` | Delete an IP set |
| `firewall security-group create <name>` | Create a security group |
| `firewall security-group delete <name>` | Delete a security group |
| `firewall group create <name>` | Create a security group (legacy alias) |
| `firewall group delete <name>` | Delete a security group (legacy alias) |
| `firewall options update <params...>` | Update firewall options |

### `nodex ha`

Inspect HA resources. Safety: Tier 0.

| Command | Description |
|---------|-------------|
| `ha list` | List HA resources |
| `ha groups` | List HA groups |
| `ha status` | Show HA status |
| `ha current` | Show current HA resource state |

`ha status` reports `standalone`, `cluster`, and `quorum_known` so a standalone
host is distinguishable from a cluster whose quorum could not be read. `enabled`
is `false` on a standalone host, where the HA subsystem does not apply.

### `nodex sdn`

Inspect and manage SDN.

**Command naming**: the plural noun is always the read-only list and the
singular noun is always the mutate verb, so every resource has a matching
`list <plural>` and `manage <singular>` pair.

**Read-only commands** (Tier 0):

| Command | Description |
|---------|-------------|
| `sdn zones` | List SDN zones |
| `sdn vnets` | List SDN VNets |
| `sdn subnets` | List SDN subnets |
| `sdn controllers` | List SDN controllers |

**Mutation commands** (varies by operation):

| Command | Description |
|---------|-------------|
| `sdn zone create <name> --type=<type>` | Create an SDN zone (`simple`, `vlan`, `qinq`, `vxlan`, or `evpn`) |
| `sdn zone delete <name>` | Delete an SDN zone |
| `sdn vnet create <name> --zone=<zone>` | Create an SDN VNet |
| `sdn vnet delete <name>` | Delete an SDN VNet |
| `sdn subnet create <vnet> <cidr> --gateway=<gw>` | Create an SDN subnet |
| `sdn subnet delete <vnet> <subnet>` | Delete an SDN subnet |
| `sdn controller create <name>` | Create an SDN controller |
| `sdn controller delete <name>` | Delete an SDN controller |

### `nodex maintenance`

Fleet maintenance status, planning, guarded apply, verification, and reporting. Requires an `inventory` section (schema version 2) and an installed `ansible-playbook` (ansible-core 2.12+) for status, plan, apply, and verify; Nodex without Ansible keeps full PVE/PBS functionality.

```bash
nodex maintenance inventory [--environment <env>] [--group <group>] [--role <role>] [--host <name>]...
nodex maintenance status    [--environment <env>] [--group <group>] [--role <role>] [--host <name>]...
nodex maintenance plan --policy security-only|approved-full-upgrade \
    [--expires-in <10m..24h>] [--batch-size <1..10>] [filters...]
nodex maintenance apply --plan <file> [--receipt-dir <dir>]
nodex maintenance resume --plan <file> --receipt <file>
nodex maintenance reconcile --plan <file> --receipt <file>
nodex maintenance abandon --receipt <file> --reason <reason>
nodex maintenance verify --plan <file>
nodex maintenance report --receipt <file>

nodex maintenance policy plan [filters...] [--expires-in <duration>]
nodex maintenance policy apply --plan <file> --yes --force --confirm-target <plan-id>
nodex maintenance policy restore --plan <file> --yes --force --confirm-target <plan-id>
```

| Command | Description |
|---------|-------------|
| `maintenance inventory` | List enrolled hosts with role, environment, group, criticality, backup requirement, reboot policy, and unattended-security opt-in |
| `maintenance status` | Run the read-only `check-updates` preflight through the allowlisted Ansible boundary: pending updates, security updates, reboot-required state, failed units, root filesystem usage per host. With `--environment`, adds the environment's backup health. Exits 11 on partial failure. |
| `maintenance plan` | Run the same preflight and emit an immutable plan: plan ID, creation/expiry timestamps (default TTL 4h), update policy, per-host package intent, execution order (standard hosts first, critical hosts serial, PVE/PBS/DNS roles last), batch size, reboot policy (always `never` in this phase), backup requirements and their observed state, infrastructure snapshot, warnings, blockers, and a SHA-256 digest over the whole plan. Security-only plans bind and print the exact candidate version, architecture, APT source, and signed Release identity. Save it with `--output json > plan.json`. |
| `maintenance apply` | Apply an existing digest-verified plan with `--yes --force --confirm-target <plan-id>`. Security-only apply rechecks candidates/origins, refuses holds, removals and unapproved dependency changes, installs exact versions and verifies installed package facts. Writes atomic receipts and refuses blocked, stale, tampered, or ambiguous reruns. |
| `maintenance resume` | Revalidate a plan-bound receipt and continue only hosts not yet started; refuses to replay non-successful hosts. |
| `maintenance reconcile` | Perform read-only postcondition verification against a plan-bound receipt. |
| `maintenance abandon` | Record an explicit operator decision that an interrupted receipt will not be resumed. |
| `maintenance verify` | Verify planned hosts through the embedded read-only Ansible operation. |
| `maintenance report` | Render a verified receipt as table, JSON, or YAML. |
| `maintenance policy plan` | Preview a digest-bound unattended-security APT policy for inventory hosts explicitly opted in with `unattended_security_updates: true`. Shows exact managed-file diffs and current timer state. PVE/PBS/DNS roles are always excluded. |
| `maintenance policy apply` | Install `unattended-upgrades` if needed, back up any existing Nodex-owned policy file, apply the reviewed security-only APT drop-in, enable the daily timer, and verify postconditions. Requires `--yes --force --confirm-target <plan-id>`. It does not install OS updates in the apply transaction and never enables automatic reboot. |
| `maintenance policy restore` | Restore the Nodex-managed policy file and prior timer state from the original plan after rechecking the current state and remote backup. Requires the same confirmation as apply; the `unattended-upgrades` package is retained. If apply used `--receipt-dir`, pass the same directory to restore. |

Plans are tamper-evident (any modification breaks the digest), expiring, deterministic for unchanged inputs, and contain no secrets. A plan created with blockers is still emitted for review but apply refuses it. Backup requirements can only be verified when `--environment` links the hosts to a PVE/PBS pair; without it, `backup_required` hosts are a blocker by design.

The unattended-security policy is distinct from `maintenance plan --policy
security-only`, which applies one reviewed package-update batch. Policy plans
configure future unattended security updates. Save the full JSON plan and
retain it to support a later restore:

```bash
nodex --output json maintenance policy plan --host web-guest > policy.json
plan_id=$(jq -r '.plan_id' policy.json)
nodex --yes --force --confirm-target "$plan_id" maintenance policy apply --plan policy.json
nodex --yes --force --confirm-target "$plan_id" maintenance policy restore --plan policy.json
```

### `nodex environment`

Unified PVE/PBS environment health (read-only). Requires an `environments` section in the configuration (schema version 2; see the configuration reference).

```bash
nodex environment list
nodex environment health <name>
nodex environment backup-health <name>
```

| Command | Description |
|---------|-------------|
| `environment list` | List configured environments and their profiles |
| `environment health <name>` | Infrastructure health: PVE/PBS reachability, datastore availability and capacity, active backup-chain tasks, recent failed PVE/PBS tasks |
| `environment backup-health <name>` | Everything in `health`, plus per-guest backup coverage: every non-excluded PVE guest's newest PBS backup, its age against the environment's thresholds, the datastore and namespace containing it, and its verification state |

Check and guest statuses are `healthy`, `warning`, `blocked`, `unknown` (required data could not be retrieved), or `unsupported` (the provider or configuration cannot answer). The overall status is never healthier than the least healthy check — an evaluation with missing data reports `unknown`, not `healthy`. The result also reports `maintenance_safe` with explicit blockers (unreachable providers, unavailable or full datastores, active backup-chain tasks, stale or missing guest backups).

Exit codes: `healthy` and `warning` exit 0; `blocked`, `unknown`, `unsupported`, or a partial retrieval failure exit 11 (partial failure), so schedulers and scripts can alert on degradation. One provider being down never masks the other: the reachable side is still fully evaluated.

### `nodex pbs`

Inspect a Proxmox Backup Server (read-only). Requires a profile with `provider: pbs` (see the configuration reference).

The whole `pbs` group is gated before any connection attempt. When the selected
profile's provider exposes no PBS capability — the common case on a PVE-only
setup — every `pbs` subcommand fails immediately with exit code 10 and one
actionable message instead of a bare "unsupported capability" error:

```console
$ nodex pbs datastore list
Error: unsupported capability: pbs commands require a profile whose provider
supports PBS, but profile "pve" uses provider "proxmox"; select a pbs profile
with --profile <name> or `nodex profile use <name>`, or add one with
`nodex profile add <name> --provider pbs`
```

`nodex pbs help` and `nodex help pbs` still list every subcommand regardless of
the selected profile, so discovery is never blocked.

```bash
nodex pbs status
nodex pbs version
nodex pbs subscription
nodex pbs certificates
nodex pbs datastore list
nodex pbs datastore show <datastore>
nodex pbs snapshot list --datastore <store> [--namespace <ns>] [--backup-type vm|ct|host] [--backup-id <id>]
nodex pbs task list [--running] [--errors]
nodex pbs task show <upid>
nodex pbs task log <upid>
nodex pbs verify list
nodex pbs prune list
nodex pbs sync list
nodex pbs garbage-collection status [--datastore <store>]
nodex pbs verify run <job-id> | --datastore <store>
nodex pbs sync run <job-id>
nodex pbs prune run <job-id>
nodex pbs garbage-collection run <datastore>
```

| Command | Description |
|---------|-------------|
| `pbs status` | PBS host status (CPU, memory, root filesystem, uptime, kernel) |
| `pbs version` | PBS server version and repository ID |
| `pbs subscription` | Subscription status |
| `pbs certificates` | API certificate details, including expiry |
| `pbs datastore list` | Datastore configurations |
| `pbs datastore show <datastore>` | One datastore's configuration plus usage; usage errors (e.g. an unmounted removable datastore) are reported in `status_error` without failing the command |
| `pbs snapshot list --datastore <store>` | Backup snapshots in a datastore, including owner, protection, and verification state; `--namespace`, `--backup-type`, and `--backup-id` narrow the listing |
| `pbs task list` | Recent tasks; `--running` and `--errors` filter, global `--limit` caps the count |
| `pbs task show <upid>` | Task status and exit state for a PBS UPID |
| `pbs task log <upid>` | Task log lines |
| `pbs verify list` | Verification job configurations |
| `pbs prune list` | Prune job configurations with keep policies |
| `pbs sync list` | Sync job configurations |
| `pbs garbage-collection status` | Garbage-collection status for all datastores, or one with `--datastore` |

The inspection commands above are read-only (safety tier: observation) and support `--output table|json|yaml`. Empty listings produce stable empty results (`[]` in JSON).

#### Guarded PBS mutations

The four `run` commands start provider-native maintenance tasks. Each returns the task UPID in the standard `OperationResult` envelope, supports `--wait` for task polling, refuses to start while a conflicting PBS task (backup, GC, prune, sync, or verify) is running on the same datastore (exit code 21), and fails closed with `--non-interactive` when its confirmation is not satisfied by flags:

| Command | Tier | Confirmation | Rationale |
|---------|------|--------------|-----------|
| `pbs verify run <job-id>` / `--datastore <store>` | reversible | `--yes` | Integrity check; writes only verification metadata, removes nothing |
| `pbs sync run <job-id>` | disruptive | `--yes --force` | Executes the admin-configured sync job. When the job has `remove-vanished` enabled, Nodex warns and escalates to typed confirmation of the job ID, because snapshots missing on the source are deleted locally |
| `pbs prune run <job-id>` | destructive | `--yes --force` + type the job ID | Permanently removes backup snapshots per the job's keep policy |
| `pbs garbage-collection run <datastore>` | disruptive | `--yes --force` | Permanently frees chunks unreferenced by any backup index; restorable snapshots are unaffected, freed data is unrecoverable |

Job-based runs validate that the job exists first (`ExitNotFound` otherwise). There is no bulk or `--all` form, and no flag weakens these gates.

### `nodex pools`

List resource pools. Safety: Tier 0.

```bash
nodex pools list
```

### `nodex network`

Inspect and manage network configuration.

| Command | Description | Tier |
|---------|-------------|------|
| `network show <node>` | Show node network interfaces | 0 |
| `network apply <node>` | Apply (reload) the node's pending network configuration; prints the reload task UPID. Interface changes are staged out of band (Proxmox UI/API) — the node-level endpoint accepts no interface parameters | 2 |
| `network revert <node>` | Discard the node's pending network changes | 2 |

### `nodex access`

Inspect and manage identity and access control.

**Command naming**: the plural noun is always the read-only list and the
singular noun is always the mutate verb. Proxmox VE exposes no create or delete
API for roles or groups, so `access roles` and `access groups` are read-only and
have no singular counterpart by design.

**Read-only commands** (Tier 0):

| Command | Description |
|---------|-------------|
| `access users` | List users (see [user discovery columns](#access-users-discovery-columns)) |
| `access groups` | List groups |
| `access roles` | List roles |
| `access acl list` | List ACL entries (`access acl` alone is a dispatch node) |
| `access domains` | List authentication domains |
| `access tokens list <user>` | List API tokens for a user |

#### `access users` discovery columns

`nodex access users` requests `full=1` so PVE includes group memberships and API
token details, which the plain index omits. The table gains two columns:

| Column | Meaning |
|--------|---------|
| `GROUPS` | Realm groups the user belongs to |
| `TOKENS` | Number of API tokens the user owns |

Both columns are honest about missing data. PVE only returns these fields to
callers with sufficient privileges, so a value that was not reported renders as
`-` in the table and is **omitted entirely** from JSON/YAML output:

```console
$ nodex access users
USERID       ENABLED  EMAIL  FIRSTNAME  LASTNAME  GROUPS       TOKENS  COMMENT
root@pam     yes                          -        admins,ops  2
svc@pve      no                           -        -           0
quiet@pve    no                           -        -           -
```

A reported `0` and an unreported count are genuinely different states, and the
output preserves that distinction. If a PVE release rejects `full=1`, the command
silently falls back to the plain index and those columns show `-` for everyone.

**Mutation commands** (Tier 4 — Security Admin, requires `--yes --force --expert`; `access user delete` also requires `--confirm-target <userid>`):

| Command | Description |
|---------|-------------|
| `access user create <userid> [email=<e>] [firstname=<f>] [lastname=<l>] [comment=<c>]` | Create a user (`--password-stdin` for a password) |
| `access user delete <userid>` | Delete a user |
| `access acl add <path> --role <role> [--user <id>] [--group <id>] [--propagate]` | Add an ACL entry |

### `nodex ceph`

Inspect and manage Ceph storage.

**Read-only commands** (Tier 0):

| Command | Description |
|---------|-------------|
| `ceph status <node>` | Show Ceph cluster status |
| `ceph osd list <node>` | List Ceph OSDs |
| `ceph mon list <node>` | List Ceph monitors |
| `ceph pool list <node>` | List Ceph pools |

**Mutation commands** (varies by operation):

| Command | Description |
|---------|-------------|
| `ceph osd create <node> <dev>` | Create a new OSD (`--yes --force`) |
| `ceph osd out <node> <id>` | Mark an OSD out (`--yes --force`) |
| `ceph osd in <node> <id>` | Mark an OSD in (`--yes`) |
| `ceph osd destroy <node> <id>` | Destroy an OSD (destructive; `--yes --force --confirm-target <id>`) |
| `ceph pool create <node> <name> [key=value...]` | Create a Ceph pool (`--yes --force`) |
| `ceph pool destroy <node> <name>` | Destroy a Ceph pool (destructive; `--yes --force --confirm-target <name>`) |

### `nodex replication`

Manage replication jobs.

| Command | Description |
|---------|-------------|
| `replication list` | List replication jobs |
| `replication show <id>` | Show replication job details |
| `replication create <id> <guest> <type> <target> [key=value...]` | Create a replication job (`--yes --force`) |
| `replication update <id> [key=value...]` | Update a replication job (`--yes --force`) |
| `replication delete <id>` | Delete a replication job (destructive; `--yes --force --confirm-target <id>`) |
| `replication schedule <node> <id>` | Schedule replication now (`--yes`) |

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
| `waited` | Whether Nodex waited for a provider task to finish |
| `synchronous` | Whether the provider applied the request inline without returning a task ID |
| `success` | Request acceptance when not waiting; provider task result when waiting. Independent of postcondition verification. |
| `changed` | `true`/`false` only when evidence establishes the fact; omitted when unknown |
| `status` | Provider status text; does not by itself verify the requested state |
| `verification` | `verified`, `failed`, or `unsupported` when a postcondition was checked; omitted when no check was attempted |
| `warnings` | Human-readable warnings |
| `error.class` | Error classification |
| `error.exit` | Recommended exit code |
| `error.detail` | Error detail message |

For an asynchronous mutation without `--wait`, exit code 0 means the provider
accepted the request; it does not mean the task finished. Use `--wait` to observe
the task result. Even then, task success and the requested end state are distinct:
inspect `verification` when present. `verification: "failed"` or
`verification: "unsupported"` does not rewrite a successful task result, so
automation must evaluate `success` and `verification` separately. A missing
`verification` field means no postcondition check was attempted. A successful
task may carry warnings without becoming a task failure.

A checked postcondition that contradicts the request returns exit 11 (partial
failure), including create, lifecycle, and synchronous configuration/template
operations. Task success remains a separate fact. Unavailable verification
alone does not turn a successful completed operation into a failure. Receipt
replay preserves the recorded error exit; terminal refreshed receipts do not
recommend another refresh as recovery.

`profile test` and `storage download` honor JSON and YAML output. Profile tests
return connectivity/version fields; downloads return node, storage, volume ID,
destination, and size. Container inventory includes `template`; `vm show`
includes `qmpstatus` when authoritative runtime inspection is available,
separately from the process `status`.

`node certificates` reads certificate details, including subject, issuer,
fingerprint, and integer Unix timestamps `not_before`/`not_after`; tables format
the timestamps for people. `sdn subnets` enumerates subnets under each vnet.
`backup content` includes only backups, with guest ID, subtype, and creation time.
Provider inventory lists such as node services honor `--limit`; agent output
marks a reached limit as potentially truncated, without promising completeness.

Backup job create/update accepts `schedule=<calendar>` (quote values containing
spaces), or legacy `starttime=<HH:MM>` with optional `dow`. Do not combine
`schedule` and `starttime`. Job list/show exposes `schedule` and `next-run`
when supplied by the provider, with legacy timing as a table fallback.

Postconditions are emitted only when the provider exposes a distinct state to
observe. For example, Proxmox's LXC `status/current` reports only `running` or
`stopped`, so Nodex does not claim a separate postcondition for an LXC suspend.

JSON and YAML mutation envelopes are written to stdout. Progress messages,
prompts, and human-readable diagnostics are written to stderr; table output is
for terminal use and is not a scripting contract. The `operation list` and
`operation describe` commands report static implementation support separately
from permissions and readiness, which are only determined during execution.

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
| 11 | Partial or indeterminate failure (for example a checked postcondition mismatch, an environment health result that is not healthy, or a multi-profile `--all` run) |
| 12 | Provider error |
| 13 | Not found |
| 14 | Timeout |
| 15 | Cancellation |
| 16 | Task failure |
| 17 | Validation error |
| 18 | Ambiguous outcome |
| 19 | Rate limit |
| 20 | Output error |
| 21 | Conflict |
| 130 | Interrupted (SIGINT) |
| 143 | Terminated (SIGTERM) |

## Signals and Cancellation

The entry point listens for SIGINT and SIGTERM. On receipt, Nodex cancels the command context. If an error is returned after cancellation, the process exits with 130 for SIGINT or 143 for SIGTERM.

## Error Output

Errors are printed to stderr as `Error: <message>`. The message is passed through redaction and terminal sanitization before printing. When `--output json` is requested, errors are written as structured JSON.
