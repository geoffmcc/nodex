# Fleet Operations Roadmap

Tracked implementation roadmap for the fleet-operations expansion described in
[ADR 0001](adr/0001-fleet-operations-architecture.md). Each phase is delivered
on its own branch and pull request, keeps `main` releasable, and updates
documentation in the same phase as behavior. This file is updated as phases
complete; do not delete remaining phases when one finishes.

Status legend: `done` merged to main · `active` in progress · `planned` not
started.

## Phase 1 — Architecture, configuration schema v2, provider foundation

Status: done (merged 2026-07-17, PR #64)

- ADR 0001 and this roadmap.
- Configuration schema version 2 with a backward-compatible loader:
  versions 1–2 accepted, file schema version preserved on read-modify-write
  (no silent rewrites), newer-than-supported versions rejected with an
  "upgrade nodex" error.
- Known-provider model: `proxmox` (Proxmox VE, unchanged) and `pbs` (Proxmox
  Backup Server, reserved here, implemented in Phase 2). Per-provider profile
  validation; CLI rejects unknown providers at entry, config files tolerate
  them until use.
- `nodex profile add --provider <proxmox|pbs>`.
- `PBSAPIToken` and `PBSAuthCookie` redaction patterns with table-driven and
  fuzz tests.
- Documentation: configuration schema v2, compatibility policy, ADR/roadmap
  links.

Explicitly deferred from this phase: an explicit `nodex config migrate`
command (lands with the first version-2-only section); any PBS commands.

## Phase 2 — Proxmox Backup Server provider

Status: read-only foundation merged 2026-07-17 (PR #65); guarded mutations
(verify/sync/prune/GC runs with safety gates and conflicting-task preflight)
active on this branch.

- `internal/provider/pbs/` with its own typed client (`/api2/json`, port 8007
  default), `PBSAPIToken` authorization, provider registration as `pbs`.
- PBS-specific domain/output models using real PBS terminology (datastores,
  backup groups/snapshots, namespaces, verify/prune/sync jobs, GC).
- Read-only commands: `pbs status|version|subscription|certificates`,
  `pbs datastore list|show`, `pbs snapshot list`, `pbs task list|show|log`,
  `pbs verify list`, `pbs prune list`, `pbs sync list`,
  `pbs garbage-collection status`.
- Full per-command contract: typed capability interfaces, request/response
  objects, capability identifiers, table/JSON/YAML parity, stable empty-list
  behavior, exit codes, pagination, cancellation, timeouts, body limits,
  redaction, sanitization; unit, mock-provider CLI, and HTTP contract tests
  with fictional endpoints only.
- Then, individually gated through the product decision gate: `pbs verify
  run`, `pbs sync run`, `pbs prune run`, `pbs garbage-collection run` —
  data-removing operations classified destructive unless provider semantics
  justify otherwise. Never tested against live infrastructure.

## Phase 3 — Unified PVE/PBS environment backup health

Status: planned

- Version-2-only `environments` config section grouping a PVE and a PBS
  profile.
- `BackupHealthReader` service over capability interfaces (no concrete client
  dependencies).
- `nodex environment list|health|backup-health` with healthy / warning /
  blocked / unknown / unsupported / partial-failure semantics; never
  "healthy" when required data is missing.
- Configurable stale-backup thresholds per environment or role.

## Phase 4 — Linux fleet inventory and Ansible execution boundary

Status: planned

- Version-2-only `inventory` section: explicit host enrollment (address,
  role, environment, profile links, SSH user/port/key-file reference,
  maintenance group, criticality, `backup_required`,
  `automatic_reboot: false` default). No secrets in config; host-key
  verification required.
- Allowlisted operation registry (`check-updates`,
  `configure-security-updates`, `install-security-updates`,
  `install-approved-updates`, `restart-approved-services`,
  `reboot-approved-host`, `verify-host`, `verify-service`).
- Shell-free Ansible adapter: version detection, minimal environment, private
  temp dirs, unsafe-path rejection, bounded time/output, cancellation, child
  termination, separated streams, redaction, per-host structured results,
  explicit partial failure. Debian/Ubuntu only initially. Ansible remains
  optional for all PVE/PBS functionality.

## Phase 5 — Maintenance status and immutable planning

Status: planned

- `nodex maintenance inventory|status|plan` (read-only) with
  `--environment/--group/--host/--role` filters.
- Preflight checks: reachability, SSH auth/host-key, OS, package manager,
  APT locks, repository health, available/security updates, held packages,
  broken dependencies, disk space, reboot-required, failed units, PVE/PBS
  health and active tasks, backup coverage and verification recency,
  configured app checks, block state.
- Serializable plans: ID, timestamps, expiry, targets, operation, package
  intent, required backup state, preflight results, ordering, batch size,
  reboot policy, safety classification, warnings, blockers, cryptographic
  digest. Deterministic, secret-free, expiring, tamper-evident.

## Phase 6 — Maintenance apply, verification, reporting

Status: planned

- `nodex maintenance apply|verify|report`.
- Apply rejects stale/modified/expired plans, materially changed
  infrastructure, unmet backup requirements, conflicting PVE/PBS tasks,
  unreachable critical dependencies. Never regenerates a plan silently.
- Policies: `security-only`, `approved-full-upgrade` (current release and
  repositories only). No automated distribution/release/major upgrades.
- Reboots: off by default everywhere; explicit plan content + confirmation +
  disruptive gate + verified reboot-required + sequencing + post-reboot
  verification. PVE/PBS/primary DNS never auto-reboot.
- Ordering: serial for critical hosts, bounded concurrency for guests, never
  PVE and PBS together, never the only DNS server with other critical infra.
- Secret-free reports with per-host detail and honest partial-failure
  disposition.

## Phase 7 — Security-update policy workflow

Status: planned

- `nodex maintenance policy plan|apply` for unattended security updates on
  Debian/Ubuntu guests: opt-in, exact config diff shown, security repos only
  by default, automatic reboot disabled, no PVE/PBS/primary-DNS enrollment by
  default, preserves admin customizations, backs up changed files, validates
  after write, reports timer/service state, idempotent, normal confirmation
  policy, safe removal/restore procedure.

## Phase 8 — One-shot monitoring and external integration

Status: planned

- Version-2-only `monitoring` config section; `nodex monitor targets|check`
  with `--environment/--target` filters.
- Checks: HTTP(S) status, TCP, TLS validity/expiry, DNS (with specified
  resolver), PVE/PBS API health, task failures, datastore capacity, backup
  age, systemd service status via the read-only Ansible path. No ICMP
  initially.
- Bounded concurrency, per-check timeouts, table/JSON/YAML, healthy /
  degraded / failed / unknown / unsupported, partial-failure exit codes.
- Documentation for external integration: cron/systemd timer invocation, exit
  code interpretation, feeding Pulse/Uptime Kuma/Prometheus, examples for
  PVE, PBS, DNS, Samba, Jellyfin, and a generic HTTP service — fictional
  addresses only. No daemon.
