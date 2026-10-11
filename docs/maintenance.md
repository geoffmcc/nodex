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

## Unattended security-update policy

This is separate from the one-shot `maintenance plan --policy security-only`
workflow. It configures unattended security updates for explicitly opted-in
Debian/Ubuntu inventory hosts:

```sh
nodex --output json maintenance policy plan --host web-guest > policy.json
plan_id=$(jq -r '.plan_id' policy.json)
nodex --yes --force --confirm-target "$plan_id" \
  maintenance policy apply --plan policy.json
nodex --yes --force --confirm-target "$plan_id" \
  maintenance policy restore --plan policy.json
```

Set `inventory.hosts.<name>.unattended_security_updates: true` before planning.
PVE, PBS, and DNS roles are always excluded. The read-only plan inspects the
effective APT security origins, prior timer state, and any existing Nodex-owned
drop-in, then includes the exact file diff, expiry, and digest. It does not
rewrite administrator-owned APT files. If the reserved Nodex drop-in exists
but does not match Nodex's generated format, planning blocks rather than
overwriting it.

Apply installs `unattended-upgrades` if needed, backs up an existing Nodex
drop-in under `/var/lib/nodex/security-policy-backups/<plan-id>`, writes the
reviewed security-only configuration, enables `apt-daily-upgrade.timer`, and
verifies the effective origins and timer state. It does not run an upgrade in
the apply transaction and never enables automatic reboot. Ansible results are
recorded in a secret-free local receipt. Keep the plan file: restore uses its
digest-bound pre-change contents, checks the remote backup and current state,
and restores the prior Nodex-managed file and timer state without removing the
package. If Nodex installed the package, package-provided defaults remain, but
the timer is returned to its previous state. A changed
drop-in or missing/mismatched backup blocks restore rather than overwriting
operator changes.
If apply used a custom `--receipt-dir`, pass that same directory to restore.
