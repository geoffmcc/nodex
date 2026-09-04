# Fleet Maintenance

`maintenance plan` creates a signed, expiring JSON/YAML plan. `maintenance apply`
loads and validates that plan, requires `--yes --force --confirm-target <plan-id>`,
rechecks inventory addresses, and executes only the embedded Ansible operations.
Security updates are limited to the package names recorded by the preflight;
`approved-full-upgrade` is a separate, explicitly selected operation.

Each apply writes a mode-0600 JSON receipt atomically after every host. Receipts
contain plan and host outcome metadata only; Ansible output and credentials are
never persisted. `maintenance report --receipt <file>` verifies the receipt
digest before rendering deterministic table, JSON, or YAML output.

An interrupted or ambiguous operation is recorded as `unknown` and is not
automatically replayed. This is intentional: Ansible may have changed a host
before the client lost its result, so automatic recovery could apply updates
twice. Review the receipt and current host state, then create a new plan before
retrying. A missing, malformed, or tampered receipt is rejected rather than
overwritten.
