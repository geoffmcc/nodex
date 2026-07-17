# ADR 0001: Fleet Operations Architecture

- Status: Accepted
- Date: 2026-07-17
- Scope: Multi-provider expansion (PVE + PBS), Linux fleet inventory, Ansible
  execution boundary, maintenance planning, backup-aware safety, one-shot
  health checks, reporting, and the phased compatibility strategy.

## Context

Nodex is a security-focused, Proxmox VE-focused CLI. It is being expanded into
a Proxmox-aware operations interface covering Proxmox VE (PVE), Proxmox Backup
Server (PBS), Debian/Ubuntu hosts (bare metal, VMs, LXCs, Raspberry Pi),
backup-aware system maintenance, one-shot infrastructure health checks, and
safe orchestration of approved Ansible operations.

Nodex must remain a local CLI. It must not become a daemon, web dashboard,
time-series database, monitoring server, generic SSH runner, raw API executor,
or an Ansible replacement. Continuous monitoring stays with external systems
(Pulse, Uptime Kuma, optionally Prometheus/Grafana); Nodex supplies one-shot
checks, structured output, orchestration, verification, and reporting.

Current architecture facts this ADR builds on (verified against `main` at the
time of writing):

- One provider (`proxmox`) registered through `internal/provider/registry.go`;
  capability discovery is structural (declared `domain.Capability` identifiers
  backed by optional Go interfaces asserted at runtime).
- Configuration schema version 1 (`internal/config`): profiles with
  `provider`, `endpoint`, `credential_ref`, `ca_file`. Validation hard-rejects
  any other schema version; no migration path exists.
- Transport (`internal/transport/httpclient`): HTTPS only, TLS 1.2 minimum,
  certificate and hostname verification always on, additive custom CA support,
  bounded retries for idempotent requests only, response body limits. There is
  no insecure TLS override anywhere in code or configuration, and none will be
  added.
- Credentials (`internal/credentials`): keyring/file/env/stdin backends,
  provider-neutral token model (`TokenID` + `TokenSecret`), strict
  `credential_ref` parsing. Secrets never live in the config file.
- Safety (`internal/safety`): five tiers (observation, reversible, disruptive,
  destructive, security_admin) with a confirmation policy that fails closed in
  non-interactive mode.
- Redaction (`internal/redact`): type-based `Secret`/`Redactable` sanitization
  plus regex defense-in-depth applied to free-text output and provider error
  bodies; terminal sanitization on all output.

## Decisions

### D1: PBS is a separate first-class provider

PBS is implemented as its own provider package (`internal/provider/pbs/` with
its own typed client), registered as provider name `pbs`. The existing PVE
client gains no PBS conditionals. Provider naming is stable: `proxmox` keeps
its current meaning (Proxmox VE); `pbs` is reserved for Proxmox Backup Server.

PBS authentication uses the `PBSAPIToken=user@realm!tokenname:secret`
authorization scheme (note PBS separates token id and secret with `:` where
PVE uses `=`), reusing the provider-neutral token credential model. PVE and
PBS credentials are always separate credential store entries referenced by
separate profiles. Both `PVEAPIToken` and `PBSAPIToken` values are redacted by
the regex defense-in-depth layer in addition to type-based redaction.

PBS resources (datastores, backup snapshots/groups, verify/prune/sync jobs,
garbage collection) get PBS-specific domain and output models using real PBS
API terminology. They are not forced into PVE-shaped models. PBS tasks use
Proxmox-style UPIDs, so the existing task polling model is reused.

PBS mutations (verify run, sync run, prune run, GC run) come only after the
read-only foundation, each individually passed through the product decision
gate, with operations that can remove backup data classified as destructive
unless provider semantics justify otherwise.

### D2: Configuration schema version 2 with a backward-compatible loader

`CurrentSchemaVersion` becomes 2; the loader accepts versions 1 through 2
(`MinSupportedSchemaVersion` = 1). Version 1 files continue to load and
validate with unchanged semantics. Nodex never silently rewrites a
configuration file: reading does not write, and read-modify-write operations
(`profile add`, `profile use`, ...) preserve the file's existing schema
version. A file is only written as version 2 when it was created as version 2
(`nodex init`) or when the user explicitly adopts a version-2-only feature.
Configs with a version newer than the binary supports are rejected with a
clear "upgrade nodex" error rather than being partially interpreted.

Schema version 2 is the vehicle for the multi-provider era: profiles may
declare `provider: proxmox` or `provider: pbs` (validated per provider type),
and later phases add optional version-2-only sections (`environments`,
`inventory`, `maintenance`, `monitoring`). Adding a profile with an unknown
provider through the CLI is rejected with the known-provider list; an unknown
provider name already present in a config file remains structurally valid
(name-shape validation only) and fails with a clear "unknown provider" error
only when a command actually uses that profile. This keeps older Nodex
binaries from bricking an entire config that a newer binary wrote, without
weakening runtime validation.

Endpoint policy is unchanged and applies to all providers: HTTPS only, no
credentials in URLs, no query/fragment/path, certificate and hostname
verification always on, per-profile additive `ca_file`. There is no
`--insecure`, `skip_verify`, or equivalent, and none will be introduced.

### D3: Environments compose profiles; a backup-health service composes providers

An `environments` section (version-2-only, later phase) groups one PVE profile
and one PBS profile under a name (e.g. `homelab`). A higher-level
backup-health service consumes provider capability interfaces — never concrete
clients — behind an interface of the shape
`CheckEnvironmentBackupHealth(ctx, BackupHealthRequest) (BackupHealthResult, error)`,
and answers: reachability and health of PVE and PBS, datastore availability
and capacity, running/failed backup-chain tasks, per-guest backup recency and
verification recency, and whether maintenance is safe to begin. Results
distinguish healthy / warning / blocked / unknown / unsupported and partial
failure; "healthy" is never reported when required data could not be
retrieved. Stale-backup thresholds are configurable per environment or host
role.

### D4: Linux fleet management requires explicit inventory enrollment

A declarative `inventory` section lists SSH-manageable hosts with role,
environment, optional PVE/PBS profile links, SSH user/port/key-file
references, maintenance group, criticality, `backup_required`, and
`automatic_reboot` (default false). Proxmox discovery may suggest candidates
but never auto-enrolls a guest: SSH management exists only for explicitly
declared hosts. Inventory stores no secrets — no private keys, passwords,
vault passwords, or sudo passwords; SSH uses agent or key-file references with
host-key verification required (configurable `known_hosts` path).

### D5: Ansible is the execution engine behind a narrow allowlisted adapter

Linux package maintenance executes through Ansible, invoked by a Nodex adapter
that only runs operations from an allowlisted registry (e.g. `check-updates`,
`install-security-updates`, `install-approved-updates`,
`restart-approved-services`, `reboot-approved-host`, `verify-host`,
`verify-service`). Users select operation identifiers; Nodex never accepts
arbitrary shell commands, modules, playbook paths, inventory scripts, callback
plugins, environment variables, or extra CLI arguments, and no
`host exec`/`ssh --command` style command will exist.

The adapter uses `exec.CommandContext` (never a shell), resolved absolute
executable paths, a minimal allowlisted environment, private restrictive-mode
temp directories, symlink and world-writable-path rejection, bounded execution
time with cancellation and safe child termination, separated and
size-bounded stdout/stderr, terminal sanitization and secret redaction on all
captured output, cleanup on success and failure, and per-host structured
results — success is never inferred from process exit alone, and partial host
failure is an explicit distinct outcome. Ansible remains an optional external
dependency: all PVE/PBS inspection works without it. Initial scope is
Debian/Ubuntu package maintenance only.

### D6: Maintenance is plan-then-apply with immutable, expiring plans

`maintenance status` and `maintenance plan` are strictly read-only.
`maintenance plan` produces a serializable plan: plan ID, creation and
expiration timestamps, environment, target hosts, operation, package intent,
required backup state, preflight results, required service checks, host
ordering, batch size, reboot policy, safety classification, warnings,
blockers, and a cryptographic digest over the security-relevant contents.
Plans contain no secrets, are deterministic for unchanged inputs, expire, and
are rejected on tampering (digest mismatch), material infrastructure state
change, unmet backup requirements, new conflicting PVE/PBS tasks, or a
newly-unreachable critical dependency. `maintenance apply` executes exactly
the reviewed plan — it never silently regenerates a different one.

Update policy defaults are conservative: `security-only` and
`approved-full-upgrade` within the current distribution release and
repositories only. No distribution/release upgrades, repository changes, or
PVE/PBS major upgrades are automated. Reboots default to off for every role
and require explicit plan content, explicit confirmation at the disruptive
tier, verified reboot-required state, no conflicting tasks, dependency-aware
sequencing, and post-reboot reachability and service verification. PVE, PBS,
and the primary DNS server never reboot merely because `reboot-required`
exists. Execution is serial for critical hosts, bounded-concurrency for
ordinary guests, never updates PVE and PBS simultaneously, never updates the
only DNS server concurrently with other critical infrastructure, and stops or
pauses on critical failure. No transactional-rollback claims are made.

### D7: Backup-aware safety gates maintenance

Hosts marked `backup_required` are not updated unless a sufficiently recent
successful backup exists on PBS and no conflicting backup/datastore task is
active. The unmet case refuses by default; any override is narrow, explicit,
strongly gated, and prominently audited. Optional pre-maintenance snapshots,
if later added, use provider-native typed methods with task tracking and
verification, and never auto-delete recovery points.

### D8: Monitoring is one-shot and configuration-driven

`nodex monitor check` runs configured checks once and exits: HTTP/HTTPS
status, TCP connectivity, TLS certificate validity/expiry, DNS resolution
(optionally via a specified resolver), PVE/PBS API health, task-failure
status, datastore capacity thresholds, backup-age thresholds, and systemd
service status via the approved read-only Ansible path. No ICMP initially.
Checks use bounded concurrency and per-check timeouts, produce
table/JSON/YAML with healthy / degraded / failed / unknown / unsupported
statuses, return partial-failure exit codes, and never mix logs into
structured output or leak sensitive URLs/credentials. Continuous execution,
history, and alerting belong to external schedulers (cron/systemd timers) and
monitoring systems (Pulse, Uptime Kuma, Prometheus); documentation covers that
integration. No scheduling daemon will be added.

### D9: Reporting is complete and secret-free

Maintenance runs record: run ID, plan ID and digest, timestamps, Nodex
version/commit, Ansible version, targets, per-host preflight results and
actions, changed/held/skipped packages, reboot status, service verification,
PVE/PBS task references, warnings, errors, partial failures, and final
disposition. Reports never contain tokens, passwords, private keys,
authorization headers, cookies, vault secrets, environment-secret values, or
raw command lines containing secrets; child-process output passes through
redaction before display or storage. Partial success is never reported as
complete success.

### D10: The security model extends, never weakens

All existing invariants carry over to every new surface: separate credential
stores per system; no secrets as CLI arguments, in config, plans, or reports;
HTTPS/TLS 1.2+ with full verification and additive CA only; no insecure
bypass flags of any kind (`--skip-safety`, `--assume-safe`, `--trust-all`,
`--insecure` and equivalents are permanently rejected); bounded retries for
safe GETs only, never mutations; shell-free process execution; safety-tier
metadata, confirmation policy, capability requirements, idempotency
expectations, and explicit exit codes declared for every operation;
non-interactive mode fails closed; auditable output that distinguishes
requested/checked/changed/unchanged/failed.

## Phased delivery and compatibility strategy

Work lands as independently reviewable phases, each on its own branch and PR,
each keeping `main` releasable, with documentation updated in the same phase
as behavior. The tracked roadmap lives in `docs/roadmap.md` and is updated as
phases complete. Sequence (details in the roadmap):

1. Architecture, configuration schema v2, provider foundation (this ADR).
2. PBS provider: read-only foundation, then guarded mutations.
3. Unified PVE/PBS environment backup-health service.
4. Linux fleet inventory and the Ansible execution boundary.
5. Maintenance status and immutable planning.
6. Maintenance apply, verification, and reporting.
7. Security-update policy workflow (unattended-upgrades configuration).
8. One-shot monitoring and external monitoring integration.

Compatibility commitments across all phases: existing version-1 configs and
all existing PVE commands keep working unchanged; new capabilities are opt-in
via configuration; Ansible-dependent features degrade cleanly when Ansible is
absent; automated tests never require live PVE, PBS, SSH, DNS, or Internet
access; CI keeps building on supported Linux, macOS, and Windows targets.

## Consequences

- Two typed API clients (PVE, PBS) share transport, credential, redaction,
  safety, task-polling, and output infrastructure but evolve independently.
- The config package gains version-range validation and per-provider profile
  validation; future sections land behind schema version 2 without breaking
  version 1 readers of their own files.
- New subsystems (inventory, maintenance, monitoring, backup-health) are
  services above providers, keeping provider packages narrow.
- The allowlisted Ansible adapter concentrates all process-execution risk in
  one heavily-tested boundary.
- Test surface grows substantially (contract tests per provider, plan
  digest/tamper/expiry tests, partial-failure tests, redaction fuzz tests);
  this is accepted cost for the safety claims Nodex makes.
