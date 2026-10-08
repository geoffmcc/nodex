# Nodex Residual Risk Register

This register records known security and operational limitations that remain
after the controls described in the [threat model](threat-model.md). It is
deliberately limited to risks that can be verified from the current design;
version-specific and deployment-specific facts must be checked in the live
repository, provider, and release workflow.

## Accepted residual risks

### Local credential exposure

**Risk:** A compromised operator account or sufficiently privileged local
process may read file-backed credentials, environment variables, process
memory, keyring entries or SSH-agent access. File credentials use restrictive
permissions but are not application-encrypted.

**Mitigation:** Prefer the OS keyring or an appropriately isolated secret
delivery method; limit API-token privileges and lifetime; protect the operator
account; never put secrets in command arguments or committed files.

### Provider identity and truthful state

**Risk:** General PVE/PBS profiles rely on standard system/custom CA trust and
do not pin a leaf certificate or use mutual TLS. A compromised trusted CA or
provider can impersonate the endpoint or return false state. Agent receipts
cannot provide stronger truth than the provider evidence they record.

**Mitigation:** Protect trust stores, use a controlled private CA where
appropriate, validate endpoint configuration, and review unknown or unsupported
results instead of assuming healthy state. Disposable certification binds a
specific authorized environment more tightly than normal profiles.

### Confirmation is not authorization

**Risk:** `--yes`, `--force`, `--expert` and `--confirm-target` acknowledge an
operation to Nodex but do not prove that a named human authorized it. A local
automation caller can invoke arbitrary commands outside `--agent`; that mode
is not a sandbox.

**Mitigation:** Use least-privilege provider credentials, constrain access to
the operator machine and automation runner, review exact targets, and do not
expose Nodex as an unauthenticated remote execution service.

### Ambiguous remote outcomes

**Risk:** A client can lose its connection after a provider accepted a mutation.
Task history may expire and identifiers may not uniquely identify a resource
forever. Nodex cannot promise exactly-once execution where a provider has no
idempotency facility.

**Mitigation:** Use `--wait` when appropriate, retain relevant task IDs/receipts,
reconcile before retrying and accept `unknown` when evidence is insufficient.
Agent receipt replay deduplicates locally but is not provider-side exactly-once
execution.

### Optional Ansible trust boundary

**Risk:** Fleet maintenance executes embedded approved operations on explicitly
enrolled hosts. A compromised local Ansible installation, SSH trust store,
operator key or managed host can undermine the workflow. Package updates can
still cause service changes even when the plan and execution evidence are
valid.

**Mitigation:** Use a trusted Ansible installation, strict SSH host-key
verification, narrow SSH/sudo privileges, explicit inventory and reviewed
plans. Nodex does not accept arbitrary playbooks or shell commands. Its current
maintenance plans set reboot policy to `never`.

### Release and dependency trust

**Risk:** Users ultimately trust source review, the Go module supply chain,
GitHub-hosted build infrastructure, release credentials and the signing
identity. Published checksums establish artifact integrity only when the
checksum file's signature/provenance and expected identity are also verified.

**Mitigation:** Verify modules and tests when building from source; verify
release checksums, Sigstore signature identity, and provenance when using
published artifacts. Independent reproducible rebuild comparison is not a
guarantee of the release process.

## Scope limitations (not automatic failures)

- PVE and PBS are the only implemented API providers.
- Provider permissions and API behavior vary by server version and token
  privilege. Static operation metadata does not prove runtime support or
  authorization.
- Some operations cannot verify their requested postcondition; the operation
  registry describes this. A missing verification result means no check was
  made, not that verification passed.
- Generic HTTP monitoring can be configured for plaintext HTTP. Monitoring
  addresses and target policy must be selected by the operator.
- `service` monitoring targets are schema-valid but currently return
  `unsupported`; there is no implemented systemd service monitoring adapter.
- Cluster join is preflight-only. Corosync membership, subscription-key
  management, TFA enrollment and arbitrary remote execution are not provided.
- Nodex is pre-1.0. See [compatibility policy](compatibility.md) for the
  stability levels of CLI, configuration and structured output.

## Maintenance

Update this register when implementation changes close or introduce a material
risk. For each update, verify claims against current source, tests, CI/release
configuration and provider behavior. Do not copy transient runtime state or
credentials into this document.
