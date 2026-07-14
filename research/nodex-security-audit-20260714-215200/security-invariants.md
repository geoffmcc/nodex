# Security Invariants

## Proven/remediated

- Secrets are redacted through typed `redact.Secret`, `Redactable`, regex redaction, top-level error sanitization, structured output sanitization, and CLI sanitizing writers. Tests cover table, JSON, YAML, direct stdout/stderr, and redaction fixtures.
- TLS verification is enabled by default; endpoint parsing rejects non-HTTPS, userinfo, path, query, and fragment; custom CA augments trust roots without disabling hostname verification.
- Redirects cannot downgrade HTTPS to HTTP, cannot cross origin, and now cannot exceed 10 hops (SEC-002).
- Mutations use `DoMutation` and are not retried automatically; GET/HEAD retry policy is bounded with jitter and context cancellation.
- Proxmox path segments use URL escaping; qemu/lxc methods keep VM/container endpoints separate.
- Safety tiers require explicit flags/typed confirmation/expert mode; noninteractive confirmation fails closed.
- JSON/YAML/table/direct text outputs are commentary-free for structured modes, secret-redacted, deterministic enough for tests, and hostile terminal strings are stripped before reaching sinks (SEC-001/SEC-003).
- Config and credential writes use atomic temp-file rename and restrictive permissions where supported.
- Tests use mock providers/httptest; no live Proxmox endpoint was contacted.

## Residual/live-validation invariants

- Real Proxmox task identity/state behavior, HA/backup/migration locks, and cluster identity revalidation must be validated only against an explicitly authorized disposable environment.
- Windows ACL semantics for file credential permissions are documented but cannot be fully proven from WSL tests alone.
