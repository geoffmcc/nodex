# Certification

Certification is an explicit, disposable-environment transaction for validating
Nodex against a real configured Proxmox test environment. It never selects a
profile implicitly and refuses every profile except `nodex-test-admin`.

The transaction is opt-in and requires `--yes`, an exact `--confirm-target`, an
explicit node, VMID, storage, and a VM name beginning with `nodex-cert-`. Nodex
records the cleanup target atomically before creation, waits for provider task
completion, verifies creation, deletes only the matching ledger resource, and
verifies absence. Failed or interrupted runs leave a pending ledger entry for
recovery.

```bash
nodex --profile nodex-test-admin --yes \
  --confirm-target nodex-cert-smoke certification run \
  --node pve-test --vmid 9000 --name nodex-cert-smoke --storage local
nodex --profile nodex-test-admin --yes \
  --confirm-target <ledger-entry-id> certification cleanup
nodex certification report
```

Use `--ledger <path>` when a separate ledger location is required. Reports
contain only sanitized target metadata and state; credentials and provider
responses are not stored. The default ledger is under the Nodex configuration
directory. Certification is not a fake-server test and must not be pointed at
production-looking targets.
