# Nodex Product Principles

> **Nodex is a secure, predictable, all-in-one CLI for understanding and operating self-hosted infrastructure—Proxmox-first, inspection-led, management-capable, automation-friendly, and designed to support additional providers without sacrificing provider-native depth.**

This document is the canonical product constitution for Nodex. Every capability decision must be consistent with these principles.

## Primary User Outcomes

Nodex exists so self-hosted infrastructure operators can:

1. **See what exists.** List nodes, VMs, containers, storage, network configurations, firewall rules, HA resources, SDN topology, Ceph state, backups, replication jobs, and resource pools.

2. **Understand state.** Inspect detailed node status (CPU, memory, disk, services, network, DNS, time, disks, certificates, subscription, updates), VM and container configuration, snapshot config, task status, cluster quorum, event history, and syslog.

3. **Diagnose problems.** Run `doctor` checks across all configured profiles to identify configuration errors, connectivity failures, and provider-level issues before they escalate.

4. **Audit configuration.** Access VM and container configurations, firewall rules at every level (cluster, node, VM, CT), identity and ACL state, backup schedules, and replication job definitions.

5. **Automate queries.** Use JSON and YAML output modes for scripting, monitoring, and infrastructure-as-code integration. Empty lists are `[]` not `null`. Structured output streams never mix human-readable text.

6. **Perform safe operations.** Start, stop, shutdown, reboot, suspend, resume, pause, and unpause VMs and containers with tiered confirmation gates. Create and manage snapshots. Update configurations. Create manual backups and manage backup schedules. Upload and download storage content.

7. **Work across environments.** Manage multiple Proxmox endpoints through named profiles with independent credentials. Use `--all` to aggregate results across all profiles.

8. **Use provider-native functionality.** Every Proxmox API endpoint exposed through Nodex preserves the provider's semantics. Nodex adds safety, not abstraction that hides platform capabilities.

## Non-Goals

Nodex is explicitly NOT:

- **A daemon or background service.** It runs as a single process and exits.
- **A dashboard or web UI.** It has no graphical interface.
- **A monitoring server.** It does not collect telemetry, store time-series data, or send alerts.
- **A remote control plane.** It is a local CLI that connects directly to infrastructure endpoints.
- **An agent.** Nothing is installed on Proxmox nodes by Nodex.
- **A GitOps reconciler.** It does not watch repositories or maintain desired-state loops.
- **A raw API executor.** Every command is purpose-built with safety checks.
- **A feature-count contest.** Capabilities are added when they serve real operator needs, not to match another tool's inventory.

## Operating Model

- **Local CLI.** Nodex runs on the operator's machine (Linux, macOS, Windows).
- **Single process.** No daemon, no agent, no sidecar process.
- **No mandatory server.** Nodex connects directly to infrastructure endpoints.
- **No hidden telemetry.** Nodex does not phone home, collect usage data, or send analytics.
- **Explicit connections.** Every remote connection is declared through a named profile with an explicit endpoint.
- **Least-privilege credentials.** Nodex works with read-only API tokens for inspection and needs only the permissions required for each operation. Narrow permissions are documented per command.

## Product Decision Gate

Every capability proposed for Nodex must answer these questions:

1. **User need.** What real operator problem does this solve?
2. **Safety tier.** What is the risk classification (Observation, Reversible, Disruptive, Destructive, Security Administration)?
3. **Confirmation requirement.** What gates protect the user (none, `--yes`, `--yes --force`, type-in verification, `--expert`)?
4. **Least privilege.** What is the narrowest Proxmox permission set required?
5. **Provider-native fidelity.** Does this expose the real Proxmox semantics or hide them?
6. **Output contract.** What does the command emit in table, JSON, and YAML modes?
7. **Exit-code behavior.** What exit codes are possible and what do they mean?
8. **Idempotency.** Is the operation safe to repeat?
9. **Cancellation.** What happens on SIGINT or timeout?
10. **Non-interactive behavior.** Does this work in scripts without a terminal?
11. **Credentials.** What credential types are needed?
12. **Multi-profile.** Does `--all` make sense for this command?
13. **Risk of cluster lockout.** Could this operation break cluster membership or Corosync?
14. **Alternatives considered.** Why is this the right approach?

## Safety Model

Nodex uses a five-tier safety classification:

| Tier | Name | Examples | Confirmation |
|------|------|----------|-------------|
| 0 | Observation | `node list`, `vm show`, `storage list` | None |
| 1 | Reversible | `vm start`, `vm shutdown`, `container reboot` | `--yes` or interactive prompt |
| 2 | Disruptive | `vm reset`, `vm migrate`, `vm stop` | `--yes --force` or double confirmation |
| 3 | Destructive | `vm delete`, `vm snapshot delete`, `storage delete` | Type-in target verification |
| 4 | Security Administration | `access user create`, ACL changes | `--expert` flag |

Non-interactive mode fails closed when confirmation is required and flags are not provided.

## Provider Model

- **Proxmox-first.** The initial and primary provider is Proxmox VE. Every Proxmox capability exposed through Nodex is validated against the official Proxmox API.
- **Extensible.** The provider registry supports additional providers without changing the CLI shell.
- **Provider-native.** Nodex does not force a lowest-common-denominator abstraction. Each provider exposes its real capabilities.
