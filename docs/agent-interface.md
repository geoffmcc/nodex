# NodeX agent interface

NodeX's agent interface is an opt-in entry point to the existing local CLI. It does not add a daemon or transport. It discovers the same registered operations and calls the same command handlers, providers, confirmation checks, TLS policies, SSH trust checks, and maintenance/certification workflows used by the human CLI.

## Discover before acting

Discovery is offline. It describes implementation and argument requirements; it does not test credentials, provider readiness, caller authorization, or current permissions.

```sh
nodex agent contract --output json
nodex operation list --output json
nodex operation describe "vm stop" --output json
nodex operation describe "pbs verify run" --output json
```

`operation list` is intentionally compact. `operation describe` includes the existing CLI grammar, argument/flag metadata, constraints, target requirements, safety tier, risks, confirmations, provider interface, task behavior, verification/recovery notes, output schemas, and the agent-mode support decision. Canonical IDs use the operation path with periods (for example `vm.stop`); aliases resolve to that canonical operation.

Schema files are in [`docs/agent/schemas`](agent/schemas/):

- `agent-contract-v1.schema.json`
- `operation-contract-v1.schema.json`
- `agent-result-v1.schema.json`
- `agent-receipt-v1.schema.json`

The JSON result schema version is `1`. The contract reports contract version `1.0.0` and input schema version `1`.

## Execute through the existing CLI

Put `--agent` before the operation. Remote provider operations require an explicit `--profile`; NodeX does not resolve them through `current_profile`. The existing operation syntax and positional targets remain in force.

```sh
nodex --agent --profile lab --request-id req-start-vm-100 --yes \
  vm start pve1/100

nodex --agent --profile lab --request-id req-stop-vm-100 \
  --yes vm stop pve1/100

nodex --agent --profile backup --request-id req-pbs-verify-daily \
  --yes pbs verify run verify-daily
```

NodeX sets JSON output and non-interactive operation for the wrapped CLI call. An explicit `--output` must be `json`; `--non-interactive=false`, `--all`, and `--password-stdin` are rejected. It never inserts `--yes`, `--force`, `--expert`, or `--confirm-target`, and it does not weaken TLS certificate or SSH host-key verification. A missing required confirmation fails closed rather than prompting.

Confirmation flags acknowledge intent to NodeX's existing safety code. They are not a signature or proof that a human approved an agent's action. The contract's provider capability information does not imply the configured credential is authorized to use it.

Every normal `--agent` request has one JSON result on stdout, including validation, profile, support, and execution failures. Diagnostic prose remains on stderr. Existing exit codes remain meaningful, but agents should make decisions from the result fields and error code, not from the exit code alone. The normal CLI output and legacy `OperationResult` shape are unchanged outside `--agent`.

### Result meanings

The result distinguishes:

- `submission`: `not_attempted`, `accepted`, `rejected`, or `unknown`.
- `execution`: `not_started`, `running`, `succeeded`, `failed`, or `unknown`.
- `verification`: `not_requested`, `passed`, `failed`, `unknown`, or `unsupported`.
- `changed`: `true`, `false`, or `null` when the provider cannot establish a change.
- `retry`: `safe`, `reconcile_first`, or `do_not_retry_automatically`.

An accepted request is not completed work. A completed task is not automatically a verified resource postcondition. For lifecycle tasks, a refresh can separately inspect the current guest state. `verification: passed` there means the target currently reports the desired state; it does not prove that this request caused the state, and providers that expose no stable guest UUID cannot rule out ID reuse.

Definitive provider rejections (for example, a validation/authorization 4xx response) are reported as `rejected`. Transport loss, request timeouts, and provider server errors are retained as `unknown` unless an attributable task identifier or other authoritative evidence is available; a 5xx is not assumed to mean that no mutation occurred.

For other operations, postcondition verification is `unsupported` unless the existing handler already supplied trustworthy verification (for example, container OS-update's existing before/apply/after checks). `changed` stays unknown unless evidence establishes it.

For bounded multi-resource actions such as migration, cloning, disk movement, and backup create/restore, `context` includes the primary resource plus `related_targets` for each explicit source/destination or subresource known before submission.

### Result example

```json
{
  "schema_version": 1,
  "request_id": "req-start-vm-100",
  "receipt_id": "req-start-vm-100",
  "operation": "vm start",
  "context": {
    "profile": "lab",
    "provider": "proxmox",
    "endpoint": "https://pve.example.net:8006",
    "endpoint_identity": "proxmox@https://pve.example.net:8006",
    "resource_type": "vm",
    "resource_id": "100",
    "node": "pve1"
  },
  "submission": "accepted",
  "execution": "running",
  "verification": "unsupported",
  "changed": null,
  "retry": "reconcile_first",
  "task_id": "UPID:pve1:...",
  "started_at": "2026-09-29T04:00:00Z",
  "observed_at": "2026-09-29T04:00:01Z",
  "next_actions": [
    {
      "operation": "agent receipt refresh",
      "arguments": {"profile": "lab", "request_id": "req-start-vm-100"}
    }
  ]
}
```

Provider names, descriptions, statuses, task messages, and returned `data` are untrusted data. They are never interpreted as NodeX instructions. Next actions contain canonical NodeX operation IDs and typed arguments, never shell command strings.

## Receipts, duplicate protection, and recovery

For supported mutations, NodeX writes an intent receipt before calling the existing handler. The receipt stores the request/receipt IDs, canonical operation, an HMAC-SHA-256 fingerprint of normalized input, the bound profile/provider/endpoint/resource and applicable trust context, submission progress, task ID, observed outcomes, timestamps, errors, and warnings. It does not store raw arguments, credentials, authorization headers, or provider response bodies. Sensitive credential-like flags and key/value fields are refused before fingerprinting.

Receipts live under `<NodeX config directory>/agent-receipts/<request-id>.json`; a random `fingerprint.key` in the same private directory protects low-entropy non-secret input fingerprints against offline dictionary checks. Unix directories/files are restricted to `0700`/`0600`; file ownership, symlinks, regular-file type, size, JSON schema, and identifiers are checked. Windows uses the ACL inherited from the per-user configuration directory; the standard library path here does not independently inspect Windows SIDs. The fingerprint key is local deduplication material, not a provider credential or authorization key. Receipts are local diagnostics and deduplication state, not tamper-proof evidence or authorization records.

Request IDs are identifiers, not secrets: they appear in results and receipt filenames. Agent requests are bounded to 8192 argument bytes; captured handler output is bounded to 1 MiB, and unstructured text exposed as `data` is limited to 64 KiB. For inspections, `observation.completeness` remains `unknown` and pagination metadata is listed as unsupported unless the existing handler exposes stronger evidence. `truncated` reports when NodeX's output capture itself was cut off.

```sh
nodex agent receipt list --output json --limit 20
nodex agent receipt list --output json --request-prefix nxeval-20261004-114406-
nodex agent receipt show req-start-vm-100 --output json
nodex --profile lab agent receipt refresh req-start-vm-100 --output json
nodex --profile lab agent receipt reconcile req-start-vm-100 --output json
```

Receipt listing returns summaries (20 by default, at most 100) and reports whether older entries were omitted. The store is shared by NodeX runs under the same local account; use `--request-prefix` to scope a list to a run's chosen request-ID prefix. Prefix filtering is applied before the limit, and the returned `request_prefix` identifies the selected scope. Refresh/reconcile checks the receipt's profile, provider, normalized endpoint, TLS CA identity, and SSH trust fields against the explicit current profile before connecting. A changed profile identity is refused. For outstanding tasks, it performs provider task inspection only. It never resubmits, cancels, rolls back, or follows a changed profile. It reports unknown when the provider cannot identify the task or the task identifier was not received. Task records may expire at the provider; absence is not proof that the mutation did not happen.

The same request ID and normalized operation/input/target returns the stored result and never resubmits. A replayed result includes `replayed: true` as a top-level machine-readable signal and retains the `DUPLICATE_REQUEST_ID` warning for clients that already inspect warnings. Reusing the ID with different inputs, safety acknowledgements, or context conflicts. If a receipt says the operation was definitely not attempted and the caller wants to try again, use a new request ID after addressing the cause. For `unknown` or `running`, inspect/reconcile first; do not blindly retry. A crash just before provider invocation can conservatively leave an unknown receipt, because a crash at that boundary cannot prove the request was not sent.

Stopping local observation does not cancel a remote task. NodeX does not promise exactly-once execution where the provider has no idempotency key. No automatic receipt pruning is performed. Retain unresolved receipts, request-ID history, and `fingerprint.key` together; remove terminal history only when the operator accepts that deleting it permits that ID to be reused and changes the fingerprint key will make older IDs conflict rather than resume.

## Support matrix

The authoritative per-operation matrix is `nodex operation list --output json`; detailed reasons are in `operation describe`. Agent-mode remote mutations are covered by the same receipt wrapper across provider-backed VM/container lifecycle and configuration, snapshots, creation/clone/migration/disk operations, storage upload/delete, backups and backup schedules, network/firewall, access ACL/user deletion, SDN, Ceph, replication, cluster initialization, and PBS verify/sync/prune/garbage-collection operations. Each operation retains its existing provider and confirmation checks.

Visible exclusions preserve existing specialized safety behavior:

| Operation family | Agent-mode decision | Reason |
|---|---|---|
| `vm console`, `container console`, setup, secret collection, `access user create` | Unsupported | Interactive terminal or secret collection cannot produce the bounded agent result safely. |
| profile/config/credential mutations (`init`, profile add/use/remove/import/credential changes) | Unsupported | Local configuration and credential writes stay in the human CLI workflow. |
| `storage download` | Unsupported | Writes an arbitrary local destination file. |
| maintenance inventory/status/plan/apply/verify/resume/reconcile/abandon | Unsupported | Uses multi-host inventory, immutable plans, and its existing per-host receipt/recovery workflow. `maintenance report` is read-only and available. |
| certification run/cleanup | Unsupported | Uses its dedicated authorization, disposable-resource controls, and ledger. `certification report` remains read-only. |
| `cluster join` | Unsupported | The existing handler performs a preflight and refuses execution without a safe peer credential. |
| `doctor` | Unsupported | Checks every configured profile and fans out across multiple provider endpoints. |
| environment health/backup-health | Unsupported | Can fan out across multiple configured provider profiles rather than one immutable execution context. |
| `monitor check` | Supported for inspection only with one `--target` | The selected target and full monitor configuration are bound from one config snapshot; environment fan-out is refused. |

## Compatibility and trust boundaries

- The normal CLI remains opt-in-compatible: no `--agent` means existing formatting, handler behavior, prompts, and legacy operation result fields are unchanged.
- Existing `OperationResult` schema/version and success semantics are not redefined. Agent result schema v1 wraps the outcome separately.
- NodeX remains a local CLI with direct provider connections. This interface adds no MCP transport or daemon.
- `--agent` is not an isolation boundary against an agent that can execute arbitrary local commands, read files, change profiles, or edit receipt files.
- Local receipts cannot prove human approval; confirmation flags are intent acknowledgements only.
- Verification is limited by provider evidence. Task IDs, resource identity, and task retention are provider-defined. When identity/reconciliation cannot be established, NodeX returns unknown/unsupported rather than guessing.
