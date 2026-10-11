# Nodex Operator Guide

This guide explains how Nodex fits together and how to use its major workflows
safely. For exact arguments and per-command options, use the [CLI reference](cli-reference.md)
and the CLI's own `--help` output. For machine-consumable discovery, use the
[agent interface](agent-interface.md) and the live operation registry.

## What Nodex is

Nodex is a local Go command-line program that connects directly to explicitly
configured infrastructure. Its built-in providers are Proxmox VE (`proxmox`)
and Proxmox Backup Server (`pbs`). It also has optional, explicitly enrolled
Linux-host maintenance through a tightly allowlisted Ansible boundary. It does
not install an agent on managed hosts or run a daemon. It does not continuously
monitor or retain monitoring history; `monitor check` runs once and exits.

The major capability areas are:

- **Proxmox VE:** nodes; VMs and LXC containers; storage; tasks, events and
  logs; backups and schedules; cluster and HA state; firewall; network apply or
  revert; SDN; Ceph; access and ACL inspection/management; resource pools; and
  replication.
- **Proxmox Backup Server:** server and datastore inspection; backup snapshots,
  namespaces, tasks and logs; verify/prune/sync job configuration and
  garbage-collection status; and confirmation-gated verify, sync, prune, and
  garbage-collection runs.
- **PVE/PBS environments:** combined reachability, datastore/task state and
  guest backup coverage, age and verification checks. Unknown or unavailable
  evidence stays unknown; it is never silently counted as healthy.
- **Linux fleet maintenance:** explicit inventory, read-only preflight,
  immutable expiring plans, guarded apply, durable receipts, verification,
  reconciliation and reporting. Requires Ansible and enrolled hosts.
- **One-shot monitoring:** configured HTTP(S), TCP, TLS and DNS checks, plus
  configured PVE/PBS health, task, datastore and backup checks. Results are
  structured for an external scheduler or monitoring system.
- **Automation:** stable JSON/YAML interfaces, command-operation discovery,
  and an opt-in agent execution contract with mutation receipts and
  deduplication.

## First connection

Install a release binary for your operating system, or build from source as
described in the [README](../README.md). Then create a profile using the guided
setup command:

```sh
nodex setup
```

For a scripted or carefully reviewed setup, initialize config and define the
profile in `config.yaml` (see [Configuration](configuration.md)). Keep the
secret out of the file and shell history. For example, a profile can reference
environment variables:

```yaml
version: 2
current_profile: lab
profiles:
  lab:
    provider: proxmox
    endpoint: https://pve.example.com:8006
    credential_ref: env:lab
```

```sh
export NODEX_LAB_TOKEN_ID='automation@pam!nodex-readonly'
export NODEX_LAB_TOKEN_SECRET='example-token-secret'
nodex profile test lab
nodex doctor
nodex --profile lab node list
```

Use a read-only API token for inspection. Add only the privileges needed for
operations you intend to perform; the provider's own authorization remains
decisive. PVE and PBS use different token schemes and should have separate
profiles and credentials.

## Select a target deliberately

Profiles bind a provider, HTTPS endpoint, trust settings and a credential
reference. `--profile <name>` overrides `current_profile`; for scripts and
agent-mode operations, specify the profile explicitly. Do not treat a profile
name as proof that a user or token has permission to perform an action.

Most guest commands address a resource as `<node>/<vmid>`, for example:

```sh
nodex --profile lab vm show pve-a/100
nodex --profile lab container config pve-a/200
```

Use `nodex profile test <name>` to test connectivity. Use
`nodex profile diagnose-permissions <name>` for bounded provider-supported
permission diagnostics; unsupported checks remain explicitly unsupported.

`--all` is intentionally narrow: it aggregates only `status`, `node list`,
`vm list` and `container list` across configured profiles. It does not run
mutations or fan out every command.

## Read, diagnose and check state

Start with narrow observations. Common entry points include:

```sh
nodex --profile lab status
nodex --profile lab node list
nodex --profile lab vm list
nodex --profile lab container list
nodex --profile lab storage list
nodex --profile lab task list pve-a
nodex --profile lab doctor
```

`doctor` checks local configuration and connectivity across configured
profiles. `status` summarizes the selected PVE target. On a standalone PVE
host, cluster quorum/HA is reported as not applicable; if cluster state cannot
be read, it is reported as unavailable rather than inferred to be standalone.

For backups, create a PBS profile and an `environments` entry linking PVE/PBS
profiles. Then use:

```sh
nodex --profile backup pbs datastore list
nodex --profile backup pbs snapshot list --datastore backups
nodex environment health homelab
nodex environment backup-health homelab
```

Environment backup health checks every PVE guest unless its VMID is explicitly
excluded. Thresholds for backup age, verification age and datastore usage are
configurable. Inspect per-check status and blockers, not only the summary.
Statuses distinguish healthy, warning, blocked, unknown and unsupported.

## Understand safety gates before mutations

Every state-changing operation declares its safety tier and confirmation
requirements. The operation registry and detailed command help are the
authoritative source for a specific command:

```sh
nodex operation list --output json
nodex operation describe "vm migrate" --output json
nodex help vm migrate
```

| Tier | Meaning | Typical acknowledgement |
|---|---|---|
| 0 | Observation | None |
| 1 | Reversible | `--yes` |
| 2 | Disruptive | `--yes --force` |
| 3 | Destructive | Required flags plus exact `--confirm-target` in non-interactive use |
| 4 | Security administration | `--expert`; additional confirmations may also apply |

The operation contract may require multiple controls. `--force` does not
replace `--yes`; `--expert` does not bypass other requirements. Interactive
confirmation is available in the normal CLI, while non-interactive commands
fail closed if an acknowledgement is missing. These flags record intent; they
do not prove a separate human approved an automated request.

An operation being implemented does not mean the selected server supports it,
the token is authorized, or the target is currently ready. Nodex checks runtime
capabilities/permissions through the existing command path. Provider-specific
limitations are documented in `operation describe` and the CLI reference.

## Track asynchronous work and verify outcomes

Many Proxmox mutations return a task UPID. Without `--wait`, success means the
provider accepted the request; it does not mean the task completed. With
`--wait`, Nodex polls the task, but task completion and the requested resource
state are still separate evidence. Where Nodex can perform a meaningful
read-back, it reports verification separately.

```sh
nodex --profile lab --yes --wait vm shutdown pve-a/100
nodex --profile lab task list pve-a
nodex --profile lab task show pve-a '<UPID>'
```

If the client times out or is interrupted after submission, the remote task
may continue. Do not blindly repeat an ambiguous mutation. Inspect its UPID or
use the operation-specific recovery workflow. Maintenance and certification
have their own durable receipts/ledgers; agent-mode mutations use agent
receipts. These workflows are not interchangeable.

## Manage an explicitly enrolled Linux fleet

Fleet maintenance uses the schema-v2 `inventory` section and SSH host-key
verification. Each managed host must be enrolled; discovering a PVE guest does
not implicitly authorize SSH management. Ansible (`ansible-playbook`,
ansible-core 2.12 or later) is an optional local dependency required for these
maintenance operations. Nodex embeds the
allowlisted playbooks and does not expose arbitrary shell, Ansible modules or
playbook paths.

The intended flow is inspect → plan → review → apply → verify/report:

```sh
nodex maintenance inventory
nodex maintenance status --environment homelab
nodex --output json maintenance plan --policy security-only --environment homelab > plan.json
nodex maintenance verify --plan plan.json
nodex --yes --force --confirm-target '<plan-id>' maintenance apply --plan plan.json
nodex maintenance report --receipt '<receipt-path>'
```

Plans are expiring and digest-protected; applying requires a valid, unexpired,
unblocked plan and the usual safety acknowledgements. Supported plan policies
are `security-only` and `approved-full-upgrade`. The current plan workflow sets
reboot policy to `never`; it does not automatically reboot hosts. Backup
requirements cannot be established without a configured PVE/PBS environment.
Read [Fleet Maintenance](maintenance.md) before applying a plan.

The separate `container os-update` operation only supports
`--policy approved-full-upgrade`, requires a running enrolled LXC and its exact
PVE execution host in inventory, performs before/apply/after evidence checks,
and does not reboot the guest.

### Configure unattended security updates

This is distinct from a one-shot `maintenance plan --policy security-only` run.
To opt an eligible Debian/Ubuntu guest into unattended security updates, set
`unattended_security_updates: true` on its inventory entry, then create and
review a policy plan:

```sh
nodex --output json maintenance policy plan --host web-guest > policy.json
plan_id=$(jq -r '.plan_id' policy.json)
nodex --yes --force --confirm-target "$plan_id" maintenance policy apply --plan policy.json
```

Plans show the exact Nodex-owned APT drop-in, current security origins, and
timer state. PVE, PBS, and DNS hosts remain excluded. Apply preserves other
administrator-owned APT files, enables only security origins, leaves automatic
reboot disabled, and verifies the timer and effective configuration. Applying
the policy installs/configures the unattended-upgrade mechanism; it does not
run an OS upgrade in the same transaction. Keep `policy.json` so the prior
Nodex-managed file and timer state can be restored later:

```sh
nodex --yes --force --confirm-target "$plan_id" maintenance policy restore --plan policy.json
```

Restoration refuses if the managed file or its retained host-side backup has
changed. It leaves the `unattended-upgrades` package installed and retains the
mode-restricted backup for operator review.

## Run one-shot checks

Monitoring targets are explicit entries in schema-v2 configuration; Nodex
does not discover targets or run in the background:

```sh
nodex monitor targets
nodex --output json monitor check
nodex --output json monitor check --target pbs-backups
```

Targets can be filtered by name or environment. General checks include HTTP,
HTTPS, TCP, TLS, DNS, ICMP and enrolled-host systemd service state. Service
targets name an inventory host and `.service` unit and require Ansible.
Provider-backed check types cover PVE/PBS APIs and tasks, datastore state,
backup age, verification and coverage. They require a matching configured
environment and profile. Checks are bounded and run once. Healthy overall
results exit 0; any non-healthy state, including unknown or unsupported, exits
11. Configuration and usage errors use their own exit codes.

Nodex does not schedule checks, retain history, serve a metrics endpoint, or
send alerts. Invoke it from cron/systemd or an external monitoring system:

```cron
*/5 * * * * /usr/local/bin/nodex --output json monitor check --environment homelab >> /var/log/nodex-monitor.jsonl 2>&1
```

Alternatively, a systemd timer can invoke the same one-shot command. The
service runs as an unprivileged account whose Nodex config contains the
explicit monitoring targets:

```ini
# /etc/systemd/system/nodex-monitor.service
[Unit]
Description=Run Nodex one-shot monitoring checks

[Service]
Type=oneshot
User=nodex-monitor
ExecStart=/usr/local/bin/nodex --output json monitor check --environment homelab
```

```ini
# /etc/systemd/system/nodex-monitor.timer
[Unit]
Description=Run Nodex monitoring checks every five minutes

[Timer]
OnBootSec=2min
OnUnitActiveSec=5min
Persistent=true

[Install]
WantedBy=timers.target
```

Pulse and Uptime Kuma can consume a wrapper's exit status or parsed JSON; the
wrapper should report exit 11 as degraded/unhealthy, not success. Prometheus
can scrape a textfile-exporter metric written by an external wrapper, while the
JSON report remains available for per-target detail:

```sh
set +e
nodex --output json monitor check --environment homelab > /tmp/nodex-monitor.json
status=$?
if [ "$status" -eq 0 ]; then healthy=1; else healthy=0; fi
printf 'nodex_monitor_healthy{environment="homelab"} %s\n' "$healthy" \
  > /var/lib/node_exporter/textfile_collector/nodex.prom.tmp
mv /var/lib/node_exporter/textfile_collector/nodex.prom.tmp \
  /var/lib/node_exporter/textfile_collector/nodex.prom
exit "$status"
```

For example, explicitly configured targets can cover PVE/PBS APIs and tasks,
DNS resolution, ICMP reachability, Samba's `smbd.service` on an enrolled host,
Jellyfin's HTTPS health endpoint, and a generic HTTP service. Use fictional
addresses in copied examples and do not put credentials in monitoring URLs.

## Automate with JSON or the agent contract

For shell scripts, prefer `--output json` or `--output yaml`; table formatting
is for people and can change. Global flags may appear anywhere in the command
line. JSON/YAML keep requested data on stdout and diagnostics on stderr.

For an AI/automation caller that needs per-operation schemas and mutation
deduplication, use the opt-in agent interface:

```sh
nodex agent contract --output json
nodex operation list --output json
nodex operation describe "pbs verify run" --output json
```

Agent-mode remote operations require an explicit profile and a unique request
ID for mutations. The agent does not bypass normal confirmation, TLS or SSH
trust. A repeated identical request ID returns the recorded result without
resubmitting. For running or unknown outcomes, refresh/reconcile the receipt;
never automatically retry an ambiguous mutation. Agent mode is a structured
CLI contract, not a sandbox or proof of human authorization. See
[`NODEX_AGENT.md`](../NODEX_AGENT.md) and the [full interface reference](agent-interface.md).

## Learn more

- [CLI reference](cli-reference.md) — command groups, flags, output, safety and exit codes
- [Configuration](configuration.md) — profiles, credentials, environments, inventory and monitoring
- [Architecture](architecture.md) — components, providers, trust and runtime behavior
- [Agent interface](agent-interface.md) — schemas, support boundaries and receipts
- [Security policy](../SECURITY.md) and [threat model](threat-model.md)
- [Fleet maintenance](maintenance.md) and [certification](certification.md)
