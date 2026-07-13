# Configuration Reference

Nodex uses a local YAML configuration file plus optional credential backends. Configuration is read by commands that need a profile or provider connection.

## Configuration path

| Platform | Path |
| --- | --- |
| Linux | `$XDG_CONFIG_HOME/nodex/config.yaml`, or `~/.config/nodex/config.yaml` when `XDG_CONFIG_HOME` is unset |
| macOS | `~/Library/Application Support/Nodex/config.yaml` |
| Windows | `%AppData%\Nodex\config.yaml` |

`nodex init` creates the configuration file. In interactive mode it prompts for provider, endpoint, credential reference, and profile name. In non-interactive mode it creates a minimal `default` profile with provider `proxmox` and no endpoint.

## Schema version 1

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
| --- | --- | --- | --- | --- |
| `version` | integer | yes | `1` from `nodex init` | Configuration schema version. Only `1` is supported. |
| `current_profile` | string | no | empty, or first profile created by `profile add` | Profile used when `--profile` is not provided. If set, it must match a key in `profiles`. |
| `profiles` | map | yes | empty map | Named provider connection profiles. |
| `profiles.<name>.provider` | string | yes | `proxmox` from `init --non-interactive` and `profile add` | Provider name. Values are normalized to lowercase during validation. |
| `profiles.<name>.endpoint` | string | required for live provider commands | empty | HTTPS provider endpoint. |
| `profiles.<name>.credential_ref` | string | no | empty | Credential backend reference. |
| `profiles.<name>.ca_file` | string | no | empty | PEM CA certificate file to add to the system trust pool for this profile. |

Profile names must match `^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`.

## Endpoint rules

Endpoints must:

- use the `https` scheme;
- include a host;
- omit URL user information;
- omit query strings and fragments;
- omit path components other than `/`.

Valid example:

```yaml
endpoint: https://pve.example.com:8006
```

Invalid examples:

```yaml
endpoint: http://pve.example.com:8006
endpoint: https://user@pve.example.com:8006
endpoint: https://pve.example.com:8006/api2/json
endpoint: https://pve.example.com:8006?token=example
```

## Profile selection

Commands that connect to a provider choose a profile in this order:

1. `--profile <name>` global flag.
2. `current_profile` from the configuration file.

Commands fail with a configuration error when no profile is selected, the selected profile does not exist, or the selected profile has no endpoint for a live provider command.

## Credential references

Credential references use one of these forms:

```text
backend:name
name
```

A bare `name` is treated as `file:name`.

Supported resolver backends are:

| Backend | Can read | Can store through `profile set-credentials` | Notes |
| --- | --- | --- | --- |
| `file` | yes | yes | JSON files under `~/.nodex/credentials/`. |
| `keyring` | yes | yes | OS keyring through `github.com/zalando/go-keyring`. |
| `env` | yes | no | Environment variables. |
| `stdin` | yes | no | Interactive token input from stdin. |

`nodex profile set-credentials <name>` stores token credentials in the `file` backend by default and updates the profile's `credential_ref`. Use `--backend keyring` to store in the OS keyring, and `--credential-name <name>` to store credentials under a different credential name.

```bash
nodex profile set-credentials lab
nodex profile set-credentials lab --backend keyring
nodex profile set-credentials lab --backend file --credential-name lab-readonly
```

The command requires an interactive terminal and is rejected when `--non-interactive` is set.

## Credential resolution

When `credential_ref` is set, Nodex resolves only that reference.

When `credential_ref` is empty, Nodex tries:

1. environment variables for the selected profile;
2. a same-name file credential under `~/.nodex/credentials/`.

If neither source is available, the command exits with a credential error.

## Environment variables

For a profile named `lab`, environment variables use the uppercase profile name with hyphens converted to underscores:

```bash
export NODEX_LAB_TOKEN_ID='root@pam!nodex'
export NODEX_LAB_TOKEN_SECRET='example-token-secret'
```

Supported variables are:

- `NODEX_<PROFILE>_TOKEN`
- `NODEX_<PROFILE>_TOKEN_ID`
- `NODEX_<PROFILE>_TOKEN_SECRET`
- `NODEX_<PROFILE>_USERNAME`
- `NODEX_<PROFILE>_PASSWORD`

The Proxmox provider uses API token credentials. Incomplete token or username/password pairs are rejected by credential validation.

## File credentials

The file backend stores one JSON file per credential name under `~/.nodex/credentials/`. Files are written through a temporary file with mode `0600` and then renamed into place.

Example file shape using fictional values:

```json
{
  "type": "token",
  "token_id": "root@pam!nodex",
  "token_secret": "example-token-secret"
}
```

Do not commit credential files or paste real token values into documentation, issues, logs, or shell transcripts.

## TLS behavior

Nodex enables TLS certificate and hostname verification by default. The transport requires TLS 1.2 or newer. There is no CLI flag or configuration field for insecure TLS.

Set `ca_file` when a Proxmox endpoint uses a private CA:

```yaml
ca_file: /home/alex/.config/nodex/lab-ca.pem
```

The CA file is appended to the system certificate pool for that profile.

## Configuration writes and locking

Nodex writes configuration atomically by writing a temporary file and renaming it into place. Configuration updates acquire a lock for read-modify-write operations. The configuration directory is created with mode `0700` on platforms that support POSIX modes.
