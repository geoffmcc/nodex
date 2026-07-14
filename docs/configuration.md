# Configuration Reference

Nodex uses a local YAML configuration file plus optional credential backends. Configuration is read by commands that need a profile or provider connection.

## Configuration Path

| Platform | Path |
|----------|------|
| Linux | `$XDG_CONFIG_HOME/nodex/config.yaml`, or `~/.config/nodex/config.yaml` when `XDG_CONFIG_HOME` is unset |
| macOS | `~/Library/Application Support/Nodex/config.yaml` |
| Windows | `%AppData%\Nodex\config.yaml` |

`nodex init` creates the configuration file. Interactive mode prompts for provider, endpoint, credential reference, and profile name. Non-interactive mode creates a minimal `default` profile with provider `proxmox` and no endpoint.

## Schema Version 1

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

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `version` | int | yes | 1 from `init` | Schema version. Only 1 is supported. |
| `current_profile` | string | no | empty, or first profile | Profile used when `--profile` is not provided. Must match a key in `profiles`. |
| `profiles` | map | yes | empty map | Named provider connection profiles. |
| `profiles.<name>.provider` | string | yes | `proxmox` from `init` and `profile add` | Provider name. Normalized to lowercase. |
| `profiles.<name>.endpoint` | string | required for live commands | empty | HTTPS provider endpoint. |
| `profiles.<name>.credential_ref` | string | no | empty | Credential backend reference. |
| `profiles.<name>.ca_file` | string | no | empty | PEM CA certificate file to add to the system trust pool. |

Profile names must match `^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`.

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

The Proxmox provider uses API token credentials. Incomplete token or username/password pairs are rejected by credential validation.

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
