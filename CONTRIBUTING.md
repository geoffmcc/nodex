# Contributing to Nodex

Thank you for improving Nodex. This project is a Go CLI for inspecting and operating self-hosted infrastructure, Proxmox-first, with a five-tier safety model for all mutation operations.

## Prerequisites

- Go 1.25.12
- `make` (optional; Go commands work directly)
- Git

CI builds and tests on Ubuntu, macOS (Apple Silicon and Intel), and Windows. The CI workflow runs `go build`, `go vet`, `go test`, and a `gofmt -s` check.

## Development Setup

```bash
git clone https://github.com/geoffmcc/nodex.git
cd nodex
go test ./...
make build
./nodex version
```

On Windows PowerShell, run the built executable as `.\nodex.exe version` when building with a Windows Go toolchain.

## Development Commands

```bash
make build      # go build with version ldflags; writes ./nodex
make test       # go test ./...
make test-race  # go test -race ./...
make vet        # go vet ./...
make fmt        # gofmt -s -w .
make lint       # gofmt check + go vet
make clean      # remove ./nodex
```

## Testing Notes

- Unit tests isolate configuration and home directories with temporary paths.
- CLI end-to-end tests use an in-process mock provider rather than a live Proxmox server.
- HTTP contract tests verify request shape without contacting real endpoints.
- Do not add tests that require a real Proxmox host unless explicitly skipped or gated behind a clear opt-in mechanism.
- Use fictional domains (`example.com`, `example.invalid`) and fake credentials in tests and documentation.
- Security tests must prove that sensitive values are redacted or omitted, not that they appear.

## Required for New Commands

When adding a new command, it must include:

1. **Operation metadata.** The command must declare its safety tier, operation description, and resource target pattern.
2. **Safety classification.** Every mutation command must declare a `safety.ConfirmationPolicy` with the correct tier and, for Tier 3+, a `TypeConfirmTarget`.
3. **Capability interface.** The command must go through a typed provider method, either on the base `domain.Provider` interface or through an optional capability interface.
4. **Typed provider method.** The Proxmox client must expose the endpoint through a typed method with structured request and response types.
5. **Output contract.** The command must produce table, JSON, and YAML output consistently. Mutation commands must use the `OperationResult` envelope.
6. **Exit-code behavior.** Errors must carry explicit exit codes through `app.ExitCoder`. The command must document which exit codes are possible.
7. **Tests.** Must include unit tests for the handler, mock-provider integration tests, and HTTP contract tests for the client method.
8. **Documentation.** Must update `docs/cli-reference.md`, `docs/nodex.1`, and any relevant architecture documentation.
9. **Least-privilege notes.** Document the narrowest Proxmox permissions required for the operation.
10. **No live-infrastructure validation.** Do not test state-changing operations against a real Proxmox host.

## Safety-Sensitive Areas

These areas require extra care and must fail closed:

- **Credential handling.** Never log, display, or store credentials in plaintext outside their designated backends.
- **TLS behavior.** No insecure TLS options. HTTPS required. Certificate verification always enabled.
- **Output redaction.** Authorization headers, tokens, passwords, and cookies must be redacted from all output.
- **Terminal sanitization.** All output must be sanitized for escape sequences.
- **Mutation transport.** Use `DoMutation()` (no retry) for POST, PUT, and DELETE. Use `Do()` (with bounded retry) only for GET.
- **Confirmation gates.** Never bypass safety tiers. Non-interactive mode must fail if confirmation is needed.
- **Task polling.** Poll with exponential backoff, bounded intervals, and context cancellation. Never tight-loop.
- **File transfers.** Stream uploads. Write downloads to temp files and rename. Clean up on error.

## Documentation Expectations

When changing user-visible behavior, update the relevant documentation in the same change:

- `README.md` for onboarding and high-level behavior
- `docs/cli-reference.md` for commands, flags, output, exit codes, and safety tiers
- `docs/configuration.md` for configuration, credentials, TLS, and paths
- `docs/architecture.md` for package boundaries, provider architecture, or new capability interfaces
- `docs/nodex.1` when CLI syntax or behavior changes
- `SECURITY.md` when security-sensitive behavior changes
- `SUPPORT.md` when supported scope changes

Verify command examples against the actual CLI. Global flags must appear before the command name: `nodex --output json node list`.

## Pull Requests

Before opening a pull request, run at least:

```bash
make lint
make test
```

Include a concise description of the behavior change, the verification performed, and any compatibility or security considerations. For mutation operations, explicitly state the safety tier and confirmation requirements.
