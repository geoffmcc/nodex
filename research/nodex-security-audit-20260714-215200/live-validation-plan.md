# Live Validation Plan

No live Proxmox endpoint was authorized or contacted during this audit. The following checks require an explicitly authorized disposable Proxmox environment before execution:

1. Confirm task UPID polling against real long-running backup/migration/snapshot tasks, including timeout/cancellation recovery instructions.
2. Validate HA, lock, backup, migration, replication, and active-task precondition behavior for destructive/disruptive operations.
3. Validate least-privilege permission matrices from `docs/cli-reference.md` against Proxmox VE roles/tokens.
4. Validate custom CA trust with a disposable HTTPS endpoint whose hostname matches the configured endpoint and with a hostile hostname mismatch.
5. Validate Windows/macOS native file permission behavior for config/credential writes outside WSL.

All live mutation tests must use disposable VMs/CTs/storage and must never target an unspecified production cluster.
