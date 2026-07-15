# Nodex Residual Risk Register

This document catalogues known limitations, accepted risks, deferred fixes,
unsupported environments, assumptions, and manual verification needs for Nodex.
It is the companion to [docs/threat-model.md](threat-model.md) and should be
updated whenever a gap is closed or a new one identified.

---

## Known Limitations

### L1 — No Binary Signing

**Status:** Accepted for pre-1.0.
**Severity:** Medium.
**Description:** Release binaries are not signed with GPG, Cosign, or Sigstore.
Users who install via `go install` build from source and benefit from Go module
checksums. Users who download pre-built binaries have no cryptographic
verification of origin or integrity.
**Mitigation:** Build from source with `go install github.com/geoffmcc/nodex/cmd/nodex@latest`.
**Planned:** Add Cosign keyless signing for GitHub Releases before v1.0.

### L2 — No SBOM

**Status:** Accepted for pre-1.0.
**Severity:** Low.
**Description:** No Software Bill of Materials (SPDX or CycloneDX) is generated
or published. Dependency information is available via `go version -m ./nodex`
for Go-built binaries, but structured SBOMs are not produced.
**Mitigation:** `go.sum` provides dependency checksums; `govulncheck` scans for
known vulnerabilities. Users can run `go version -m` on the binary.
**Planned:** Add SBOM generation (likely via `goreleaser` or `ko`) before v1.0.

### L3 — No Reproducible Builds

**Status:** Accepted for pre-1.0.
**Severity:** Low.
**Description:** Builds are not configured for reproducibility (`-trimpath`,
`CGO_ENABLED=0`, fixed timestamps). Build artifacts may differ between
environments.
**Mitigation:** Source is public and verifiable. `go.sum` anchors dependency
versions.
**Planned:** Evaluate reproducible build configuration before v1.0.

### L4 — No Encryption-at-Rest for File Credentials

**Status:** Accepted.
**Severity:** Low.
**Description:** Credential files stored under `~/.nodex/credentials/` are
plain JSON with restrictive file permissions (`0600`). They are not encrypted
at the application layer.
**Mitigation:** OS file permissions restrict access to the file owner. Users
who need stronger protection should use the OS keyring backend, which delegates
encryption to the operating system.
**Rationale:** Application-layer encryption adds key management complexity
(key storage, rotation) without meaningfully improving security when the OS
already enforces file ownership. The keyring backend is available for users
who need encryption-at-rest.

### L5 — Environment Variable Visibility

**Status:** Accepted.
**Severity:** Low.
**Description:** Credentials stored in environment variables
(`NODEX_<PROFILE>_TOKEN_SECRET`) may be visible in process listings
(`/proc/<pid>/environ` on Linux, `ps eww` on macOS).
**Mitigation:** This is inherent to environment-variable-based credential
delivery. Users in high-security environments should use the file or keyring
backends instead. Environment variables are documented as suitable for CI/CD
and scripting where process isolation is already trusted.
**Planned:** None. This is a fundamental property of environment variables.

### L6 — No Certificate Pinning

**Status:** Accepted.
**Severity:** Low.
**Description:** Nodex relies on the system trust store for TLS certificate
validation. There is no mechanism to pin a specific certificate or public key
for a Proxmox endpoint.
**Mitigation:** Custom CA support allows operators to narrow trust to a private
CA. Certificate validation is always enabled with no bypass.
**Rationale:** Certificate pinning adds operational complexity (rotation,
recovery) and is rarely needed when the system trust store is well-managed.
**Planned:** Evaluate optional SSH-style known-hosts mechanism post-1.0.

### L7 — No Mutual TLS

**Status:** Accepted.
**Severity:** Low.
**Description:** Nodex authenticates to Proxmox with API tokens or passwords.
Proxmox authenticates to Nodex with TLS server certificates. Mutual TLS
(client certificate authentication) is not supported.
**Mitigation:** Proxmox VE does not natively support mTLS for API
authentication; API tokens are the recommended mechanism.
**Planned:** None unless Proxmox adds mTLS support.

### L8 — JSON Response Without Schema Validation

**Status:** Accepted.
**Severity:** Low.
**Description:** Proxmox API responses are decoded into typed Go structs but
not validated against a formal JSON Schema. Malformed or unexpected fields
are silently ignored by Go's JSON decoder.
**Mitigation:** Go's typed struct decoding prevents type confusion. Unknown
fields are ignored by default, which is safe (additive API changes won't break
the client).
**Planned:** None. Schema validation would add overhead without clear benefit
given the typed struct approach.

---

## Accepted Risks

### AR1 — Single Maintainer

**Risk:** The project has a single maintainer. Bus factor is 1.
**Acceptance rationale:** The project is pre-1.0 and intentionally focused.
Code is public and forkable under the project license.

### AR2 — No SLSA Provenance

**Risk:** Build provenance is not attested. A compromised CI runner could
produce tampered binaries without detection.
**Acceptance rationale:** The primary distribution channel (`go install`)
builds from source with Go module verification. Pre-built binaries are
secondary. SLSA L3 provenance will be evaluated before v1.0.

### AR3 — go-keyring Transitive Dependencies

**Risk:** The `github.com/zalando/go-keyring` package brings in
platform-specific dependencies (`wincred` for Windows, `godbus/dbus` for
Linux). A vulnerability in any transitive dependency could affect Nodex.
**Acceptance rationale:** These are well-known, widely-used packages. All
dependencies are pinned in `go.sum` and scanned with `govulncheck` (currently
clean). The attack surface of 6 total dependencies is very small.

### AR4 — No Workflow Approval for First-Time Contributors

**Risk:** A first-time contributor could submit a PR that exfiltrates secrets
or compromises the CI runner.
**Acceptance rationale:** CI has `contents: read` only. No secrets are
available in the primary build/test workflow. GitGuardian scans for credential
leaks. PR review gates exist as a process control.

---

## Deferred Fixes

### DF1 — CI Tool Version Pinning

**Status:** Deferred to this workstream (Workstream 6).
**Description:** `staticcheck` and `govulncheck` are installed with `@latest`
in CI, creating a moving-target supply chain risk.
**Action:** Pin to specific versions (`staticcheck@v0.6.1`,
`govulncheck@v1.6.0`).
**Target:** Completed in this workstream.

### DF2 — go.sum Verification in CI

**Status:** Deferred to this workstream (Workstream 6).
**Description:** `go mod verify` is not run as an explicit CI step.
**Action:** Add `go mod verify` to CI workflow.
**Target:** Completed in this workstream.

### DF3 — Dependabot / Renovate Configuration

**Status:** Deferred to post-1.0.
**Description:** No automated dependency update tooling is configured.
**Rationale:** With only 6 dependencies and a small update surface, manual
updates are manageable pre-1.0. Automated updates will be configured before
the first stable release.
**Target:** Before v1.0.

### DF4 — Fuzzing

**Status:** Deferred to post-1.0.
**Description:** No fuzz tests exist for parsing functions (UPID parsing,
config parsing, endpoint validation).
**Rationale:** The current input surface is small and well-covered by unit
tests. Fuzzing will be added for critical parsing paths before v1.0.
**Target:** Before v1.0.

---

## Unsupported Environments

| Environment | Reason |
|-------------|--------|
| Go < 1.25.12 | Minimum Go version. Older versions are not tested. |
| HTTP (non-TLS) Proxmox endpoints | Endpoint validation rejects `http://` URLs. HTTPS is required. |
| Proxmox VE < 8.0 | Not tested. May work but compatibility is not asserted. |
| Non-x86/ARM64 architectures | Only amd64 and arm64 are tested in CI (via ubuntu, macOS-x64, macOS-arm64, and Windows runners). |
| Plan 9, Solaris, AIX | Go may cross-compile but these platforms are not tested. |

---

## Live-Test Fixture Limitations

### LF1 — Guest-Agent Fixture Requires Guest-Side Setup

**Status:** Documented.
**Severity:** Low.
**Description:** Proxmox VM configuration `agent=1` only enables the virtual
guest-agent channel. It does not install or start `qemu-guest-agent` inside the
guest OS. Live tests that require guest-agent behavior need a booted guest image
with the package installed and the service running.
**Mitigation:** Use an explicitly disposable cloud image fixture with cloud-init
or a NoCloud seed ISO that installs and starts `qemu-guest-agent`. Verify
`/nodes/<node>/qemu/<vmid>/agent/ping` before making guest-agent assertions.

---

## Assumptions

1. **The operator's machine is not compromised.** Nodex assumes the local
   filesystem, process memory, and OS keyring are trusted. No local security
   control can protect against a compromised host.

2. **The system trust store is correctly managed.** TLS certificate validation
   depends on the system CA bundle. A compromised CA in the trust store
   undermines all TLS guarantees.

3. **The Proxmox endpoint is the intended target.** Nodex connects to the
   endpoint configured in the profile. It cannot detect if a user
   misconfigures the endpoint to point to a different server.

4. **API tokens have least privilege.** Nodex documents the permissions needed
   for each operation but does not enforce them. It is the operator's
   responsibility to create tokens with appropriate roles.

5. **GitHub Actions runners are trustworthy.** CI builds depend on
   GitHub-hosted runners. A compromised runner could produce tampered
   binaries or exfiltrate source code (which is public anyway).

6. **Go module proxy is trustworthy.** `proxy.golang.org` and `sum.golang.org`
   serve dependency code and checksums. Compromise of these services would
   undermine dependency integrity for all Go projects, not just Nodex.

7. **GitHub is the authoritative source.** The `github.com/geoffmcc/nodex`
   repository is the canonical source. There are no mirrors or alternative
   distribution channels.

---

## Manual Verification Needs

These checks cannot be fully automated and require periodic manual review:

| Check | Frequency | Method |
|-------|-----------|--------|
| Dependency maintenance status | Quarterly | Review each dependency's repository for activity, open security issues, and release cadence |
| go-keyring upstream changes | Quarterly | Review changelog for security-relevant changes |
| GitHub Actions SHA validity | Monthly | Verify pinned SHAs still correspond to expected tag versions |
| Branch protection settings | After repository changes | Verify required reviews, status checks, and branch restrictions |
| GitGuardian alert review | Per alert | Triage each alert; rotate credentials if real exposure occurred |
| go.sum consistency | After dependency updates | `go mod verify` after every `go get` or `go mod tidy` |
| CI runner image changes | Per GitHub announcement | Review `ubuntu-latest`, `macos-15`, `windows-latest` image updates for Go toolchain changes |
| Proxmox API version compatibility | Per Proxmox release | Review Proxmox changelog for API breaking changes or new endpoints |

---

## Dependency Audit (Workstream 6)

### Direct Dependencies

Nodex has **no direct dependencies** outside the Go standard library. All six
`require` entries in `go.mod` are transitive dependencies brought in by
`github.com/zalando/go-keyring`.

### Transitive Dependency Inventory

| Package | Version | Purpose | Platform | Maintenance |
|---------|---------|---------|----------|-------------|
| `github.com/zalando/go-keyring` | v0.2.8 | Cross-platform OS keyring access | All | Active (last release 2024) |
| `github.com/danieljoos/wincred` | v1.2.3 | Windows Credential Manager access | Windows | Stable |
| `github.com/godbus/dbus/v5` | v5.2.2 | D-Bus communication (Secret Service) | Linux | Active |
| `golang.org/x/sys` | v0.47.0 | Low-level OS system calls | All | Active (Go team) |
| `golang.org/x/term` | v0.45.0 | Terminal handling | All | Active (Go team) |
| `gopkg.in/yaml.v3` | v3.0.1 | YAML parsing (config files) | All | Stable |

### Verification Results

- `go mod verify`: **All modules verified** ✓
- `govulncheck ./...`: **No vulnerabilities found** ✓
- No `replace` directives in `go.mod` ✓
- All dependencies from well-known sources ✓

### Dependency Update Policy (Recommended)

1. Run `govulncheck ./...` before every release.
2. Update dependencies monthly with `go get -u ./...` and run full test suite.
3. Review changelogs for `go-keyring`, `wincred`, and `godbus/dbus` before
   updating, as these handle credential operations.
4. Pin CI tool versions (`staticcheck`, `govulncheck`) to specific releases
   rather than `@latest`.
5. Configure Dependabot or Renovate for automated update PRs before v1.0.

---

## Secret Scanning

### GitGuardian

GitGuardian Security Checks are integrated into the repository. They scan:

- All commits pushed to any branch.
- Pull request diffs.
- New commits in active branches.

GitGuardian detects:
- API tokens and keys.
- Private keys (RSA, ECDSA, Ed25519).
- Database connection strings.
- Cloud provider credentials.
- Other secret patterns.

### Repository Protections

- `.gitignore` blocks: `.env`, `.env.*`, `*.pem`, `*.key`, `*.p12`, `*.pfx`.
- Test fixtures use fictional domains and fake credentials.
- Documentation examples use placeholder values.

### Exclusions

No files or patterns are excluded from GitGuardian scanning. If false positives
require exclusions, they should be documented here with rationale.

---

## SBOM / Provenance Recommendations

### Current State

No SBOM is generated. No provenance attestation exists. Release binaries are
unsigned.

### Recommended Approach

For Go projects, several options exist:

1. **`go version -m ./nodex`** — Already works. Embeds module versions, build
   settings, and compiler info in the binary. Useful for ad-hoc inspection.

2. **`goreleaser`** — Can generate SBOMs (SPDX or CycloneDX) as part of the
   release pipeline. Also supports Cosign keyless signing, checksum generation,
   and multi-platform builds. Recommended for v1.0 release automation.

3. **`ko`** — Can build and publish container images with SBOMs and SLSA
   provenance. Relevant only if container distribution is added.

4. **SLSA GitHub Generator** — Can generate SLSA L3 provenance for Go builds
   using the official `slsa-framework/slsa-github-generator` reusable workflow.

### Recommendation

Before v1.0:
- Integrate `goreleaser` for release automation with:
  - Multi-platform builds (Linux amd64/arm64, macOS amd64/arm64, Windows amd64).
  - SPDX SBOM generation.
  - SHA256 checksums.
  - Cosign keyless signing (OIDC-based, no key management needed).
- Publish SBOMs and checksums alongside release binaries.

---

## Version

This register was created for Nodex pre-1.0 (Workstream 6: Security and
Supply-Chain Hardening). Update it when gaps are closed or new gaps
identified. Review at minimum before each minor release.
