# Nodex Threat Model

This document describes Nodex's security boundaries and material risks. It
complements the operator-facing [Security Policy](../SECURITY.md) and the
implementation overview in [Architecture](architecture.md). It describes
repository behavior, not a guarantee about a particular deployment or provider.

## System and trust boundaries

Nodex is a local single-process CLI. Normal provider operations connect directly
from the operator's machine to an explicitly configured PVE or PBS HTTPS
endpoint. Optional maintenance connects to explicitly enrolled Linux hosts
through a local allowlisted Ansible adapter. No Nodex daemon or managed-host
agent is installed. `monitor check` runs only explicitly configured checks and
does not create a background service or history store.

```text
Operator / automation caller
        |
        | local CLI arguments, config, receipts, credentials
        v
Nodex process  ---- HTTPS / API token ----> PVE or PBS API
        |
        +-------- allowlisted Ansible / SSH -> enrolled Linux host
        |
        +-------- explicitly configured one-shot monitoring targets
```

The operator workstation, local configuration and credential stores, remote
provider, enrolled hosts, DNS/network path, Go dependency supply chain, and
release workflow are separate trust domains. Provider responses, task logs,
monitor results and host names are data from outside the Nodex process; they
must not be interpreted as trusted instructions.

## Protected assets

- PVE and PBS API tokens and any password supplied to an individual command.
- SSH key material held by the OS agent or a referenced local key file.
- Local configuration, including endpoints, CA/SSH paths, inventory, and
  disposable certification policy.
- Infrastructure state, especially guest data, backup snapshots, identity,
  firewall/network policy, storage and cluster membership.
- Local agent and maintenance receipts, plans, and certification ledger state.
- Release binaries, checksums, signatures, SBOMs and build provenance.

## Principal threats and controls

### Credential exposure

Nodex resolves credentials from file, OS keyring, environment or stdin
references. Credential values are not accepted as ordinary provider CLI
arguments. File credentials are written atomically with restrictive permissions
on supported Unix systems; they are not application-encrypted. Environment
variables can be visible to sufficiently privileged local processes. Secrets
are redacted from diagnostic output, and terminal escape sequences are
sanitized. PVE and PBS have separate token schemes and should use separate
credentials.

### Network interception or endpoint impersonation

PVE/PBS provider endpoints must use HTTPS. TLS 1.2 or newer is required, normal
certificate and hostname validation stay enabled, and an optional custom CA is
additive to system trust. There is no general insecure-TLS switch. General
profiles do not pin a leaf certificate; disposable certification has an
explicit endpoint/certificate binding. Generic configured monitoring can
intentionally use HTTP; operators should use HTTPS where the target supports
it and avoid credentials in monitoring URLs.

### Accidental, duplicated or ambiguous mutations

The command metadata assigns each operation one of five safety tiers. Required
confirmation fails closed in non-interactive mode. Provider mutations use a
no-automatic-retry transport path; a timeout or lost response can still leave
the remote outcome unknown. `--wait` observes a provider task but does not by
itself establish the resource postcondition. Operators and automation must
distinguish submission, execution and verification, and must reconcile an
unknown outcome before considering a new attempt.

Agent mode adds request-ID deduplication and local receipts for supported
operations. Receipts are local recovery/deduplication state, not proof of
authorization or tamper-proof audit evidence. Identical request IDs do not
resubmit; changed input conflicts. Refresh/reconcile never resubmits. Agent
mode is not a sandbox against a caller that can run arbitrary local commands or
modify Nodex files.

Maintenance plans are expiring and digest-checked; receipts are written
atomically and avoid persisting raw Ansible output or credentials. Certification
uses an explicit disposable-environment allowlist and ledger. These controls
reduce accidental scope and replay, but do not replace backup verification,
operator review or provider-side least privilege.

### Excessive local process execution

The Ansible adapter accepts allowlisted operation IDs backed by playbooks
embedded in the binary. It does not accept arbitrary shell commands, modules,
playbook paths, inventory scripts or extra arguments. It validates the local
executable, uses a minimal child environment and private temporary files, pins
host-key checking, and bounds output. Ansible is only needed for its optional
maintenance workflows. These controls constrain Nodex's own invocation; they
do not make a compromised local machine or remote host trustworthy.

### Malicious or misleading provider/host data

Remote text can be stale, incomplete or hostile. Nodex applies size limits,
typed decoding where implemented, redaction and terminal sanitization. It does
not treat a provider message as authorization, a successful task as proof of
desired end state, or unavailable evidence as healthy. The agent interface
marks its data as untrusted and constrains next actions to operation IDs and
typed arguments rather than shell strings.

### Supply-chain and release compromise

Go dependency checksums are recorded in `go.sum`; CI runs module verification,
static analysis, tests and vulnerability scanning. The tag-driven release
workflow builds platform archives and publishes checksums, SPDX SBOMs, a
Sigstore bundle for the checksum file, and build provenance. Users should
verify downloaded checksums and the signature identity against the release
workflow. A compromised source repository, build runner, dependency, signing
identity or release account can still undermine artifact trust.

## Residual trust and limitations

- The local machine and its OS account are trusted. A local administrator or
  process able to read Nodex memory/files may access credentials and receipts.
- The remote provider is authoritative for its API responses and authorization;
  Nodex cannot prove that a compromised provider reports truthful state.
- Normal provider profiles rely on configured system/custom CA trust and do not
  provide general certificate pinning or mutual TLS.
- File-backed credentials are permission-protected, not encrypted at rest.
- Confirmation flags express the caller's intent; they do not authenticate a
  human approver or prove change authorization.
- Provider task records can expire and provider resource identifiers can be
  reused. Some outcomes therefore remain unknown or cannot be attributed
  uniquely to one request.
- Not every provider/operation exposes postcondition verification. The live
  operation contract states known task, verification and recovery behavior.
- Nodex is pre-1.0; command/config/output compatibility commitments are defined
  in [compatibility.md](compatibility.md), and internal Go APIs may change.

## Verification map

Security-relevant behavior has unit, fuzz, mock-provider and local HTTP contract
tests. The main implementation boundaries are `internal/credentials`,
`internal/redact`, `internal/safety`, `internal/transport/httpclient`,
`internal/agent`, `internal/ansible`, `internal/maintenance`, and both provider
client packages. See [test-coverage.md](test-coverage.md) for a repository map
and verification commands. Automated tests do not require live PVE, PBS or SSH
infrastructure; disposable-environment certification is an explicit opt-in.

Review this threat model when a new provider, mutation surface, credential
source, external process, or release workflow is introduced, and whenever a
security incident changes the assumptions above.
