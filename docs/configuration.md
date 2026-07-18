# Configuration Reference

Nodex uses a local YAML configuration file plus optional credential backends. Configuration is read by commands that need a profile or provider connection.

## Configuration Path

| Platform | Path |
|----------|------|
| Linux | `$XDG_CONFIG_HOME/nodex/config.yaml`, or `~/.config/nodex/config.yaml` when `XDG_CONFIG_HOME` is unset |
| macOS | `~/Library/Application Support/Nodex/config.yaml` |
| Windows | `%AppData%\Nodex\config.yaml` |

`nodex init` creates the configuration file. Interactive mode prompts for provider, endpoint, credential reference, and profile name. Non-interactive mode creates a minimal `default` profile with provider `proxmox` and no endpoint.

## Schema Versions

Nodex reads schema versions 1 and 2. New configurations are written as
version 2. Version 1 files keep loading with unchanged semantics, and Nodex
never silently rewrites a file to a newer schema version: reading a config
never writes it, and commands that modify the config (`profile add`,
`profile use`, ...) preserve the file's existing version. A config whose
version is newer than the running binary supports is rejected with an error
asking you to upgrade Nodex.

Version 2 is the multi-provider schema: profiles may use any known provider
(`proxmox` for Proxmox VE, `pbs` for Proxmox Backup Server), and future
optional sections (environments, inventory, monitoring) attach to version 2.
Versions 1 and 2 are structurally identical for the fields below.

```yaml
version: 2
current_profile: production-pve
profiles:
  production-pve:
    provider: proxmox
    endpoint: https://pve.example.com:8006
    credential_ref: keyring:production-pve
    ca_file: /home/alex/.config/nodex/lab-ca.pem
  production-pbs:
    provider: pbs
    endpoint: https://pbs.example.com:8007
    credential_ref: keyring:production-pbs
```

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `version` | int | yes | 2 from `init` | Schema version. 1 and 2 are supported. |
| `current_profile` | string | no | empty, or first profile | Profile used when `--profile` is not provided. Must match a key in `profiles`. |
| `profiles` | map | yes | empty map | Named provider connection profiles. |
| `profiles.<name>.provider` | string | yes | `proxmox` from `init` and `profile add` | Provider name (`proxmox` or `pbs`). Normalized to lowercase. |
| `profiles.<name>.endpoint` | string | required for live commands | empty | HTTPS provider endpoint. |
| `profiles.<name>.credential_ref` | string | no | empty | Credential backend reference. |
| `profiles.<name>.ca_file` | string | no | empty | PEM CA certificate file to add to the system trust pool. |

Profile names must match `^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`.

`nodex init`, `nodex profile add --provider <name>`, and `nodex profile
import` accept only known provider names (`proxmox`, `pbs`). A config file
containing an unknown but well-formed provider name still loads — so a file
written by a newer Nodex does not invalidate your other profiles — but any
command that uses such a profile fails with an unknown-provider error. The
`pbs` provider name is reserved by the fleet-operations roadmap
(`docs/roadmap.md`); PBS commands ship in a later phase. Endpoint, TLS, and
credential rules below apply identically to every provider: PVE and PBS
credentials are always separate credential-store entries, and there is no
insecure TLS option for any provider.

## Endpoint Rules

Endpoints must:
- Use the `https` scheme
- Include a host
- Omit URL user information
- Omit query strings and fragments
- Omit path components other than `/`

Valid: `https://pve.example.com:8006`

Invalid:
- `http://pve.example.com:8006` (not HTTPS)
- `https://user@pve.example.com:8006` (contains userinfo)
- `https://pve.example.com:8006/api2/json` (contains path)
- `https://pve.example.com:8006?token=abc` (contains query string)

## Profile Selection

Commands that connect to a provider choose a profile in this order:

1. `--profile <name>` global flag
2. `current_profile` from the configuration file

Commands fail with a configuration error when no profile is selected, the selected profile does not exist, or the selected profile has no endpoint for a live provider command.

## Profile Import and Export

### Export

```bash
nodex profile export lab
```

Writes a sanitized JSON object to stdout with `name`, `provider`, `endpoint`, and `ca_file` fields. Credential data is never exported.

### Import

```bash
nodex profile import < lab-profile.json
```

Reads a JSON profile from stdin and adds it to the configuration. The profile must have at minimum a `name` field.

## Credential References

Credential references use one of these forms:

```text
backend:name
name
```

A bare `name` is treated as `file:name`.

| Backend | Read | Write | Description |
|---------|------|-------|-------------|
| `file` | yes | yes | JSON files under `~/.nodex/credentials/` |
| `keyring` | yes | yes | OS keyring via `github.com/zalando/go-keyring` |
| `env` | yes | no | Environment variables |
| `stdin` | yes | no | Interactive token input from stdin |

`nodex profile set-credentials <name>` stores token credentials in the `file` backend by default and updates the profile's `credential_ref`. Options:

```bash
nodex profile set-credentials lab
nodex profile set-credentials lab --backend keyring
nodex profile set-credentials lab --backend file --credential-name lab-readonly
```

This command requires interactive input and is rejected when `--non-interactive` is set.

## Credential Resolution

When `credential_ref` is set, Nodex resolves only that reference.

When `credential_ref` is empty, Nodex tries:

1. Environment variables for the selected profile
2. A same-name file credential under `~/.nodex/credentials/`

If neither source is available, the command exits with a credential error (exit code 4).

## Environment Variables

For a profile named `lab`, environment variables use the uppercase profile name with hyphens converted to underscores:

```bash
export NODEX_LAB_TOKEN_ID='root@pam!nodex'
export NODEX_LAB_TOKEN_SECRET='example-token-secret'
```

Supported variables:
- `NODEX_<PROFILE>_TOKEN`
- `NODEX_<PROFILE>_TOKEN_ID`
- `NODEX_<PROFILE>_TOKEN_SECRET`
- `NODEX_<PROFILE>_USERNAME`
- `NODEX_<PROFILE>_PASSWORD`

The Proxmox VE and PBS providers use API token credentials. Incomplete token or username/password pairs are rejected by credential validation.

## Proxmox Backup Server Profiles

A PBS profile uses `provider: pbs` and the PBS API port (8007 by default):

```yaml
profiles:
  production-pbs:
    provider: pbs
    endpoint: https://pbs.example.com:8007
    credential_ref: keyring:production-pbs
```

```bash
nodex profile add production-pbs --provider pbs
nodex profile set-credentials production-pbs --backend keyring
```

PBS authenticates with its own API-token scheme
(`Authorization: PBSAPIToken=user@realm!tokenname:secret`). The token ID has
the form `user@realm!tokenname` (for example `automation@pbs!nodex`); PBS
separates the token name and secret with `:` where PVE uses `=`. Nodex builds
the header from the same `token_id`/`token_secret` credential fields used for
PVE — store the PBS token ID and secret exactly as PBS displays them at token
creation.

Keep PVE and PBS credentials in separate credential-store entries referenced
by separate profiles. Never reuse a PVE token for PBS or vice versa.

### PBS API token least privilege

Create a dedicated API token for Nodex and grant only what the commands you
use need:

- **Inspection only** (`pbs status/version/datastore/snapshot/task/...`):
  audit-level roles are sufficient — `Audit` on `/` (or, narrower,
  `Datastore.Audit` on `/datastore` plus `Sys.Audit` on `/system`).
- **Guarded maintenance runs** (`pbs verify run`, `pbs sync run`,
  `pbs prune run`, `pbs garbage-collection run`): additionally require the
  corresponding datastore privileges on the datastores involved —
  verification and GC need `Datastore.Verify`/`Datastore.Modify`-level
  rights, prune needs `Datastore.Prune`/`Datastore.Modify`, and sync jobs
  need the privileges PBS documents for the job's direction (typically
  `Remote.Read` plus `Datastore.Backup`/`Datastore.Prune` on the local
  store). Scope them to `/datastore/<name>` rather than `/` where possible.

Do not grant `Admin`, and keep a separate audit-only token if you script
inspection separately from maintenance. Never reuse the PVE token.

## Environments

The `environments` section (schema version 2 only) groups a PVE profile and
a PBS profile for `nodex environment health` and `nodex environment
backup-health`:

```yaml
version: 2
environments:
  homelab:
    pve_profile: production-pve
    pbs_profile: production-pbs
    backup_max_age_hours: 36
    verify_max_age_days: 14
    datastore_usage_warn_percent: 80
    datastore_usage_block_percent: 95
    namespaces: ["", "prod"]
    exclude_guests: [900]
```

| Field | Default | Description |
|-------|---------|-------------|
| `pve_profile` | — | Profile with `provider: proxmox`. At least one of the two profile fields is required. |
| `pbs_profile` | — | Profile with `provider: pbs`. |
| `backup_max_age_hours` | 26 | Newest-backup age beyond which a protected guest degrades to warning. |
| `verify_max_age_days` | 8 | Snapshot age beyond which a missing verification degrades to warning. |
| `datastore_usage_warn_percent` | 80 | Datastore usage warning threshold. |
| `datastore_usage_block_percent` | 95 | Datastore usage blocking threshold (must be >= the warn threshold). |
| `namespaces` | root only | PBS namespaces searched for guest backups. |
| `exclude_guests` | none | VMIDs exempt from backup-coverage checks; every other PVE guest is treated as protected. |

Referenced profiles must exist and use the matching provider type. Adding an
`environments` section to a version-1 file is rejected with an error telling
you to set `version: 2` — this is the explicit (never silent) migration
path.

## Inventory

The `inventory` section (schema version 2 only) declares the Linux hosts
Nodex may manage over SSH through the allowlisted Ansible operations,
consumed by the `maintenance` commands (see the CLI reference).
Enrollment is always explicit: Proxmox discovery may suggest candidates, but
a guest is never SSH-manageable until it has an inventory entry.

```yaml
version: 2
inventory:
  hosts:
    pve-primary:
      address: pve.example.com
      role: pve
      environment: homelab
      pve_profile: production-pve
      ssh_user: automation
      ssh_port: 22
      ssh_key_file: ~/.ssh/nodex_automation
      known_hosts_file: ~/.ssh/known_hosts_nodex
      maintenance_group: hypervisors
      criticality: critical
      backup_required: true
      automatic_reboot: false
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `address` | yes | — | Hostname or IP. No scheme, port, or userinfo. |
| `role` | yes | — | Host role: `pve`, `pbs`, `dns`, `generic`, or another lowercase identifier. `pve`, `pbs`, and `dns` receive extra protection in maintenance sequencing. |
| `environment` | no | — | Environment this host belongs to; must exist in `environments`. |
| `pve_profile` / `pbs_profile` | no | — | Provider profile linkage for backup-aware checks. |
| `ssh_user` | yes | — | SSH user name. |
| `ssh_port` | no | 22 | SSH port. |
| `ssh_key_file` | no | agent | Path to the private key file. Only a path — key material never appears in configuration. |
| `known_hosts_file` | no | SSH default | Dedicated known_hosts file for host-key verification. |
| `maintenance_group` | no | — | Grouping for maintenance sequencing. |
| `criticality` | no | `standard` | `critical` or `standard`. |
| `backup_required` | no | false | Require a recent successful PBS backup before maintenance. |
| `automatic_reboot` | no | false | Never enabled by default, for any role. |

### SSH trust model

The inventory stores no secrets: no private keys, SSH passwords, vault
passwords, or sudo passwords — only file path references. Authentication
uses the SSH agent or the referenced key file. Host-key verification is
always enforced (`ANSIBLE_HOST_KEY_CHECKING=True` is pinned by the
execution boundary and cannot be disabled through Nodex); populate the
configured `known_hosts_file` out of band before first use.

## File Credentials

The file backend stores one JSON file per credential name under `~/.nodex/credentials/`. Files are written through a temporary file with restricted permissions and renamed into place.

Example file shape (use fictional values):

```json
{
  "type": "token",
  "token_id": "root@pam!nodex",
  "token_secret": "example-token-secret"
}
```

Do not commit credential files or paste real token values into documentation, issues, logs, or shell transcripts.

## TLS Behavior

Nodex enables TLS certificate and hostname verification by default. The transport requires TLS 1.2 or newer. There is no CLI flag or configuration field for insecure TLS.

Set `ca_file` when a Proxmox endpoint uses a private CA:

```yaml
ca_file: /home/alex/.config/nodex/lab-ca.pem
```

The CA file is appended to the system certificate pool for that profile.

## Password Input

For commands that require a password (e.g., `access user create`), use `--password-stdin` to read the password from stdin, or run interactively for a hidden prompt. Passwords are never accepted as command-line arguments.

## Configuration Writes and Locking

Nodex writes configuration atomically by writing a temporary file and renaming it into place. Configuration updates acquire a lock for read-modify-write operations. The configuration directory is created with mode `0700` on platforms that support POSIX modes.
