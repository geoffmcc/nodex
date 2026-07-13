# Nodex

Open infrastructure management platform for self-hosters and homelabs.

## What it does

Nodex connects to your Proxmox VE environment and gives you a clean CLI to inspect nodes, VMs, containers, and storage — no web UI required.

## Status

Early development (v0.1). The current built-in Proxmox provider issues only read-only `GET` requests for version, nodes, and cluster resources. Nodex has no daemon and no telemetry.

## Install

```bash
go install github.com/geoffmcc/nodex/cmd/nodex@latest
```

Or build from source:

```bash
git clone https://github.com/geoffmcc/nodex.git
cd nodex
make build
```

## Quick start

```bash
nodex init                    # create config
nodex profile add home        # add a Proxmox endpoint
nodex profile test            # verify connectivity
nodex node list               # list nodes
nodex vm list                 # list VMs
nodex container list          # list containers
nodex storage list            # list storage
```

## Configuration

Config is stored at:

| Platform | Path |
|----------|------|
| Linux | `~/.config/nodex/config.yaml` |
| macOS | `~/Library/Application Support/Nodex/config.yaml` |
| Windows | `%AppData%\Nodex\config.yaml` |

Profiles must use `https://` Proxmox endpoints without URL user info, query strings, fragments, or extra path components. Custom CAs can be configured with `ca_file`; certificate and hostname verification remain enabled. The global `--timeout` flag controls provider request timeouts.

File-backed credentials are stored under `~/.nodex/credentials` using validated credential names. Credential references use `backend:name` (`file`, `keyring`, `env`, or `stdin`) or a bare name for the file backend. Incomplete token or username/password credential pairs are rejected.

## Commands

```
nodex version
nodex init
nodex profile add|list|show|use|current|test|remove
nodex provider list|capabilities
nodex doctor
nodex node list
nodex vm list
nodex container list
nodex storage list
```

## Output

Supports `table` (default for TTY), `json`, and `yaml` output formats:

```bash
nodex vm list --output json
nodex node list --output yaml
```

Human-readable table and error output is redacted and terminal-sanitized. JSON and YAML are emitted through structured encoders and redaction while preserving valid syntax.

## Development

Requires Go 1.25.12 or newer within the Go 1.25 release family.

```bash
make build    # build binary
make test     # run tests
make lint     # run linter
make clean    # remove binary
```

## License

[GPL-3.0](LICENSE)
