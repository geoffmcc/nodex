# Architecture and Data Flow

## Repository identity

- Git root: `C:/Users/geoff/Projects/nodex`
- Branch: `security-audit-20260714`
- Module: `github.com/geoffmcc/nodex`
- Entry point: `cmd/nodex/main.go`
- Product markers: `README.md`, `docs/product_requirements.md`, command name `nodex`, Proxmox-first provider.

## Command and configuration flow

`cmd/nodex/main.go` creates a cancellation context for SIGINT/SIGTERM, calls `cli.Run`, redacts and terminal-sanitizes top-level errors, emits JSON error objects when `--output json` is requested, and maps typed application errors to exit codes.

`internal/cli/root.go` registers the command tree and parses global flags: `--profile`, `--output`, `--timeout`, `--limit`, `--all`, `--no-color`, `--non-interactive`, `--quiet`, `--verbose`, `--debug`, `--yes`, `--force`, `--wait`, `--expert`, and `--password-stdin`. The audit added a sanitizing writer wrapper at the CLI boundary so direct handler writes are redacted and stripped of terminal controls.

Configuration is loaded from platform config paths in `internal/config`, validates profile names/endpoints, writes through locked atomic temp-file rename with `0700` config directories and `0600` files, and rejects endpoint userinfo/path/query/fragment/non-HTTPS. Credential backends are environment, file, keyring, and stdin. Credential-file names reject traversal/absolute/path-separator forms and file writes are atomic with restricted permissions.

## Provider and transport flow

The Proxmox provider implements domain capability interfaces and delegates typed operations to `internal/provider/proxmox/client`. Endpoints are normalized to HTTPS origins, paths are assembled with `url.PathEscape`/`url.Values`, Authorization is sent as `PVEAPIToken` only when token ID and secret are configured, and mutating requests validate host identity before dispatch.

`internal/transport/httpclient` enforces TLS 1.2 minimum, response and error-body size limits, cancellation contexts, GET/HEAD-only default retry, no automatic mutation retry via `DoMutation`, same-origin/no-downgrade redirect policy, and (fixed in SEC-002) a 10-hop redirect cap.

## Output/data-flow boundaries

Structured JSON/YAML output flows through `output.WriteJSON`/`WriteYAML`, which now redacts and strips terminal-control strings before serialization (SEC-001) and applies regex redaction after serialization. Table output sanitizes cells. Direct stdout/stderr writes are protected by `output.SanitizingWriter` at `cli.Run` (SEC-003). Diagnostics go to stderr; primary command output goes to stdout.

## Proxmox operation inventory

`proxmox-operation-inventory.tsv` was generated from current client source and reconciled against the CLI dispatch tree. It contains 126 Proxmox client request rows covering GET/POST/PUT/DELETE request paths. Helper-agent CLI reconstruction identified compatibility/default dispatch paths that do not create additional Proxmox endpoints: `access users|groups|roles|domains` default list, `firewall options` default show, and `firewall ipset <name>` show shorthand. No unexplained client request path remains outside the inventory; endpoint rows use `{node}`, `{vmid}`, `{storage}`, `{id}` placeholders for encoded path segments.
