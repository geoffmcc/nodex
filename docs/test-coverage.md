# Nodex Command Coverage Matrix

Generated from `internal/cli/operations.go` — the canonical operation registry.

**Legend:**
- ✅ = Covered
- ❌ = Not covered / not applicable
- 🔶 = Partially covered
- N/A = Not applicable for this category

---

## Test Coverage by Operation

| Operation Path | Tier | Unit | E2E/Mock | Golden | Fuzz | Negative |
|---|---|---|---|---|---|---|
| **Version & System** | | | | | | |
| `version` | Observation | ✅ | ✅ | ❌ | N/A | ❌ |
| `version compare` | Observation | ✅ | ✅ | ❌ | N/A | ✅ |
| `version parse` | Observation | ✅ | ✅ | ❌ | N/A | ✅ |
| `init` | Reversible | ❌ | ❌ | ❌ | N/A | ❌ |
| `completion` | Observation | ✅ | ❌ | ❌ | N/A | ❌ |
| `doctor` | Observation | ✅ | ✅ | ❌ | N/A | ✅ |
| **Profile Management** | | | | | | |
| `profile add` | Reversible | ✅ | ❌ | ❌ | N/A | ❌ |
| `profile list` | Observation | ✅ | ❌ | ❌ | ❌ | ❌ |
| `profile show` | Observation | ✅ | ❌ | ❌ | ❌ | ❌ |
| `profile set-credentials` | Reversible | ✅ | ✅ | ❌ | ❌ | ❌ |
| `profile use` | Reversible | ✅ | ❌ | ❌ | N/A | ❌ |
| `profile current` | Observation | ✅ | ❌ | ❌ | N/A | ❌ |
| `profile test` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| `profile remove` | Reversible | ✅ | ❌ | ❌ | N/A | ❌ |
| `profile export` | Observation | ❌ | ✅ | ❌ | N/A | ✅ |
| `profile import` | Reversible | ❌ | ✅ | ❌ | N/A | ✅ |
| **Provider** | | | | | | |
| `provider list` | Observation | ✅ | ✅ | ❌ | N/A | ❌ |
| `provider capabilities` | Observation | ✅ | ✅ | ❌ | N/A | ✅ |
| **Status** | | | | | | |
| `status` | Observation | ❌ | ✅ | ❌ | N/A | ❌ |
| **Node** | | | | | | |
| `node list` | Observation | ✅ | ✅ | ❌ | N/A | ❌ |
| `node show` | Observation | ✅ | ✅ | ❌ | N/A | ✅ |
| `node status` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| `node services` | Observation | ✅ | ✅ | ❌ | N/A | ✅ |
| `node network` | Observation | ✅ | ❌ | ❌ | N/A | ❌ |
| `node dns` | Observation | ✅ | ✅ | ❌ | N/A | ✅ |
| `node time` | Observation | ✅ | ❌ | ❌ | N/A | ✅ |
| `node disks` | Observation | ✅ | ❌ | ❌ | N/A | ✅ |
| `node certificates` | Observation | ✅ | ❌ | ❌ | N/A | ✅ |
| `node subscription` | Observation | ✅ | ✅ | ❌ | N/A | ✅ |
| `node updates` | Observation | ✅ | ❌ | ❌ | N/A | ✅ |
| **VM — Inspection** | | | | | | |
| `vm list` | Observation | ✅ | ✅ | ❌ | N/A | ❌ |
| `vm show` | Observation | ✅ | ✅ | ❌ | N/A | ✅ |
| `vm config` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| `vm snapshots` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| `vm snapshot-config` | Observation | ❌ | ✅ | ❌ | N/A | ✅ |
| **VM — Lifecycle (Tier 1)** | | | | | | |
| `vm start` | Reversible | ✅ | ✅ | ❌ | N/A | ✅ |
| `vm stop` | Reversible | ✅ | ✅ | ❌ | N/A | ✅ |
| `vm shutdown` | Reversible | ✅ | ✅ | ❌ | N/A | ✅ |
| `vm suspend` | Reversible | ✅ | ❌ | ❌ | N/A | ✅ |
| `vm resume` | Reversible | ✅ | ❌ | ❌ | N/A | ✅ |
| `vm pause` | Reversible | ✅ | ❌ | ❌ | N/A | ✅ |
| `vm unpause` | Reversible | ✅ | ❌ | ❌ | N/A | ✅ |
| **VM — Lifecycle (Tier 2)** | | | | | | |
| `vm reset` | Disruptive | ✅ | ✅ | ❌ | N/A | ✅ |
| `vm reboot` | Disruptive | ✅ | ✅ | ❌ | N/A | ✅ |
| **VM — Config Mutations** | | | | | | |
| `vm update` | Reversible | ✅ | ✅ | ❌ | ❌ | ✅ |
| `vm cloud-init` | Reversible | ✅ | ✅ | ❌ | N/A | ✅ |
| **VM — Destructive** | | | | | | |
| `vm delete` | Destructive | ✅ | ✅ | ❌ | N/A | ✅ |
| **VM — Disruptive** | | | | | | |
| `vm template` | Disruptive | ✅ | ✅ | ❌ | N/A | ✅ |
| `vm migrate` | Disruptive | ✅ | ❌ | ❌ | N/A | ❌ |
| `vm clone` | Disruptive | ✅ | ❌ | ❌ | N/A | ❌ |
| `vm disk resize` | Disruptive | ✅ | ❌ | ❌ | N/A | ❌ |
| `vm disk move` | Disruptive | ✅ | ❌ | ❌ | N/A | ❌ |
| **VM — Snapshot Mutations** | | | | | | |
| `vm snapshot create` | Reversible | ✅ | ✅ | ❌ | N/A | ✅ |
| `vm snapshot delete` | Destructive | ✅ | ❌ | ❌ | N/A | ✅ |
| `vm snapshot rollback` | Disruptive | ✅ | ❌ | ❌ | N/A | ✅ |
| **Container — Inspection** | | | | | | |
| `container list` | Observation | ✅ | ✅ | ❌ | N/A | ❌ |
| `container show` | Observation | ✅ | ✅ | ❌ | N/A | ✅ |
| `container config` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| `container snapshots` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| `container snapshot-config` | Observation | ❌ | ✅ | ❌ | N/A | ✅ |
| **Container — Lifecycle (Tier 1)** | | | | | | |
| `container start` | Reversible | ✅ | ✅ | ❌ | N/A | ✅ |
| `container stop` | Reversible | ✅ | ❌ | ❌ | N/A | ✅ |
| `container shutdown` | Reversible | ✅ | ❌ | ❌ | N/A | ✅ |
| `container suspend` | Reversible | ✅ | ❌ | ❌ | N/A | ✅ |
| `container resume` | Reversible | ✅ | ❌ | ❌ | N/A | ✅ |
| **Container — Lifecycle (Tier 2)** | | | | | | |
| `container reboot` | Disruptive | ✅ | ❌ | ❌ | N/A | ✅ |
| **Container — Config** | | | | | | |
| `container update` | Reversible | ✅ | ✅ | ❌ | ❌ | ✅ |
| **Container — Destructive** | | | | | | |
| `container delete` | Destructive | ✅ | ✅ | ❌ | N/A | ✅ |
| **Container — Disruptive** | | | | | | |
| `container template` | Disruptive | ✅ | ✅ | ❌ | N/A | ✅ |
| `container migrate` | Disruptive | ✅ | ❌ | ❌ | N/A | ❌ |
| `container clone` | Disruptive | ✅ | ❌ | ❌ | N/A | ❌ |
| **Container — Snapshot Mutations** | | | | | | |
| `container snapshot create` | Reversible | ✅ | ❌ | ❌ | N/A | ✅ |
| `container snapshot delete` | Destructive | ✅ | ❌ | ❌ | N/A | ✅ |
| `container snapshot rollback` | Disruptive | ✅ | ❌ | ❌ | N/A | ✅ |
| **Storage** | | | | | | |
| `storage list` | Observation | ✅ | ❌ | ❌ | N/A | ❌ |
| `storage show` | Observation | ✅ | ✅ | ❌ | N/A | ✅ |
| `storage content` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| `storage upload` | Disruptive | ✅ | ❌ | ❌ | N/A | ❌ |
| `storage download` | Reversible | ❌ | ❌ | ❌ | N/A | ❌ |
| `storage delete` | Destructive | ✅ | ❌ | ❌ | N/A | ❌ |
| **Cluster** | | | | | | |
| `cluster status` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| `cluster log` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| **Events / Logs** | | | | | | |
| `event list` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| `log` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| **Task** | | | | | | |
| `task list` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| `task show` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| **Backup** | | | | | | |
| `backup list` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| `backup content` | Observation | ❌ | ✅ | ❌ | N/A | ✅ |
| `backup create` | Disruptive | ✅ | ❌ | ❌ | N/A | ❌ |
| `backup restore` | Disruptive | ✅ | ❌ | ❌ | N/A | ❌ |
| `backup job list` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| `backup job show` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| `backup job create` | Disruptive | ✅ | ❌ | ❌ | N/A | ❌ |
| `backup job update` | Disruptive | ✅ | ❌ | ❌ | N/A | ❌ |
| `backup job delete` | Destructive | ✅ | ❌ | ❌ | N/A | ❌ |
| **Firewall** | | | | | | |
| `firewall list` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| `firewall aliases` | Observation | ✅ | ✅ | ❌ | N/A | ✅ |
| `firewall ipsets` | Observation | ✅ | ❌ | ❌ | N/A | ✅ |
| `firewall security-groups` | Observation | ✅ | ❌ | ❌ | N/A | ✅ |
| `firewall node-rules` | Observation | ❌ | ❌ | ❌ | N/A | ✅ |
| `firewall vm-rules` | Observation | ❌ | ❌ | ❌ | N/A | ✅ |
| `firewall rule create` | Disruptive | ✅ | ❌ | ❌ | N/A | ❌ |
| `firewall rule update` | Disruptive | ✅ | ❌ | ❌ | N/A | ❌ |
| `firewall rule delete` | Destructive | ✅ | ❌ | ❌ | N/A | ❌ |
| `firewall alias create` | Disruptive | ✅ | ❌ | ❌ | N/A | ❌ |
| `firewall alias delete` | Destructive | ✅ | ❌ | ❌ | N/A | ❌ |
| `firewall ipset create` | Disruptive | ✅ | ❌ | ❌ | N/A | ❌ |
| `firewall ipset entry add` | Disruptive | ✅ | ❌ | ❌ | N/A | ❌ |
| `firewall ipset entry remove` | Destructive | ✅ | ❌ | ❌ | N/A | ❌ |
| `firewall ipset delete` | Destructive | ✅ | ❌ | ❌ | N/A | ❌ |
| `firewall group create` | Disruptive | ✅ | ❌ | ❌ | N/A | ❌ |
| `firewall group delete` | Destructive | ✅ | ❌ | ❌ | N/A | ❌ |
| `firewall options update` | Disruptive | ✅ | ❌ | ❌ | N/A | ❌ |
| **HA** | | | | | | |
| `ha list` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| `ha groups` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| `ha status` | Observation | ❌ | ✅ | ❌ | N/A | ✅ |
| `ha current` | Observation | ❌ | ❌ | ❌ | N/A | ✅ |
| **SDN** | | | | | | |
| `sdn zones` | Observation | ✅ | ✅ | ❌ | N/A | ✅ |
| `sdn vnets` | Observation | ✅ | ✅ | ❌ | N/A | ✅ |
| `sdn zone create` | Disruptive | ✅ | ❌ | ❌ | N/A | ❌ |
| `sdn zone delete` | Destructive | ✅ | ❌ | ❌ | N/A | ❌ |
| `sdn vnet create` | Disruptive | ✅ | ❌ | ❌ | N/A | ❌ |
| `sdn vnet delete` | Destructive | ✅ | ❌ | ❌ | N/A | ❌ |
| `sdn subnet create` | Disruptive | ✅ | ❌ | ❌ | N/A | ❌ |
| `sdn subnet delete` | Destructive | ✅ | ❌ | ❌ | N/A | ❌ |
| `sdn controller create` | Disruptive | ✅ | ❌ | ❌ | N/A | ❌ |
| `sdn controller delete` | Destructive | ✅ | ❌ | ❌ | N/A | ❌ |
| **Pools** | | | | | | |
| `pools list` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| **Network** | | | | | | |
| `network show` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| `network apply` | Disruptive | ✅ | ❌ | ❌ | N/A | ❌ |
| `network revert` | Disruptive | ✅ | ❌ | ❌ | N/A | ❌ |
| **Access** | | | | | | |
| `access users list` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| `access groups list` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| `access roles list` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| `access acl list` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| `access domains list` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| `access tokens list` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| `access user create` | SecurityAdmin | ✅ | ❌ | ❌ | N/A | ✅ |
| `access user delete` | SecurityAdmin | ✅ | ❌ | ❌ | N/A | ✅ |
| `access acl add` | SecurityAdmin | ✅ | ❌ | ❌ | N/A | ✅ |
| **Ceph** | | | | | | |
| `ceph status` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| `ceph osd list` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| `ceph mon list` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| `ceph pool list` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| `ceph osd create` | Disruptive | ✅ | ❌ | ❌ | N/A | ❌ |
| `ceph osd out` | Disruptive | ✅ | ❌ | ❌ | N/A | ❌ |
| `ceph osd in` | Reversible | ✅ | ❌ | ❌ | N/A | ❌ |
| `ceph osd destroy` | Destructive | ✅ | ❌ | ❌ | N/A | ❌ |
| `ceph pool create` | Disruptive | ✅ | ❌ | ❌ | N/A | ❌ |
| `ceph pool destroy` | Destructive | ✅ | ❌ | ❌ | N/A | ❌ |
| **Replication** | | | | | | |
| `replication list` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| `replication show` | Observation | ❌ | ❌ | ❌ | N/A | ❌ |
| `replication create` | Disruptive | ✅ | ❌ | ❌ | N/A | ❌ |
| `replication update` | Disruptive | ✅ | ❌ | ❌ | N/A | ❌ |
| `replication delete` | Destructive | ✅ | ❌ | ❌ | N/A | ❌ |
| `replication schedule` | Reversible | ✅ | ❌ | ❌ | N/A | ❌ |
| **Dispatch Commands** | | | | | | |
| `vm snapshot` (route) | Observation | ✅ | ❌ | ❌ | N/A | ❌ |
| `vm disk` (route) | Observation | ✅ | ❌ | ❌ | N/A | ❌ |
| `container snapshot` (route) | Observation | ✅ | ❌ | ❌ | N/A | ❌ |
| `firewall rule` (route) | Observation | ✅ | ❌ | ❌ | N/A | ❌ |
| `firewall alias` (route) | Observation | ✅ | ❌ | ❌ | N/A | ❌ |
| `firewall ipset` (route) | Observation | ✅ | ❌ | ❌ | N/A | ❌ |
| `firewall group` (route) | Observation | ✅ | ❌ | ❌ | N/A | ❌ |
| `firewall options` (route) | Observation | ✅ | ❌ | ❌ | N/A | ❌ |
| `backup job` (route) | Observation | ✅ | ❌ | ❌ | N/A | ❌ |
| `sdn zone` (route) | Observation | ✅ | ❌ | ❌ | N/A | ❌ |
| `sdn vnet` (route) | Observation | ✅ | ❌ | ❌ | N/A | ❌ |
| `sdn subnet` (route) | Observation | ✅ | ❌ | ❌ | N/A | ❌ |
| `sdn controller` (route) | Observation | ✅ | ❌ | ❌ | N/A | ❌ |
| `ceph osd` (route) | Observation | ✅ | ❌ | ❌ | N/A | ❌ |
| `ceph mon` (route) | Observation | ✅ | ❌ | ❌ | N/A | ❌ |
| `ceph pool` (route) | Observation | ✅ | ❌ | ❌ | N/A | ❌ |
| `access user` (route) | Observation | ✅ | ❌ | ❌ | N/A | ❌ |
| `access users` (route) | Observation | ✅ | ❌ | ❌ | N/A | ❌ |
| `access groups` (route) | Observation | ✅ | ❌ | ❌ | N/A | ❌ |
| `access roles` (route) | Observation | ✅ | ❌ | ❌ | N/A | ❌ |
| `access acl` (route) | Observation | ✅ | ❌ | ❌ | N/A | ❌ |
| `access domains` (route) | Observation | ✅ | ❌ | ❌ | N/A | ❌ |
| `access tokens` (route) | Observation | ✅ | ❌ | ❌ | N/A | ❌ |

---

## Cross-Cutting Test Coverage

| Category | Coverage | Details |
|---|---|---|
| **Safety tiers** | ✅ | All 5 tiers tested in `internal/safety/safety_test.go` |
| **Confirmation policy** | ✅ | Tier 1-4 checks, non-interactive mode, double confirm, type confirm |
| **Exit codes** | ✅ | All 21 exit codes covered in `internal/app/errors_test.go` |
| **Provider errors** | ✅ | Typed classification for all HTTP status codes |
| **Redaction** | ✅ | String, bytes, struct, JSON, YAML, nested, secret types |
| **UPID parsing** | ✅ | Full, colon, slash formats, empty, invalid |
| **Task polling** | ✅ | Success, failure, cancellation, timeout, transient recovery |
| **HTTP client** | ✅ | Retry, mutation no-retry, cancellation, jitter |
| **Config IO** | ✅ | Read, write, validate, update, lock |
| **Path validation** | ✅ | Traversal, symlink, non-regular files |
| **Credential backends** | ✅ | File, env, stdin, keyring |
| **Operation registry** | ✅ | Count, tiers, uniqueness, validation |
| **JSON/YAML output** | ✅ | Structured output for operations |
| **Multi-profile** | ✅ | --all flag, envelopes, nil handling |
| **Golden tests** | ✅ | CLI output snapshots in `internal/cli/golden_test.go` |
| **Fuzz (new in WS7)** | ✅ | Endpoint, profile names, UPID, credential refs, VMID, key=value |

---

## Cross-Platform Build Verification

| Target | Status |
|---|---|
| `GOOS=linux GOARCH=amd64` | ✅ Builds |
| `GOOS=windows GOARCH=amd64` | ✅ Builds |
| `GOOS=darwin GOARCH=amd64` | ✅ Builds |
| `GOOS=darwin GOARCH=arm64` | ✅ Builds |

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
| Cross-platform build targets | 4/4 ✅ |

---

## Notes

1. **Observation-only operations** that query external providers (e.g., `cluster status`, `event list`, `task list`, `ceph status`) require a live Proxmox connection and are tested through E2E mock providers where feasible. Many have unit-level command registration tests but not full integration tests.

2. **Dispatch commands** are routing commands that delegate to sub-operations. They have unit tests for registration and arg-count validation but typically don't have golden or E2E tests since their sub-operations handle the actual logic.

3. **Fuzz targets** focus on trust boundaries where untrusted input enters the system: endpoint URLs, UPID strings, credential refs, VMID parsing, and key=value argument parsing. These complement the existing unit tests for the same functions.

4. **Golden tests** capture CLI output for `version`, `provider list`, `profile list`, `node list`, `vm list`, and similar read-only commands. They are stored in `internal/cli/golden_test.go`.
