# Security Policy

## Threat Model

Nodex is a local CLI tool that connects directly to Proxmox VE endpoints over HTTPS. It stores credentials locally on the operator's machine and transmits them only to the configured endpoint. The primary threats are:

1. **Credential exposure.** API tokens, passwords, and authorization headers leaking through logs, errors, debug output, or documentation.
2. **Man-in-the-middle.** Interception or modification of HTTPS traffic to Proxmox endpoints.
3. **Unintended mutations.** Accidental state changes to infrastructure resources due to missing or bypassed confirmation gates.
4. **Privilege escalation.** Operations performed with broader permissions than necessary.
5. **Local file access.** Unauthorized read of credential files or configuration on the operator's machine.

## Supported Versions

Nodex is in early development and has no stable release series. Security fixes are provided on the `main` branch until versioned releases exist.

## Vulnerability Reporting

Please report suspected vulnerabilities privately to the repository maintainer instead of opening a public issue with exploit details or secrets. If no private channel is available, open a public GitHub issue with only a minimal, non-sensitive summary and ask for a private coordination channel.

Include the affected commit or version, a concise impact description, and safe reproduction steps using local test fixtures where possible.

Do not include live Proxmox tokens, passwords, private keys, or authorization headers in reports. If a credential may have been exposed, rotate it before sharing diagnostics.

The maintainer will investigate reachability and impact, then coordinate a fix or documented mitigation when appropriate. Public disclosure should wait until a fix or mitigation is available, or until coordinated timing has been agreed.

## Supported Credential Sources

Nodex supports four credential backends:

- **File.** JSON credential files under `~/.nodex/credentials/`. Written with restricted permissions (mode `0600`) through atomic temporary-file rename.
- **Keyring.** OS keyring via `github.com/zalando/go-keyring` (macOS Keychain, Linux Secret Service, Windows Credential Manager).
- **Environment variables.** `NODEX_<PROFILE>_TOKEN_ID` and `NODEX_<PROFILE>_TOKEN_SECRET`. Suitable for CI and scripts. Environment variables may be visible in process listings.
- **Stdin.** Interactive prompt, not stored. Use `--password-stdin` for scripted password input (used by commands like `access user create`, not for provider authentication).

The Proxmox provider authenticates with API tokens (`PVEAPIToken` scheme). Password-based authentication is not supported for connecting to Proxmox endpoints.

## Secret Handling Rules

Nodex enforces these rules for all credential operations:

- **No CLI arguments for passwords.** Passwords and token secrets are never accepted as command-line arguments (which would appear in shell history and process listings).
- **Hidden prompts.** Interactive password prompts do not echo input.
- **`--password-stdin`.** Passwords may be piped from stdin for automation.
- **Redaction.** Authorization headers, `PVEAPIToken` and `PBSAPIToken` values, cookie headers, CSRF tokens, and password fields are redacted from debug output, error messages, and logs before printing.
- **No secret logging.** Debug mode (`--debug`) passes all output through the redaction pipeline.
- **Terminal sanitization.** All output is sanitized for escape sequences to prevent terminal injection.

## TLS Policy

- **HTTPS required.** Endpoint URLs must use the `https://` scheme. HTTP endpoints are rejected.
- **Certificate verification.** TLS certificate and hostname verification is enabled. There is no `--insecure` flag, no configuration field to disable verification, and no hidden TLS bypass.
- **Minimum TLS 1.2.** The transport requires TLS 1.2 or newer.
- **Custom CA support.** Profiles may specify a `ca_file` to add a private CA certificate to the system trust pool. The CA file is read at connection time and is not persisted beyond the session.
- **No URL userinfo.** Endpoints containing embedded credentials (`https://user:pass@host`) are rejected.

## Safety Gates

All mutation commands are protected by a five-tier safety model:

| Tier | Name | Gate |
|------|------|------|
| 0 | Observation | None required |
| 1 | Reversible | `--yes` flag or interactive confirmation |
| 2 | Disruptive | `--yes --force` or double confirmation |
| 3 | Destructive | Type-in target verification (e.g., type the VM ID) |
| 4 | Security Admin | `--expert` flag |

Additional protections:

- **Non-interactive fail-closed.** When `--non-interactive` is set and confirmation is required, the command fails instead of proceeding silently.
- **No generic bypass.** There is no `--skip-safety` or equivalent flag. Each tier requires its specific gate.

## Read-Only Token Support

Nodex works with Proxmox API tokens that have only read permissions. For inspection-only use, create a token with the `PVEAuditor` role or a custom role with only `Sys.Audit` and `Datastore.Audit` privileges. Nodex does not require administrator privileges when narrower permissions are sufficient.

## Mutation Permissions

Each management command documents its minimum required Proxmox permissions. Common patterns:

- **VM lifecycle** (`vm start`, `vm shutdown`): `VM.PowerMgmt` on the target VM or pool
- **VM configuration** (`vm update`): `VM.Config` on the target VM
- **Backup creation**: `VM.Backup` on the target VM or pool
- **Storage content management**: `Datastore.Allocate` and `Datastore.Audit` on the target storage
- **Access management**: `Permissions.Modify` (requires `--expert`)

Use the narrowest permissions possible. Create purpose-scoped API tokens rather than using the root `root@pam` token.

## File Transfer Risks

Storage upload and download operations involve file system access:

- **Upload.** Files are streamed to the provider using `io.Pipe`. No whole-file buffering in memory.
- **Download.** Content is written to a temporary file and renamed into place with `os.Rename`. The `--overwrite` flag is required to overwrite an existing file. Partial downloads are cleaned up on error.
- **Symlinks.** Upload operations should verify file paths. Nodex treats upload paths as explicit user input.

## Output Redaction

Nodex redacts these patterns from all output before printing:

- `Authorization: ...` headers
- `Cookie: ...` headers
- `CSRFPreventionToken: ...` values
- `PVEAPIToken=...` and `PBSAPIToken=...` values
- API token secrets in request bodies
- Password fields in request bodies

Redaction is applied to debug logs, verbose output, error messages, table output, JSON output, and YAML output.

## What Nodex Does NOT Protect Against

Nodex is a local CLI and cannot protect against:

- **Compromised operator machine.** If the machine running Nodex is compromised, credentials in environment variables, keyrings, or file backends may be accessible.
- **Compromised Proxmox endpoint.** If the Proxmox server itself is compromised, API responses and task outcomes cannot be trusted.
- **Social engineering.** Nodex provides confirmation gates but cannot prevent an operator from deliberately confirming a destructive operation.
- **Physical access.** Unlocked machines with credential files or active shell sessions are vulnerable.
- **Network-level attacks below TLS.** Nodex requires TLS but cannot detect or prevent attacks at lower network layers if TLS is somehow subverted.

## Scope

**In scope:** The Nodex CLI, local configuration and credential handling, Proxmox provider request handling, output redaction and sanitization, safety authorization, CI workflow configuration, and documentation that affects security decisions.

**Out of scope unless explicitly authorized:** Testing against a live Proxmox server that you do not own, denial-of-service testing against third-party infrastructure, social engineering, and disclosure of unrelated secrets.
