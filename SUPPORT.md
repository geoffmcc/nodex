# Support

Nodex is a pre-1.0 local CLI for operating self-hosted Proxmox VE and Proxmox Backup Server infrastructure, with optional enrolled-host maintenance and one-shot monitoring. This document describes supported scope and how to get help.

## Getting Help

Open an issue at `https://github.com/geoffmcc/nodex/issues` for:

- Installation or build problems
- Unexpected CLI output or behavior
- Configuration or credential-resolution issues
- Proxmox provider errors
- Documentation corrections
- Feature requests and use case discussion

Include:
- The exact Nodex command you ran
- Your operating system and architecture
- `nodex version` output
- Whether stdout was a terminal or redirected (for output formatting issues)
- Sanitized configuration snippets (remove real tokens, passwords, hostnames, and IPs)
- Redacted error messages

Do not include live Proxmox tokens, passwords, private keys, authorization headers, private hostnames, or public IP addresses that should remain private.

## Security Issues

For suspected vulnerabilities, follow the [security policy](SECURITY.md). Do not open a public issue containing exploit details or secrets.

## Supported Scope

### Supported

- **Local CLI use** on Linux, macOS, and Windows. Release archives target amd64 and arm64; the CI test matrix is maintained in `.github/workflows/ci.yml`.
- **Proxmox VE provider** — read-only inspection and confirmation-gated management across nodes, VMs, containers, storage, tasks, events, logs, snapshots, firewalls, HA, backups, SDN, Ceph, access, pools, network configuration and replication
- **Proxmox Backup Server provider** — a separate `pbs` profile with status, datastore, snapshot, task, job, and garbage-collection inspection; guarded verify, sync, prune and GC runs
- **PVE/PBS environments** — combined reachability, datastore, task and per-guest backup coverage/age/verification evaluation
- **Fleet maintenance** — optional Ansible-backed workflows for explicitly enrolled Linux hosts: inventory, status/preflight, immutable plans, guarded apply, verify, recover/reconcile and report
- **One-shot monitoring** — explicitly configured generic HTTP(S), TCP, TLS and DNS checks, plus provider-backed PVE/PBS API, task, datastore and backup checks
- **Agent/automation interface** — JSON/YAML output, operation discovery, opt-in versioned `--agent` execution, mutation receipt deduplication and read-only reconciliation
- **Configuration** via YAML schemas v1 and v2
- **Credential management** through file, keyring, environment, and stdin backends
- **API token authentication** (supported) for provider connections
- **TLS 1.2+** with certificate verification and custom CA support
- **Output formats** — table, JSON, and YAML
- **Shell completion** for bash, zsh, fish, and PowerShell

### Limitations and exclusions

- Only the built-in `proxmox` (PVE) and `pbs` (PBS) providers are implemented.
- Cluster join performs a read-only preflight and refuses the join request; the
  required peer credential is not accepted or transported by Nodex.
- `container os-update` supports only the fixed `approved-full-upgrade` policy
  for an explicitly enrolled running LXC. It does not reboot the guest. VM OS
  updates and arbitrary remote shell/Ansible commands are not provided.
- Fleet maintenance requires a supported local Ansible installation. Most
  PVE/PBS CLI operations do not require Ansible; no Ansible-backed service
  monitoring integration is currently implemented.
- One-shot monitoring does not discover targets, schedule checks, store history
  or send alerts. Configured `service` targets currently report `unsupported`.
- Agent mode is not a sandbox or proof of human approval. Some local, fan-out,
  interactive, credential and native-ledger operations intentionally remain
  outside its supported operation set; inspect `nodex operation describe`.
- Nodex does not configure Corosync membership, manage subscription keys or
  perform TFA enrollment.

Passwords and token secrets are never accepted as ordinary command-line
arguments. Network apply/revert is implemented but disruptive; review the exact
target and pending configuration and follow the operation's confirmation
requirements.

## Compatibility

Nodex targets current Proxmox VE releases. Version-specific API differences are documented when known. See [compatibility.md](docs/compatibility.md) for the formal compatibility policy.

The internal Go APIs (everything under `internal/`) are not stable and may change without notice before 1.0.

## Building from Source

Requirements:
- Go 1.27.2
- `make` (optional; `go build` works directly)

```bash
git clone https://github.com/geoffmcc/nodex.git
cd nodex
make build
./nodex version
```

## Documentation

- [Product principles](docs/product-principles.md)
- [Operator guide](docs/operator-guide.md)
- [Agent instructions](NODEX_AGENT.md)
- [CLI reference](docs/cli-reference.md)
- [Configuration reference](docs/configuration.md)
- [Architecture](docs/architecture.md)
- [Product requirements](docs/product_requirements.md)
- [Compatibility policy](docs/compatibility.md)
- [Security policy](SECURITY.md)
- [Contributing](CONTRIBUTING.md)
