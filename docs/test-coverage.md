# Nodex Command Coverage Matrix

Generated from `internal/cli/operations.go` — the canonical operation registry.

**Cell values:** `yes` = covered, `no` = not covered, `partial` = partly covered,
`n/a` = not applicable for that test type.

**Coverage columns:**

- `Unit`: direct unit tests for command parsing, validation, safety gates, or provider calls.
- `E2E/Mock`: CLI-level tests against mocked providers or end-to-end harnesses.
- `Golden`: snapshot-style CLI output tests.
- `Fuzz`: fuzz coverage for untrusted parsing or validation boundaries.
- `Negative`: tests for invalid input, provider errors, or safety failures.

---

## Test Coverage by Operation

| Operation Path | Tier | Unit | E2E/Mock | Golden | Fuzz | Negative |
|---|---|---|---|---|---|---|
| **Version & System** | | | | | | |
| `version` | Observation | yes | yes | no | n/a | no |
| `version compare` | Observation | yes | yes | no | n/a | yes |
| `version parse` | Observation | yes | yes | no | n/a | yes |
| `init` | Reversible | no | no | no | n/a | no |
| `completion` | Observation | yes | no | no | n/a | no |
| `doctor` | Observation | yes | yes | no | n/a | yes |
| **Profile Management** | | | | | | |
| `profile add` | Reversible | yes | no | no | n/a | no |
| `profile list` | Observation | yes | no | no | no | no |
| `profile show` | Observation | yes | no | no | no | no |
| `profile set-credentials` | Reversible | yes | yes | no | no | no |
| `profile use` | Reversible | yes | no | no | n/a | no |
| `profile current` | Observation | yes | no | no | n/a | no |
| `profile test` | Observation | no | no | no | n/a | no |
| `profile remove` | Reversible | yes | no | no | n/a | no |
| `profile export` | Observation | no | yes | no | n/a | yes |
| `profile import` | Reversible | no | yes | no | n/a | yes |
| **Provider** | | | | | | |
| `provider list` | Observation | yes | yes | no | n/a | no |
| `provider capabilities` | Observation | yes | yes | no | n/a | yes |
| **Status** | | | | | | |
| `status` | Observation | no | yes | no | n/a | no |
| **Node** | | | | | | |
| `node list` | Observation | yes | yes | no | n/a | no |
| `node show` | Observation | yes | yes | no | n/a | yes |
| `node status` | Observation | no | no | no | n/a | no |
| `node services` | Observation | yes | yes | no | n/a | yes |
| `node network` | Observation | yes | no | no | n/a | no |
| `node dns` | Observation | yes | yes | no | n/a | yes |
| `node time` | Observation | yes | no | no | n/a | yes |
| `node disks` | Observation | yes | no | no | n/a | yes |
| `node certificates` | Observation | yes | no | no | n/a | yes |
| `node subscription` | Observation | yes | yes | no | n/a | yes |
| `node updates` | Observation | yes | no | no | n/a | yes |
| **VM — Inspection** | | | | | | |
| `vm list` | Observation | yes | yes | no | n/a | no |
| `vm show` | Observation | yes | yes | no | n/a | yes |
| `vm config` | Observation | no | no | no | n/a | no |
| `vm snapshots` | Observation | no | no | no | n/a | no |
| `vm snapshot-config` | Observation | no | yes | no | n/a | yes |
| **VM — Lifecycle (Tier 1)** | | | | | | |
| `vm start` | Reversible | yes | yes | no | n/a | yes |
| `vm stop` | Reversible | yes | yes | no | n/a | yes |
| `vm shutdown` | Reversible | yes | yes | no | n/a | yes |
| `vm suspend` | Reversible | yes | no | no | n/a | yes |
| `vm resume` | Reversible | yes | no | no | n/a | yes |
| `vm pause` | Reversible | yes | no | no | n/a | yes |
| `vm unpause` | Reversible | yes | no | no | n/a | yes |
| **VM — Lifecycle (Tier 2)** | | | | | | |
| `vm reset` | Disruptive | yes | yes | no | n/a | yes |
| `vm reboot` | Disruptive | yes | yes | no | n/a | yes |
| **VM — Config Mutations** | | | | | | |
| `vm update` | Reversible | yes | yes | no | no | yes |
| `vm cloud-init` | Reversible | yes | yes | no | n/a | yes |
| **VM — Destructive** | | | | | | |
| `vm delete` | Destructive | yes | yes | no | n/a | yes |
| **VM — Disruptive** | | | | | | |
| `vm template` | Disruptive | yes | yes | no | n/a | yes |
| `vm migrate` | Disruptive | yes | no | no | n/a | no |
| `vm clone` | Disruptive | yes | no | no | n/a | no |
| `vm disk resize` | Disruptive | yes | no | no | n/a | no |
| `vm disk move` | Disruptive | yes | no | no | n/a | no |
| **VM — Snapshot Mutations** | | | | | | |
| `vm snapshot create` | Reversible | yes | yes | no | n/a | yes |
| `vm snapshot delete` | Destructive | yes | no | no | n/a | yes |
| `vm snapshot rollback` | Disruptive | yes | no | no | n/a | yes |
| **Container — Inspection** | | | | | | |
| `container list` | Observation | yes | yes | no | n/a | no |
| `container show` | Observation | yes | yes | no | n/a | yes |
| `container config` | Observation | no | no | no | n/a | no |
| `container snapshots` | Observation | no | no | no | n/a | no |
| `container snapshot-config` | Observation | no | yes | no | n/a | yes |
| **Container — Lifecycle (Tier 1)** | | | | | | |
| `container start` | Reversible | yes | yes | no | n/a | yes |
| `container stop` | Reversible | yes | no | no | n/a | yes |
| `container shutdown` | Reversible | yes | no | no | n/a | yes |
| `container suspend` | Reversible | yes | no | no | n/a | yes |
| `container resume` | Reversible | yes | no | no | n/a | yes |
| **Container — Lifecycle (Tier 2)** | | | | | | |
| `container reboot` | Disruptive | yes | no | no | n/a | yes |
| **Container — Config** | | | | | | |
| `container update` | Reversible | yes | yes | no | no | yes |
| **Container — Destructive** | | | | | | |
| `container delete` | Destructive | yes | yes | no | n/a | yes |
| **Container — Disruptive** | | | | | | |
| `container template` | Disruptive | yes | yes | no | n/a | yes |
| `container migrate` | Disruptive | yes | no | no | n/a | no |
| `container clone` | Disruptive | yes | no | no | n/a | no |
| **Container — Snapshot Mutations** | | | | | | |
| `container snapshot create` | Reversible | yes | no | no | n/a | yes |
| `container snapshot delete` | Destructive | yes | no | no | n/a | yes |
| `container snapshot rollback` | Disruptive | yes | no | no | n/a | yes |
| **Storage** | | | | | | |
| `storage list` | Observation | yes | no | no | n/a | no |
| `storage show` | Observation | yes | yes | no | n/a | yes |
| `storage content` | Observation | no | no | no | n/a | no |
| `storage upload` | Disruptive | yes | no | no | n/a | no |
| `storage download` | Reversible | no | no | no | n/a | no |
| `storage delete` | Destructive | yes | no | no | n/a | no |
| **Cluster** | | | | | | |
| `cluster status` | Observation | no | no | no | n/a | no |
| `cluster log` | Observation | no | no | no | n/a | no |
| **Events / Logs** | | | | | | |
| `event list` | Observation | no | no | no | n/a | no |
| `log` | Observation | no | no | no | n/a | no |
| **Task** | | | | | | |
| `task list` | Observation | no | no | no | n/a | no |
| `task show` | Observation | no | no | no | n/a | no |
| **Backup** | | | | | | |
| `backup list` | Observation | no | no | no | n/a | no |
| `backup content` | Observation | no | yes | no | n/a | yes |
| `backup create` | Disruptive | yes | no | no | n/a | no |
| `backup restore` | Disruptive | yes | no | no | n/a | no |
| `backup job list` | Observation | no | no | no | n/a | no |
| `backup job show` | Observation | no | no | no | n/a | no |
| `backup job create` | Disruptive | yes | no | no | n/a | no |
| `backup job update` | Disruptive | yes | no | no | n/a | no |
| `backup job delete` | Destructive | yes | no | no | n/a | no |
| **Firewall** | | | | | | |
| `firewall list` | Observation | no | no | no | n/a | no |
| `firewall aliases` | Observation | yes | yes | no | n/a | yes |
| `firewall ipsets` | Observation | yes | no | no | n/a | yes |
| `firewall security-groups` | Observation | yes | no | no | n/a | yes |
| `firewall node-rules` | Observation | no | no | no | n/a | yes |
| `firewall vm-rules` | Observation | no | no | no | n/a | yes |
| `firewall rule create` | Disruptive | yes | no | no | n/a | no |
| `firewall rule update` | Disruptive | yes | no | no | n/a | no |
| `firewall rule delete` | Destructive | yes | no | no | n/a | no |
| `firewall alias create` | Disruptive | yes | no | no | n/a | no |
| `firewall alias delete` | Destructive | yes | no | no | n/a | no |
| `firewall ipset create` | Disruptive | yes | no | no | n/a | no |
| `firewall ipset entry add` | Disruptive | yes | no | no | n/a | no |
| `firewall ipset entry remove` | Destructive | yes | no | no | n/a | no |
| `firewall ipset delete` | Destructive | yes | no | no | n/a | no |
| `firewall group create` | Disruptive | yes | no | no | n/a | no |
| `firewall group delete` | Destructive | yes | no | no | n/a | no |
| `firewall options update` | Disruptive | yes | no | no | n/a | no |
| **HA** | | | | | | |
| `ha list` | Observation | no | no | no | n/a | no |
| `ha groups` | Observation | no | no | no | n/a | no |
| `ha status` | Observation | no | yes | no | n/a | yes |
| `ha current` | Observation | no | no | no | n/a | yes |
| **SDN** | | | | | | |
| `sdn zones` | Observation | yes | yes | no | n/a | yes |
| `sdn vnets` | Observation | yes | yes | no | n/a | yes |
| `sdn zone create` | Disruptive | yes | no | no | n/a | no |
| `sdn zone delete` | Destructive | yes | no | no | n/a | no |
| `sdn vnet create` | Disruptive | yes | no | no | n/a | no |
| `sdn vnet delete` | Destructive | yes | no | no | n/a | no |
| `sdn subnet create` | Disruptive | yes | no | no | n/a | no |
| `sdn subnet delete` | Destructive | yes | no | no | n/a | no |
| `sdn controller create` | Disruptive | yes | no | no | n/a | no |
| `sdn controller delete` | Destructive | yes | no | no | n/a | no |
| **Pools** | | | | | | |
| `pools list` | Observation | no | no | no | n/a | no |
| **Network** | | | | | | |
| `network show` | Observation | no | no | no | n/a | no |
| `network apply` | Disruptive | yes | no | no | n/a | no |
| `network revert` | Disruptive | yes | no | no | n/a | no |
| **Access** | | | | | | |
| `access users list` | Observation | no | no | no | n/a | no |
| `access groups list` | Observation | no | no | no | n/a | no |
| `access roles list` | Observation | no | no | no | n/a | no |
| `access acl list` | Observation | no | no | no | n/a | no |
| `access domains list` | Observation | no | no | no | n/a | no |
| `access tokens list` | Observation | no | no | no | n/a | no |
| `access user create` | SecurityAdmin | yes | no | no | n/a | yes |
| `access user delete` | SecurityAdmin | yes | no | no | n/a | yes |
| `access acl add` | SecurityAdmin | yes | no | no | n/a | yes |
| **Ceph** | | | | | | |
| `ceph status` | Observation | no | no | no | n/a | no |
| `ceph osd list` | Observation | no | no | no | n/a | no |
| `ceph mon list` | Observation | no | no | no | n/a | no |
| `ceph pool list` | Observation | no | no | no | n/a | no |
| `ceph osd create` | Disruptive | yes | no | no | n/a | no |
| `ceph osd out` | Disruptive | yes | no | no | n/a | no |
| `ceph osd in` | Reversible | yes | no | no | n/a | no |
| `ceph osd destroy` | Destructive | yes | no | no | n/a | no |
| `ceph pool create` | Disruptive | yes | no | no | n/a | no |
| `ceph pool destroy` | Destructive | yes | no | no | n/a | no |
| **Replication** | | | | | | |
| `replication list` | Observation | no | no | no | n/a | no |
| `replication show` | Observation | no | no | no | n/a | no |
| `replication create` | Disruptive | yes | no | no | n/a | no |
| `replication update` | Disruptive | yes | no | no | n/a | no |
| `replication delete` | Destructive | yes | no | no | n/a | no |
| `replication schedule` | Reversible | yes | no | no | n/a | no |
| **Dispatch Commands** | | | | | | |
| `vm snapshot` (route) | Observation | yes | no | no | n/a | no |
| `vm disk` (route) | Observation | yes | no | no | n/a | no |
| `container snapshot` (route) | Observation | yes | no | no | n/a | no |
| `firewall rule` (route) | Observation | yes | no | no | n/a | no |
| `firewall alias` (route) | Observation | yes | no | no | n/a | no |
| `firewall ipset` (route) | Observation | yes | no | no | n/a | no |
| `firewall group` (route) | Observation | yes | no | no | n/a | no |
| `firewall options` (route) | Observation | yes | no | no | n/a | no |
| `backup job` (route) | Observation | yes | no | no | n/a | no |
| `sdn zone` (route) | Observation | yes | no | no | n/a | no |
| `sdn vnet` (route) | Observation | yes | no | no | n/a | no |
| `sdn subnet` (route) | Observation | yes | no | no | n/a | no |
| `sdn controller` (route) | Observation | yes | no | no | n/a | no |
| `ceph osd` (route) | Observation | yes | no | no | n/a | no |
| `ceph mon` (route) | Observation | yes | no | no | n/a | no |
| `ceph pool` (route) | Observation | yes | no | no | n/a | no |
| `access user` (route) | Observation | yes | no | no | n/a | no |
| `access users` (route) | Observation | yes | no | no | n/a | no |
| `access groups` (route) | Observation | yes | no | no | n/a | no |
| `access roles` (route) | Observation | yes | no | no | n/a | no |
| `access acl` (route) | Observation | yes | no | no | n/a | no |
| `access domains` (route) | Observation | yes | no | no | n/a | no |
| `access tokens` (route) | Observation | yes | no | no | n/a | no |

---

## Cross-Cutting Test Coverage

| Category | Coverage | Details |
|---|---|---|
| **Safety tiers** | yes | All 5 tiers tested in `internal/safety/safety_test.go` |
| **Confirmation policy** | yes | Tier 1-4 checks, non-interactive mode, double confirm, type confirm |
| **Exit codes** | yes | All 21 exit codes covered in `internal/app/errors_test.go` |
| **Provider errors** | yes | Typed classification for all HTTP status codes |
| **Redaction** | yes | String, bytes, struct, JSON, YAML, nested, secret types |
| **UPID parsing** | yes | Full, colon, slash formats, empty, invalid |
| **Task polling** | yes | Success, failure, cancellation, timeout, transient recovery |
| **HTTP client** | yes | Retry, mutation no-retry, cancellation, jitter |
| **Config IO** | yes | Read, write, validate, update, lock |
| **Path validation** | yes | Traversal, symlink, non-regular files |
| **Credential backends** | yes | File, env, stdin, keyring |
| **Operation registry** | yes | Count, tiers, uniqueness, validation |
| **JSON/YAML output** | yes | Structured output for operations |
| **Multi-profile** | yes | --all flag, envelopes, nil handling |
| **Golden tests** | yes | CLI output snapshots in `internal/cli/golden_test.go` |
| **Fuzz (new in WS7)** | yes | Endpoint, profile names, UPID, credential refs, VMID, key=value |

---

## Cross-Platform Build Verification

| Target | Status |
|---|---|
| `GOOS=linux GOARCH=amd64` | Builds |
| `GOOS=windows GOARCH=amd64` | Builds |
| `GOOS=darwin GOARCH=amd64` | Builds |
| `GOOS=darwin GOARCH=arm64` | Builds |

---

## Summary

| Metric | Count |
|---|---|
| Total operations | ~144 |
| Inspection operations | ~96 |
| Mutation operations | ~48 |
| Unit tested operations | ~120 |
| E2E/Mock tested operations | ~55 |
| Operations with negative tests | ~70 |
| Operations with fuzz targets | 6 (targeted trust boundaries) |
| Cross-platform build targets | 4/4 |

---

## Notes

1. **Observation-only operations** that query external providers (e.g., `cluster status`, `event list`, `task list`, `ceph status`) require a live Proxmox connection and are tested through E2E mock providers where feasible. Many have unit-level command registration tests but not full integration tests.

2. **Dispatch commands** are routing commands that delegate to sub-operations. They have unit tests for registration and arg-count validation but typically don't have golden or E2E tests since their sub-operations handle the actual logic.

3. **Fuzz targets** focus on trust boundaries where untrusted input enters the system: endpoint URLs, UPID strings, credential refs, VMID parsing, and key=value argument parsing. These complement the existing unit tests for the same functions.

4. **Golden tests** capture CLI output for `version`, `provider list`, `profile list`, `node list`, `vm list`, and similar read-only commands. They are stored in `internal/cli/golden_test.go`.

5. **Live Proxmox tests** are not part of the default suite. Destructive live testing requires explicit opt-in, disposable resources, endpoint identity checks, and cleanup verification.
