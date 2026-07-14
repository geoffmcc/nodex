# Remediation Log

## Group 1 — Output sanitization and redaction consistency (SEC-001, SEC-003)

- Violated invariants: all outputs must be secret-free and terminal-control-safe; direct diagnostics must not bypass centralized redaction/sanitization.
- Design: sanitize structured string values before JSON/YAML serialization and enforce sanitization at CLI writer boundary for direct `fmt` output.
- Files changed: `internal/output/terminal_data.go`, `internal/output/sanitizing_writer.go`, `internal/output/json.go`, `internal/output/yaml.go`, `internal/output/output_test.go`, `internal/cli/root.go`, `internal/cli/cli_test.go`.
- Tests added: JSON/YAML hostile string tests and CLI direct stdout/stderr wrapper test.
- Focused verification: `go test -count=1 ./internal/cli ./internal/output ./internal/redact` passed.

## Group 2 — Redirect policy cap (SEC-002)

- Violated invariants: redirects must be bounded and cannot create unbounded operation/resource use.
- Design: consolidate redirect policy in `checkRedirect` and cap `via` at 10 hops.
- Files changed: `internal/transport/httpclient/client.go`, `internal/transport/httpclient/client_test.go`.
- Tests added: same-origin redirect loop cap integration test.
- Focused verification: `go test -count=1 ./internal/transport/httpclient` passed.

## Scanner fixture cleanup

- File changed: `internal/redact/redact_test.go`.
- Purpose: avoid a redacted gitleaks false positive on a fake CSRF fixture by splitting the string literal while preserving the same runtime test input.
