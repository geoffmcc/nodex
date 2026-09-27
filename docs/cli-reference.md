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

## Mutation Flags

| Flag | Description |
|------|-------------|
| `--yes` | Confirm reversible operations (Tier 1) |
| `--force` | Confirm disruptive operations (Tier 2, requires `--yes`) |
| `--wait` | Wait for provider task to complete before exiting |
| `--expert` | Enable expert-mode operations (Tier 4: identity, ACL changes) |
| `--password-stdin` | Read password from stdin instead of interactive prompt |
| `--confirm-target <text>` | Exact target text for non-interactive destructive confirmation |

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
| `vm stop <id>` | Stop a VM (force) |
| `vm shutdown <id>` | Graceful VM shutdown (60s timeout) |
| `vm suspend <id>` | Suspend a VM to disk |
| `vm resume <id>` | Resume a suspended VM |
| `vm pause <id>` | Pause (freeze) a VM |
| `vm unpause <id>` | Unpause a frozen VM |

**Disruptive commands** (Tier 2, requires `--yes --force`):

| Command | Description |
|---------|-------------|
| `vm reset <id>` | Hard reset a VM |
| `vm reboot <id>` | Reboot a VM |
| `vm migrate <id> --target <node>` | Migrate VM to another node |
| `vm create <node> <vmid> [name] [iso] [disk-storage]` | Create a minimal VM |

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
| `container stop <id>` | Stop a container (force) |
| `container shutdown <id>` | Graceful container shutdown |
| `container suspend <id>` | Suspend a container |
| `container resume <id>` | Resume a suspended container |

**Disruptive commands** (Tier 2, requires `--yes --force`):

| Command | Description |
|---------|-------------|
| `container reboot <id>` | Reboot a container |
| `container migrate <id> --target <node>` | Migrate container |
| `container create <node> <vmid> <ostemplate> [hostname] [storage]` | Create a container from an OS template |
| `container restore <node> <vmid> <archive> [storage]` | Restore a container from a backup archive |

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
| `container snapshot <action> <id> [args]` | Create, delete, or rollback snapshots |
| `container clone <id> --newid <id>` | Clone a container |

`container os-update` is a disruptive operation and requires `--yes --force`.
It only targets an explicitly identified running LXC, executes the fixed APT
full-upgrade procedure through the enrolled PVE host's Ansible connection, and
never reboots the guest. Nodex verifies that the guest remains running and has
no remaining APT or package-database issues after the update. VM operating
system updates are not provided by this command.

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
| `--last <n>` | 50 | Show only the `n` most recent entries. `--last 0` removes the cap. |
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
| `sdn zone create <type> <zone>` | Create an SDN zone |
| `sdn zone delete <zone>` | Delete an SDN zone |
| `sdn vnet create <vnet> --zone <zone>` | Create an SDN VNet |
| `sdn vnet delete <vnet>` | Delete an SDN VNet |
| `sdn subnet create <vnet> <cidr> <gateway>` | Create an SDN subnet |
| `sdn subnet delete <vnet> <subnet>` | Delete an SDN subnet |
| `sdn controller create <ctrl>` | Create an SDN controller |
| `sdn controller delete <ctrl>` | Delete an SDN controller |

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
```

| Command | Description |
|---------|-------------|
| `maintenance inventory` | List enrolled hosts with role, environment, group, criticality, backup requirement, and reboot policy |
| `maintenance status` | Run the read-only `check-updates` preflight through the allowlisted Ansible boundary: pending updates, security updates, reboot-required state, failed units, root filesystem usage per host. With `--environment`, adds the environment's backup health. Exits 11 on partial failure. |
| `maintenance plan` | Run the same preflight and emit an immutable plan: plan ID, creation/expiry timestamps (default TTL 4h), update policy, per-host package intent, execution order (standard hosts first, critical hosts serial, PVE/PBS/DNS roles last), batch size, reboot policy (always `never` in this phase), backup requirements and their observed state, infrastructure snapshot, warnings, blockers, and a SHA-256 digest over the whole plan. Save it with `--output json > plan.json`. |
| `maintenance apply` | Apply an existing digest-verified plan with `--yes --force --confirm-target <plan-id>`. Writes atomic receipts and refuses blocked, stale, tampered, or ambiguous reruns. |
| `maintenance resume` | Revalidate a plan-bound receipt and continue only hosts not yet started; refuses to replay non-successful hosts. |
| `maintenance reconcile` | Perform read-only postcondition verification against a plan-bound receipt. |
| `maintenance abandon` | Record an explicit operator decision that an interrupted receipt will not be resumed. |
| `maintenance verify` | Verify planned hosts through the embedded read-only Ansible operation. |
| `maintenance report` | Render a verified receipt as table, JSON, or YAML. |

Plans are tamper-evident (any modification breaks the digest), expiring, deterministic for unchanged inputs, and contain no secrets. A plan created with blockers is still emitted for review but apply refuses it. Backup requirements can only be verified when `--environment` links the hosts to a PVE/PBS pair; without it, `backup_required` hosts are a blocker by design.

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
| `access acl` | List ACL entries |
| `access domains` | List authentication domains |
| `access tokens <user>` | List API tokens for a user |

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
