# Support

Nodex is an early-development CLI for inspecting and operating self-hosted Proxmox VE infrastructure. This document describes what is supported and how to get help.

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

- **Local CLI use** on Linux, macOS, and Windows (amd64, arm64 on macOS)
- **Proxmox VE provider** — the built-in `proxmox` provider
  - Read-only inspection of nodes, VMs, containers, storage, tasks, events, snapshots, firewall rules, HA resources, backups, SDN, pools, Ceph, and access control
  - Mutation operations through the five-tier safety model (lifecycle, config updates, snapshots, backups, storage, migration, clone, firewall, SDN, Ceph, replication, access)
- **Configuration** via YAML schema v1
- **Credential management** through file, keyring, environment, and stdin backends
- **API token authentication** (recommended) and password authentication
- **TLS 1.2+** with certificate verification and custom CA support
- **Output formats** — table, JSON, and YAML
- **Shell completion** for bash, zsh, and fish

### Not Yet Supported

- **Stable release artifacts.** No versioned release binaries are published.
- **Non-Proxmox providers.** The provider registry supports additional providers, but only `proxmox` is implemented.
- **Corosync configuration.** Cluster membership changes are excluded.
- **Subscription key management.**
- **TFA enrollment.**
- **CA certificate management.**
- **Guest console access.**
- **PCI or USB passthrough configuration.**
- **Daemon or service operation.** Nodex is a CLI tool, not a background service.

### Explicitly Excluded

- **Network apply operations** carry cluster-lockout risk and require explicit user approval before use.
- **Automated password argument passing.** Passwords are never accepted as CLI arguments.

## Compatibility

Nodex targets current Proxmox VE releases. Version-specific API differences are documented when known. See [compatibility.md](docs/compatibility.md) for the formal compatibility policy.

The internal Go APIs (everything under `internal/`) are not stable and may change without notice before 1.0.

## Building from Source

Requirements:
- Go 1.25.12
- `make` (optional; `go build` works directly)

```bash
git clone https://github.com/geoffmcc/nodex.git
cd nodex
make build
./nodex version
```

## Documentation

- [Product principles](docs/product-principles.md)
- [CLI reference](docs/cli-reference.md)
- [Configuration reference](docs/configuration.md)
- [Architecture](docs/architecture.md)
- [Product requirements](docs/product_requirements.md)
- [Compatibility policy](docs/compatibility.md)
- [Security policy](SECURITY.md)
- [Contributing](CONTRIBUTING.md)
