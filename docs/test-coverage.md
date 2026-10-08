# Verification and Test Map

This is a guide to the repository's verification layers, not a generated
per-operation coverage guarantee. Test files and exact coverage evolve with the
code; run the commands below against the checkout you intend to evaluate.

## Standard local checks

```sh
go mod verify
go build ./cmd/nodex/
go vet ./...
go test ./...
go test -race ./...
```

Run `make lint` for formatting and `go vet`, and consult `.github/workflows/ci.yml`
for the current CI-only checks and platform matrix. CI additionally runs
Staticcheck, `govulncheck`, actionlint/shellcheck for workflow files, release
configuration validation and cross-platform builds/tests. The pinned tool
versions and CI runners are maintained in the workflow itself.

## Test layers

- **Unit tests** validate parsing, configuration/schema versions, credentials,
  redaction, output, safety policies, task polling, plan/receipt integrity and
  provider response mapping.
- **Mock-provider CLI tests** exercise command parsing, profile selection,
  confirmation gates, structured output and failures without a live PVE/PBS
  service.
- **HTTP contract tests** use local test servers to assert request paths,
  methods, headers, payloads, TLS policy and retry behavior. They do not send
  mutations to real infrastructure.
- **Fuzz tests** target untrusted parsers and validation boundaries, including
  configuration, credentials, UPIDs, redaction and CLI inputs.
- **Cross-platform checks** compile/test on the operating systems configured in
  CI. Release archives target Linux, Windows and macOS on amd64 and arm64.
- **Opt-in certification** is a separate workflow for a declared disposable
  PVE environment. It is not part of the ordinary test suite and must not target
  production infrastructure; see [Certification](certification.md).

## Source map

| Concern | Main implementation/tests |
|---|---|
| CLI registration, operation metadata, help and output | `internal/cli/`, especially `root.go`, `operations.go`, and CLI tests |
| Agent contract, schemas and mutation receipts | `internal/agent/`, `internal/cli/agent_*.go`, `docs/agent/schemas/` |
| PVE provider and typed API client | `internal/provider/proxmox/` and `internal/provider/proxmox/client/` |
| PBS provider and typed API client | `internal/provider/pbs/` and `internal/provider/pbs/client/` |
| Configuration and platform paths | `internal/config/` |
| Credential sources and validation | `internal/credentials/` |
| Transport and TLS/retry behavior | `internal/transport/httpclient/` |
| Confirmation and safety tiers | `internal/safety/` and CLI operation tests |
| Task parsing/polling | `internal/task/` |
| Maintenance plans and receipts | `internal/maintenance/`, `internal/ansible/`, and maintenance CLI tests |
| Combined PVE/PBS health | `internal/backuphealth/` |
| One-shot monitoring | `internal/monitor/` and monitor CLI tests |

## Scope of evidence

Passing automated tests establish behavior against the code and fixtures in
that checkout. They do not prove permissions on a real provider, suitability
for a particular infrastructure environment, or that a remote asynchronous
task completed. For a mutation, distinguish request acceptance, task result,
and any explicit postcondition verification. Live disposable-environment
certification is an opt-in extra check, not a substitute for review or a
production safety guarantee.
