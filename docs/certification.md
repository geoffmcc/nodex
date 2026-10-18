# Certification

Certification is an explicit, disposable-environment transaction for validating
Nodex against a real configured Proxmox test environment. It never selects a
profile implicitly; the explicit `--profile` must match the profile bound to
the named certification environment. The environment must be declared in the configuration with its exact endpoint,
provider, allowed node/storage/VMID range, expiry, expected leaf certificate
fingerprint, and trusted-CA identity.

The `readonly` suite performs no VM mutation. The `disposable-mutations` suite
is opt-in and requires `--yes`, an exact `--confirm-target`, an explicit node,
VMID, storage, and a VM name beginning with `nodex-cert-`. Nodex verifies the
provider TLS identity before mutation and pins the authorized leaf fingerprint
on the provider transport, records the cleanup target atomically, waits for task
completion, verifies creation, deletes only the matching ledger resource, and
verifies absence. A ledger transaction lock and expiring lease prevent stale
workers from overlapping cleanup recovery. Failed or interrupted runs leave a
pending ledger entry for recovery. The readonly suite does not read or depend on
the cleanup ledger. Mutation runs and cleanup require an integrity-verified
ledger; a legacy or corrupt ledger fails closed before provider mutation.

To upgrade a completed schema 1 ledger, run
`nodex --yes certification migrate-ledger`. This local-only command does not
connect to Proxmox. It accepts migration only when every legacy entry is marked
cleanup-complete and has no lease, preserves the exact original at
`certification-ledger.json.legacy-v1.bak`, and refuses to overwrite a different
existing backup. Pending or ambiguous entries must be reconciled from trusted
state; migration will not convert them into mutation authority.

```bash
nodex --profile nodex-test-profile --yes \
  --confirm-target nodex-cert-smoke certification run \
  --environment test-lab --suite disposable-mutations \
  --node pve-test --vmid 9000 --name nodex-cert-smoke --storage local
nodex --profile nodex-test-profile --yes \
  --confirm-target <ledger-entry-id> certification cleanup
nodex certification report
```

Mutation runs always use the default ledger under the Nodex configuration
directory. This keeps the environment resource cap in one shared accounting
file; `--ledger` is rejected for mutation runs. Use `--ledger <path>` for
read-only reports or to select a ledger during cleanup recovery. Reports
contain only sanitized target metadata and state; credentials and provider
responses are not stored. To relocate the mutation ledger, configure the
platform's normal Nodex configuration directory (for example,
`XDG_CONFIG_HOME` on Linux) consistently for all invocations. Certification is
not a fake-server test and must not be pointed at production-looking targets.
