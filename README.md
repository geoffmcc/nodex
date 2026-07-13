# Nodex

Nodex is a local command-line tool for inspecting self-hosted infrastructure. The current implementation connects to Proxmox VE and provides read-only commands for nodes, virtual machines, containers, storage pools, provider metadata, profile configuration, and local health checks.

Nodex is intended for homelab and self-hosting operators who want scriptable infrastructure inspection without using the Proxmox web UI. It is an early-development CLI, not a daemon, service, or telemetry collector.

## Current scope

- Built-in provider: `proxmox`
- Proxmox API operations: read-only `GET` requests to `/version`, `/nodes`, and `/cluster/resources`
- Resource commands: list and show nodes, VMs, containers, and storage
- Configuration: local YAML profile file with schema version `1`
- Credentials: environment variables, local JSON credential files, OS keyring, or stdin-backed references
- Output formats: table, JSON, and YAML

Nodex does not currently create, modify, start, stop, migrate, or delete infrastructure resources.

## Requirements

- Go `1.25.12` for building from source or installing with `go install`
- A Proxmox VE endpoint reachable over HTTPS for live provider commands
- A Proxmox API token for authenticated API access

CI currently builds and tests on Ubuntu, macOS Apple Silicon, macOS Intel, and Windows using Go `1.25.12`.

## Installation

Install the CLI with Go:

```bash
go install github.com/geoffmcc/nodex/cmd/nodex@latest
```

Or build from a source checkout:

```bash
git clone https://github.com/geoffmcc/nodex.git
cd nodex
make build
```

The source build writes a `nodex` binary in the repository root.

## Quick start

Create a minimal configuration and confirm the CLI works:

```bash
nodex init --non-interactive
nodex provider list
nodex --output json provider capabilities proxmox
```

Expected result: `provider list` shows `proxmox`, and the capabilities command returns JSON entries such as `nodes`, `vms`, `containers`, `storage`, and `cluster`.

To connect to Proxmox, edit the generated configuration file and add an HTTPS endpoint and credential reference. For example:

```yaml
version: 1
current_profile: lab
profiles:
  lab:
    provider: proxmox
    endpoint: https://pve.example.com:8006
    credential_ref: env:lab
```

Then provide credentials through the selected backend. For an environment-backed API token on a POSIX shell:

```bash
export NODEX_LAB_TOKEN_ID='root@pam!nodex'
export NODEX_LAB_TOKEN_SECRET='example-token-secret'
nodex profile test lab
nodex node list
```

Use fictional or test credentials in examples. Do not paste real tokens into shell history or documentation.

## Configuration files

Nodex stores its main configuration in a platform-specific user configuration directory:

| Platform | Default path |
| --- | --- |
| Linux | `$XDG_CONFIG_HOME/nodex/config.yaml`, or `~/.config/nodex/config.yaml` when `XDG_CONFIG_HOME` is unset |
| macOS | `~/Library/Application Support/Nodex/config.yaml` |
| Windows | `%AppData%\Nodex\config.yaml` |

Profiles require a provider name and may include an endpoint, credential reference, and custom CA file:

```yaml
version: 1
current_profile: lab
profiles:
  lab:
    provider: proxmox
    endpoint: https://pve.example.com:8006
    credential_ref: file:lab
    ca_file: /home/alex/.config/nodex/lab-ca.pem
```

Endpoint URLs must use `https://`, include a host, and must not include URL user info, query strings, fragments, or path components other than `/`.

See the [configuration reference](docs/configuration.md) for schema details, credential resolution, TLS behavior, and examples.

## Commands

Global flags must be placed before the command name because Nodex uses Go's standard flag parser:

```bash
nodex --output json vm list
```

Use `nodex help` for the top-level command list and `nodex help <command>` for top-level command help.

Implemented commands:

```text
nodex version
nodex init
nodex completion bash|zsh|fish
nodex profile add|list|show|set-credentials|use|current|test|remove
nodex provider list|capabilities
nodex doctor
nodex node list|show
nodex vm list|show
nodex container list|show
nodex storage list|show
```

See the [CLI reference](docs/cli-reference.md) for syntax, output behavior, and exit codes.

## Output and safety

- `--output table` is the default when stdout is a terminal.
- `--output json` is the default when stdout is not a terminal.
- `--output yaml` uses native YAML serialization.
- Error output is terminal-sanitized and redacted before printing.
- Provider operations are currently read-only.
- `nodex profile remove <name> [--remove-credential]` removes local profile configuration and can also delete the referenced local credential.

## Development

Common development commands:

```bash
make build
make test
make test-race
make lint
make clean
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for setup, testing, linting, and pull-request guidance.

## Documentation

- [CLI reference](docs/cli-reference.md)
- [Configuration reference](docs/configuration.md)
- [Architecture](docs/architecture.md)
- [Product requirements](docs/product_requirements.md)
- [Security policy](SECURITY.md)
- [Support guide](SUPPORT.md)

## License

Nodex is licensed under the [GNU General Public License v3.0](LICENSE).
