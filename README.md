# Nodex

Open infrastructure management platform for self-hosters and homelabs.

## What it does

Nodex connects to your Proxmox VE environment and gives you a clean CLI to inspect nodes, VMs, containers, and storage — no web UI required.

## Status

Early development (v0.1). Read-only CLI for Proxmox. No mutations, no daemon, no telemetry — ever.

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

## Development

Requires Go 1.25+.

```bash
make build    # build binary
make test     # run tests
make lint     # run linter
make clean    # remove binary
```

## License

[GPL-3.0](LICENSE)
