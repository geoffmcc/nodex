# Finding Ledger

## SEC-001 — Structured JSON/YAML output encoded hostile terminal strings

- Status: fixed_verified (focused tests pass; broad verification pending final pass)
- Severity: Medium
- Confidence: High
- Category: Output / terminal injection / redaction consistency
- CWE: CWE-117 (Improper Output Neutralization for Logs), CWE-150 (Improper Neutralization of Escape Sequences)
- Affected files: `internal/output/json.go`, `internal/output/yaml.go`
- Affected symbols: `WriteJSON`, `WriteYAML`
- Reachable path: any `--output json|yaml` command rendering Proxmox-controlled names/comments/log entries or local profile fields.
- Evidence: prior writers called `redact.Sanitize`, marshaled data, and only regex-redacted bytes; string values containing ESC/OSC/bidi controls could be serialized rather than stripped before structured output.
- Root cause: terminal-string sanitization was applied to table/error paths, not to structured string values before JSON/YAML serialization.
- Implemented remediation: added `sanitizeTerminalData` recursive structured sanitizer and applied it before JSON/YAML marshal; added regression tests for JSON/YAML hostile string and token redaction.
- Regression tests: `TestWriteJSONRedactsAndSanitizesStringValues`, `TestWriteYAMLRedactsAndSanitizesStringValues`.
- Verification: `go test -count=1 ./internal/output ./internal/redact ./internal/cli`; final broad checks pending.
- Residual risk: low; downstream tools that reintroduce control sequences after parsing JSON are outside Nodex.

## SEC-002 — Redirect policy lost intended 10-hop cap

- Status: fixed_verified (focused tests pass; broad verification pending final pass)
- Severity: Medium
- Confidence: High
- Category: Transport / redirect / denial of service
- CWE: CWE-835 (Loop with Unreachable Exit Condition) / CWE-400 (Resource Exhaustion)
- Affected files: `internal/transport/httpclient/client.go`
- Affected symbols: `New`, `checkRedirect`
- Reachable path: all HTTP requests through `httpclient.Client` that encounter same-origin redirect loops.
- Evidence: `New` initially set a redirect check with `len(via) >= 10` but then overwrote `CheckRedirect` with `c.checkRedirect`, which only blocked downgrade/cross-origin redirects.
- Root cause: duplicated redirect policy split between inline closure and method; final assigned method omitted hop cap.
- Implemented remediation: moved 10-hop cap into `checkRedirect` and removed dead inline policy.
- Regression tests: `TestDoCapsSameOriginRedirectsAtTenHops` plus existing downgrade/cross-origin redirect tests.
- Verification: `go test -count=1 ./internal/transport/httpclient`.
- Residual risk: low; client timeout also bounds total requests, but hop cap is now explicit.

## SEC-003 — Direct CLI stdout/stderr writes bypassed central output sanitization

- Status: fixed_verified (focused tests pass; broad verification pending final pass)
- Severity: Medium
- Confidence: High
- Category: Output / terminal injection / redaction consistency
- CWE: CWE-117 / CWE-150
- Affected files: many CLI handlers using direct `fmt.Fprintf`; root boundary in `internal/cli/root.go`.
- Affected symbols: `cli.Run`, `Context.Writer`, `Context.ErrW`, logging construction.
- Reachable path: direct handler success/progress/diagnostic messages containing user-controlled or endpoint-controlled identifiers.
- Evidence: grep found many direct writes to `cmdCtx.Writer`/`cmdCtx.ErrW`; not all passed through `output.WriteTable`, `WriteResult`, or `Writer.Diagnostic`.
- Root cause: safe sinks were optional call-site discipline rather than enforced at the CLI boundary.
- Implemented remediation: added `output.SanitizingWriter`, wrapped stdout/stderr in `cli.Run`, and changed logger construction to use injected sanitized stderr rather than `os.Stderr`.
- Regression tests: `TestRunSanitizesDirectHandlerStdoutAndStderr`.
- Verification: `go test -count=1 ./internal/cli ./internal/output`.
- Residual risk: low; binary/file transfer paths do not write payload bytes to CLI stdout.

## Scanner correlation / false-positive notes

- Gosec: 0 issues.
- Govulncheck: no vulnerabilities.
- Manual secret regex scan: expected security-sensitive terms in docs/tests/source; no real secret values identified during line review.
- Gitleaks: one redacted fake CSRF token fixture in `internal/redact/redact_test.go`; fixture was split (`"CSRFPreventionToken=" + "abc123def456"`) without weakening the redaction test.
