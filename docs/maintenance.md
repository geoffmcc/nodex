# Fleet Maintenance

`maintenance plan` creates an expiring JSON/YAML plan with an integrity digest. `maintenance apply`
loads and validates that plan, requires `--yes --force --confirm-target <plan-id>`,
rechecks inventory addresses, and executes only the embedded Ansible operations.
Security updates are limited to the package names recorded by the preflight;
`approved-full-upgrade` is a separate, explicitly selected operation.

Each apply writes a mode-0600 JSON receipt atomically after every host. Receipts
contain plan and host outcome metadata only; Ansible output and credentials are
never persisted. The embedded callback uses the versioned
`nodex.ansible.task-results.v1` evidence contract; a nonzero Ansible exit status,
missing evidence, or unusable required task result is never treated as success.
`maintenance report --receipt <file>` verifies the receipt digest before
rendering deterministic table, JSON, or YAML output.

An interrupted or ambiguous operation is recorded as `unknown`. `maintenance
resume` revalidates the exact plan and receipt but refuses to replay any
non-successful host; review the receipt and current host state, then create a
new plan before retrying. `maintenance reconcile` performs read-only
postcondition verification, while `maintenance abandon` records an explicit
operator decision. A missing, malformed, or tampered receipt is rejected
rather than overwritten.
