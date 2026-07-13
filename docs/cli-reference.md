# Nodex CLI Reference

This reference describes the commands implemented by the `nodex` CLI.

## Syntax

```text
nodex [global-flags] <command> [command-args]
```

Global flags must appear before the command name:

```bash
nodex --output json node list
```

This does not work as a global output flag because it appears after the command arguments:

```bash
nodex node list --output json
```

Use `nodex help` for top-level help and `nodex help <command>` for top-level command help. The CLI does not provide detailed `--help` output for subcommands.

## Global flags

| Flag | Description | Default |
| --- | --- | --- |
| `--profile <name>` | Override the configured current profile. | empty |
| `--output <format>` | Output format: `table`, `json`, or `yaml`. | `table` when stdout is a terminal; `json` otherwise |
| `--timeout <duration>` | Provider request timeout using Go duration syntax such as `10s` or `1m`. Must be greater than zero. | `30s` |
| `--no-color` | Disable color output. | `false` |
| `--non-interactive` | Disable prompts. | `false` |
| `--quiet` | Suppress non-essential success output for commands that support it. | `false` |
| `--verbose` | Enable info-level stderr logging. | `false` |
| `--debug` | Enable redacted debug-level stderr logging. | `false` |

`--debug` takes precedence over `--verbose`; `--quiet` suppresses logger output unless a more verbose level is selected.

## Commands

### `nodex version`

Print version metadata.

```bash
nodex version
```

Output fields are:

- `Nodex <version>`
- `Go: <go-version>`
- `Commit: <commit>`
- `Built: <build-date>`
- `Dirty: true` when Go build metadata reports modified source state

`make build` injects version, commit, build date, and Go version through linker flags. Builds without linker flags fall back to Go build information when available.

### `nodex init`

Create the configuration file.

```bash
nodex init
nodex --non-interactive init
```

Interactive mode prompts for provider, endpoint URL, credential reference, and profile name. If the configuration file already exists, interactive mode asks before overwriting it.

Non-interactive mode creates a minimal configuration with a `default` profile using provider `proxmox` and no endpoint.

### `nodex completion`

Generate shell completion scripts.

```bash
nodex completion bash
nodex completion zsh
nodex completion fish
```

The command writes the completion script to stdout.

### `nodex provider list`

List registered providers.

```bash
nodex provider list
nodex --output json provider list
```

The current built-in provider list contains `proxmox`.

### `nodex provider capabilities <name>`

Show capabilities reported by a provider.

```bash
nodex provider capabilities proxmox
```

The Proxmox provider currently reports:

- `cluster`
- `containers`
- `nodes`
- `storage`
- `vms`

### `nodex profile add <name>`

Add a profile with provider `proxmox`.

```bash
nodex profile add lab
```

If this is the first profile, it also becomes `current_profile`. The new profile has no endpoint until you edit the configuration file.

### `nodex profile list`

List configured profiles.

```bash
nodex profile list
nodex --output yaml profile list
```

Table output columns are `NAME`, `PROVIDER`, `ENDPOINT`, and `CURRENT`.

### `nodex profile show <name>`

Show profile details.

```bash
nodex profile show lab
```

Table output includes name, provider, endpoint, credential reference, optional CA file, and current-profile status.

### `nodex profile set-credentials <name>`

Prompt for a Proxmox API token ID and token secret, store them, and update the profile's `credential_ref`.

```bash
nodex profile set-credentials lab
nodex profile set-credentials lab --backend keyring
nodex profile set-credentials lab --backend file --credential-name lab-readonly
```

Options:

| Option | Values | Default | Description |
| --- | --- | --- | --- |
| `--backend <backend>` | `file`, `keyring` | `file` | Storage backend to write. |
| `--credential-name <name>` | validated credential name | profile name | Credential name to store and reference. |

This command requires interactive input. It is rejected when `--non-interactive` is set.

### `nodex profile use <name>`

Set the current profile.

```bash
nodex profile use lab
```

### `nodex profile current`

Print the current profile.

```bash
nodex profile current
nodex --output json profile current
```

### `nodex profile test [name]`

Connect to a profile and request the provider version endpoint.

```bash
nodex profile test
nodex profile test lab
```

When no name is provided, Nodex uses the current profile.

### `nodex profile remove <name> [--remove-credential]`

Remove a profile from the configuration file.

```bash
nodex profile remove lab
nodex profile remove lab --remove-credential
```

When `--remove-credential` is present, Nodex also deletes the referenced credential. If the profile has no `credential_ref`, it attempts to delete a same-name file credential.

### `nodex node list`

List Proxmox nodes for the selected profile.

```bash
nodex node list
nodex --profile lab --output json node list
```

Table output columns are `NAME`, `STATUS`, `IP`, `ROLE`, and `UPTIME`.

### `nodex node show <name>`

Show one node by node name or node ID.

```bash
nodex node show pve-a
```

### `nodex vm list`

List virtual machines from Proxmox cluster resources.

```bash
nodex vm list
```

Table output columns are `ID`, `NAME`, `STATUS`, `NODE`, `CPU`, `MEMORY`, and `DISK`.

### `nodex vm show <id>`

Show one virtual machine by ID.

```bash
nodex vm show pve-a/100
```

### `nodex container list`

List containers from Proxmox cluster resources.

```bash
nodex container list
```

Table output columns are `ID`, `NAME`, `STATUS`, `NODE`, `OS`, `MEMORY`, and `DISK`.

### `nodex container show <id>`

Show one container by ID.

```bash
nodex container show pve-a/200
```

### `nodex storage list`

List storage pools from Proxmox cluster resources.

```bash
nodex storage list
```

Table output columns are `NAME`, `TYPE`, `STATUS`, `NODE`, `TOTAL`, `USED`, and `AVAIL`.

### `nodex storage show <name>`

Show one storage pool by storage name or storage ID.

```bash
nodex storage show local-lvm
```

### `nodex doctor`

Run local configuration checks and connectivity checks for configured profiles.

```bash
nodex doctor
nodex --output json doctor
```

Table output includes `CHECK`, `STATUS`, and `MESSAGE`, followed by a summary. If any table-mode check fails, the command returns an error after printing the report. JSON and YAML modes return a structured report with `pass`, `fail`, `warn`, and `results` fields.

## Output formats

### Table

Table output is intended for terminal use. Byte values in resource tables are rendered in IEC units such as `1.0 KiB`.

### JSON

JSON output is indented with two spaces. Empty VM, container, and storage lists are emitted as `[]` instead of `null`.

### YAML

YAML output uses native YAML serialization. Empty VM, container, and storage lists are emitted as `[]`.

## Credential resolution

See the [configuration reference](configuration.md) for credential backends, environment variables, file paths, and TLS settings.

## Exit codes

| Code | Meaning |
| ---: | --- |
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
| 11 | Partial failure |
| 12 | Provider error |
| 130 | Interrupted by Ctrl+C/SIGINT |
| 143 | Terminated by SIGTERM |

Not every exit code is currently produced by every provider path, but the codes are reserved by the application package.

## Signals and cancellation

The entry point listens for SIGINT and SIGTERM. On receipt, Nodex cancels the command context. If an error is returned after cancellation, the process exits with `130` for SIGINT or `143` for SIGTERM.

## Error output

Errors are printed to stderr as:

```text
Error: <message>
```

The message is passed through redaction and terminal sanitization before printing.
