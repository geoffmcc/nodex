# Nodex Threat Model

This document describes the security threats facing Nodex, the mitigations in
place, and the gaps that remain. It supplements the security policy in
[SECURITY.md](../SECURITY.md) and the architecture documentation in
[docs/architecture.md](architecture.md).

## Scope

Nodex is a single-process local CLI that connects directly to Proxmox VE
endpoints over HTTPS. The threat model covers:

- The Nodex binary, source code, configuration, and local credential storage.
- Communication between Nodex and Proxmox endpoints.
- The CI/CD pipeline that builds and tests Nodex.
- The dependency supply chain.
- The local operator workstation.

**Out of scope:** The Proxmox VE server itself (Nodex cannot protect a
compromised server), physical security of the operator's machine, social
engineering, and network attacks below the TLS layer.

---

## Assets

| Asset | Location | Sensitivity |
|-------|----------|-------------|
| Source code | GitHub repository | Public |
| Configuration files | `~/.config/nodex/config.yaml` (Linux), `~/Library/Application Support/Nodex/config.yaml` (macOS), `%AppData%\Nodex\config.yaml` (Windows) | Low (no credentials stored here) |
| Credential files | `~/.nodex/credentials/` | **High** — contains API tokens/passwords |
| OS keyring entries | System credential store | **High** — contains API tokens/passwords |
| Environment variables | Process memory | **High** — may contain tokens |
| Release binaries | GitHub Releases, `go install` | **Medium** — integrity matters |
| CI/CD secrets | GitHub Actions secrets | **Critical** — controls publishing |
| go.sum | Repository | **Medium** — dependency integrity anchor |

---

## Attacker Profiles

### 1. Compromised Proxmox Endpoint
The Proxmox server is under attacker control or serving malicious responses.

**Impact:** Attacker can return falsified inspection data, reject legitimate
operations, or respond to read requests with crafted payloads.

**Existing mitigations:**
- TLS certificate verification ensures endpoint identity (no `--insecure` flag).
- Body size limits (50 MiB success, 256 KiB error) prevent memory exhaustion.
- Responses are decoded into typed Go structs, limiting attack surface.

**Gaps:**
- No response content-type validation beyond what the Go HTTP client provides.
- JSON responses are accepted without schema validation.

### 2. Network Man-in-the-Middle
An attacker intercepts HTTPS traffic between Nodex and the Proxmox endpoint.

**Impact:** Credential theft, traffic inspection, request/response modification.

**Existing mitigations:**
- HTTPS required; HTTP endpoints are rejected at the URL validation layer.
- TLS 1.2 minimum; certificate and hostname verification always enabled.
- No `--insecure` flag, no hidden TLS bypass, no `InsecureSkipVerify`.
- Custom CA support is explicit per profile via `ca_file`.

**Gaps:**
- No certificate pinning (relies on system trust store).
- No mutual TLS support.

### 3. Malicious Certificate Authority
A compromised or untrustworthy CA in the system trust store issues a valid
certificate for the Proxmox endpoint's hostname.

**Impact:** Attacker can impersonate the Proxmox endpoint with a valid
certificate.

**Existing mitigations:**
- TLS certificate verification uses the system trust pool.
- Custom CA support allows operators to narrow trust to a private CA.

**Gaps:**
- No certificate pinning or known-hosts mechanism.
- No warning when the CA changes between connections.

### 4. Stolen Credentials
API tokens, passwords, or authorization headers are exposed through logs,
errors, shell history, process listings, or backup files.

**Impact:** Attacker gains access to the Proxmox endpoint with the stolen
credential's privileges.

**Existing mitigations:**
- Redaction pipeline strips `Authorization`, `Cookie`, `CSRFPreventionToken`,
  `PVEAPIToken`, and password fields from all output (debug, verbose, error,
  table, JSON, YAML).
- Credentials are never accepted as command-line arguments (no shell history
  exposure).
- Interactive password prompts do not echo.
- `--password-stdin` for scripted authentication.
- Atomic file writes with mode `0600` for credential files.
- Config directory created with mode `0700`.
- `.gitignore` blocks `.env`, `*.pem`, `*.key`, `*.p12`, `*.pfx`.

**Gaps:**
- Environment variables may be visible in process listings (`/proc` on Linux,
  `ps` on macOS).
- Keyring backends depend on OS-specific security; Windows Credential Manager
  and Linux Secret Service have their own threat models.
- No automatic credential rotation or expiry.

### 5. Local Untrusted User
Another user on the same machine attempts to read Nodex credential files or
intercept keyring access.

**Impact:** Credential theft from file or keyring backends.

**Existing mitigations:**
- Credential files written with mode `0600` (owner read/write only).
- Config directory created with mode `0700`.
- Keyring backends delegate to OS-level access controls.

**Gaps:**
- No encryption-at-rest for file credentials (plain JSON on disk).
- File permissions rely on OS enforcement; no application-layer encryption.

### 6. Malicious Pull Request
An external contributor submits a PR containing malicious code, credential
extraction, or CI/CD compromise.

**Impact:** Backdoor in source code, credential exfiltration in CI, or test
tampering.

**Existing mitigations:**
- CI runs on every PR with `gofmt`, `go vet`, `staticcheck`, and full test suite.
- `govulncheck` runs in CI.
- CI has `permissions: contents: read` — no write access to repository or
  releases.
- GitGuardian scans for secrets in commits.
- PR review required before merge (process, not enforced by tooling).

**Gaps:**
- No required reviewer enforcement in branch protection (repository setting,
  not code).
- CI tools installed with `@latest` — a compromised upstream release could
  inject through `staticcheck` or `govulncheck`.
- No workflow approval requirement for first-time contributors.

### 7. Dependency Compromise
A direct or transitive dependency is compromised (typosquatting, account
takeover, malicious update).

**Impact:** Malicious code executed at build time, test time, or runtime.

**Existing mitigations:**
- `go.sum` locks dependency checksums; verified by `go mod verify`.
- `govulncheck` scans for known vulnerabilities in CI (currently clean).
- Only 6 dependencies (all transitive through `go-keyring`), minimizing attack
  surface.
- No `replace` directives in `go.mod`.
- All dependencies are from well-known sources (`golang.org/x`, `github.com`).

**Gaps:**
- No automated dependency update tooling (Dependabot, Renovate).
- No SBOM (Software Bill of Materials) generated.
- No provenance attestation for builds.
- `go.sum` is not explicitly verified as a CI step before building.

### 8. CI/CD Compromise
The GitHub Actions workflow or runner is compromised, allowing tampering with
build artifacts or exfiltration of secrets.

**Impact:** Malicious release binaries, credential theft, repository tampering.

**Existing mitigations:**
- Actions pinned to commit SHAs (`actions/checkout@93cb6ef...`,
  `actions/setup-go@924ae3a...`), preventing tag mutation attacks.
- CI permissions restricted to `contents: read`.
- No secrets used in the primary build/test workflow.
- GitHub-hosted runners are ephemeral.

**Gaps:**
- No SLSA provenance generation for builds.
- No code signing for release binaries.
- No reproducible build configuration.
- No `id-token` permission for keyless signing integration.
- No Step Security Harden Runner step.

### 9. Release Tampering
An attacker modifies a release binary after build but before distribution.

**Impact:** Users download and run a compromised binary.

**Existing mitigations:**
- Users can build from source with `go install`.
- Source is in a public Git repository with signed commits.

**Gaps:**
- No binary signing (no GPG, Cosign, or Sigstore signatures).
- No checksums published for release binaries.
- No SBOM to verify component provenance.
- No reproducible builds to independently verify binary integrity.

---

## Trust Boundaries

```text
┌─────────────────────┐       HTTPS (TLS 1.2+)       ┌──────────────────────┐
│   User Workstation  │ ──────────────────────────────│  Proxmox VE Server  │
│                     │                                │                      │
│  ┌───────────────┐  │                                └──────────────────────┘
│  │ Nodex binary  │  │
│  │               │  │
│  │  ┌─────────┐  │  │
│  │  │ Config  │  │  │
│  │  └─────────┘  │  │
│  │  ┌─────────┐  │  │
│  │  │Creds    │  │  │
│  │  └─────────┘  │  │
│  └───────────────┘  │
│         │           │
│    ┌────┴────┐      │
│    │ OS      │      │
│    │ Keyring │      │
│    └─────────┘      │
└─────────────────────┘

┌──────────────────────┐      go modules       ┌──────────────────────┐
│   GitHub Actions CI  │ ──────────────────────│  Module Proxies      │
│                      │                        │  (proxy.golang.org)  │
│  ┌────────────────┐  │                        └──────────────────────┘
│  │ CI Secrets     │  │
│  └────────────────┘  │
│  ┌────────────────┐  │
│  │ Build Artifacts│  │
│  └────────────────┘  │
└──────────────────────┘
```

### Boundary Security Controls

| Boundary | Control |
|----------|---------|
| Nodex → Proxmox | TLS 1.2+, HTTPS required, cert verification, no insecure mode, body size limits, DoMutation no-retry |
| Nodex → Credential files | Mode `0600` files, mode `0700` directory, atomic writes, name validation |
| Nodex → OS Keyring | Delegated to OS access controls |
| Nodex → Environment | Read-only; env vars may be visible in process listings |
| CI → Dependencies | `go.sum` checksums, `govulncheck` scanning, `go mod verify` |
| CI → Repository | `contents: read` permission, action SHA pinning, ephemeral runners |
| Developer → CI | PR-based workflow, GitGuardian secret scanning |

---

## Mitigation Inventory

### Implemented

| # | Mitigation | Covers |
|---|-----------|--------|
| M1 | HTTPS-only with TLS 1.2+ | Network MITM, credential theft in transit |
| M2 | Certificate verification (no `--insecure`) | Malicious endpoint, MITM |
| M3 | Custom CA support per profile | Private CA environments |
| M4 | No URL userinfo in endpoints | Credential leakage in URLs |
| M5 | Secret redaction pipeline | Credential exposure in output |
| M6 | No CLI-argument passwords | Shell history exposure |
| M7 | Hidden password prompts | Shoulder surfing |
| M8 | Atomic credential writes (mode `0600`) | Local file access |
| M9 | Config directory (mode `0700`) | Local file access |
| M10 | Five-tier safety model | Unintended mutations |
| M11 | Non-interactive fail-closed | Scripted bypass prevention |
| M12 | `DoMutation()` never retries | Duplicate mutations |
| M13 | `Do()` bounded retry (2 attempts, jitter) | Transient network errors |
| M14 | Body size limits (50 MiB / 256 KiB) | Memory exhaustion |
| M15 | Terminal escape sanitization | Terminal injection |
| M16 | Signal handling with distinct exit codes | Clean cancellation |
| M17 | Path validation for file transfers | Path traversal |
| M18 | Streaming uploads via `io.Pipe` | Memory exhaustion on upload |
| M19 | Atomic downloads via temp file + rename | Partial file corruption |
| M20 | `go.sum` integrity | Dependency tampering |
| M21 | `govulncheck` in CI | Known vulnerability detection |
| M22 | CI actions pinned to SHAs | Action tag mutation |
| M23 | CI `contents: read` permission | CI token scope |
| M24 | GitGuardian secret scanning | Credential leaks in commits |
| M25 | `.gitignore` for secrets (`.env`, `*.pem`, `*.key`) | Accidental credential commit |

### Not Yet Implemented

| # | Gap | Priority | Notes |
|---|-----|----------|-------|
| G1 | SBOM generation | Medium | `go version -m` provides basic info; full SBOM (SPDX/CycloneDX) recommended |
| G2 | Binary signing | Medium | Cosign/Sigstore for release binaries |
| G3 | Reproducible builds | Low | Requires build environment standardization |
| G4 | CI tool version pinning | High | `staticcheck` and `govulncheck` use `@latest` |
| G5 | SLSA provenance | Medium | Build provenance attestation |
| G6 | Certificate pinning | Low | Adds operational complexity; custom CA covers most cases |
| G7 | Encryption-at-rest for file credentials | Low | OS-level permissions are primary control |
| G8 | Dependency update automation | Medium | Dependabot/Renovate for automated updates |
| G9 | Fuzzing in CI | Low | Go native fuzzing for parsing/input handlers |
| G10 | `go.sum` verification in CI | Medium | Explicit `go mod verify` step before build |

---

## Test Coverage Mapping to Threats

| Threat | Relevant Tests |
|--------|---------------|
| Credential exposure | `internal/redact/redact_test.go` — redaction pattern tests |
| Credential exposure | `internal/cli/` — auth header tests, credential resolution tests |
| Network MITM | `internal/transport/httpclient/` — TLS config, HTTPS enforcement, cert verification |
| Network MITM | `internal/config/` — endpoint validation, HTTP rejection |
| Malicious endpoint | `internal/transport/httpclient/` — body size limits, typed decoding |
| Malicious endpoint | `internal/provider/proxmox/client/` — response contract tests |
| Unintended mutations | `internal/safety/safety_test.go` — tier classification, confirmation policy |
| Unintended mutations | `internal/cli/` — command safety classification tests |
| Path traversal | `internal/pathvalidate/` — path validation tests |
| File transfer | `internal/atomicwrite/` — atomic write tests |
| Task polling safety | `internal/task/` — exponential backoff, timeout, cancellation tests |
| Terminal injection | `internal/output/` — sanitization tests |
| Exit code behavior | `internal/app/` — exit code tests |
| Configuration integrity | `internal/config/` — schema validation, atomic write, lock tests |
| Credential integrity | `internal/credentials/` — backend resolution, validation tests |

---

## Residual Risks

See [docs/residual-risks.md](residual-risks.md) for the structured residual risk
register with accepted risks, deferred fixes, assumptions, and manual
verification needs.

---

## Review Cycle

This threat model should be reviewed:

- When a new capability is added (especially mutation operations).
- When a new dependency is introduced.
- When the CI/CD pipeline changes.
- When a security incident occurs.
- At least once per major development cycle.

The [residual risk register](residual-risks.md) should be updated whenever a gap
is closed or a new gap is identified.
