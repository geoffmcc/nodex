# Nodex Compatibility Policy (Pre-1.0)

This document defines the compatibility commitments for Nodex before the 1.0
release. It describes which interfaces are considered stable and which may
change, so integrators, script authors, and operators can build against Nodex
with confidence.

## Compatibility Tiers

| Tier | Meaning                                                   |
|------|-----------------------------------------------------------|
| 1    | Backward-compatible changes only (stable).                |
| 2    | Additive changes allowed; existing fields preserved.      |
| 3    | Subject to change with notice in release notes.           |
| 4    | Unstable; may change at any time.                         |

## Command-Line Interface

### Tier 2 — Command paths

The top-level command hierarchy (`nodex <resource> <action>`) is additive.
New subcommands may be added without notice. Existing subcommand names and
argument order will not change without a documented deprecation period.

### Tier 3 — Flags

Global flags (`--profile`, `--output`, `--timeout`, `--wait`, `--yes`,
`--force`, `--expert`, `--all`, `--quiet`, `--verbose`, `--debug`,
`--non-interactive`, `--no-color`, `--limit`) are considered stable in
name and semantics.  New flags may be added.

Mutation-specific flags (e.g. `--node`, `--storage`, `--delete-orphan-disks`)
may be renamed or restructured before 1.0.

### Tier 3 — Argument conventions

Resource addressing uses `<node>/<vmid>` or `<node>/<vmid>/<snapshot>` for
nested resources. This convention is considered stable.

## Configuration

### Tier 2 — Config schema

The config file format (JSON in the XDG config directory) and the profile
schema (`provider`, `endpoint`, `ca_file`, `credential_ref`) are additive.
New fields may appear. Existing fields will not be removed or renamed
without a deprecation period.

### Tier 2 — Profile import/export

The exported profile JSON shape (`name`, `provider`, `endpoint`, `ca_file`)
is considered stable.  Credential data is never exported.

## Output Formats

### JSON output

#### Tier 2 — JSON field names and types

For read-only resource listings (nodes, VMs, containers, storage, tasks,
events, firewall rules, HA resources, backup content, SDN zones/VNets,
pools, cluster log), the JSON shape is additive. New fields may appear.
Existing field names, types, and semantics will not change.

#### Tier 2 — Operation result envelope

The `OperationResult` envelope for mutation commands has the following
stable fields:

| Field         | Type        | Description                                  |
|---------------|-------------|----------------------------------------------|
| `schema`      | `int`       | Schema version (currently 1)                 |
| `operation`   | `string`    | Command name (e.g., `"vm start"`)            |
| `profile`     | `string`    | Profile name (omitted when empty)            |
| `provider`    | `string`    | Provider backend (e.g., `"proxmox"`)         |
| `target`      | `string`    | Resource identifier (omitted when empty)     |
| `safety`      | `string`    | Safety tier label (omitted when empty)       |
| `upid`        | `string`    | Provider task ID (omitted when empty)        |
| `submitted`   | `bool`      | Whether the request was accepted             |
| `waited`      | `bool`      | Whether Nodex waited for task completion     |
| `success`     | `bool`      | Overall success                              |
| `changed`     | `bool|null` | Whether state was modified (null=unknown)    |
| `status`      | `string`    | Provider status text (omitted when empty)    |
| `warnings`    | `[string]`  | Human-readable warnings (omitted when empty) |
| `error`       | `object`    | Error details (omitted on success)           |
| `error.class` | `string`    | Error classification                         |
| `error.exit`  | `int`       | Recommended exit code                        |
| `error.detail`| `string`    | Human-readable error detail                  |

New fields may be added to the envelope. Existing fields will not be removed
or change type. The `schema` field will be incremented for
backward-incompatible shape changes only.

#### Tier 2 — Empty lists

Empty resource lists are represented as `[]` (not `null`) in JSON.

#### Tier 3 — Nested object shapes

The internal shape of domain objects (e.g., `domain.Node`, `domain.VM`)
may gain or lose optional fields before 1.0.

### YAML output

#### Tier 3 — YAML representation

YAML output mirrors the JSON shape using `yaml` struct tags. Field names
and semantics match the JSON contract. The YAML representation may change
formatting details (indentation, key ordering) before 1.0.

### Table output

#### Tier 4 — Table format

Table (text) output is intended for human consumption and may change column
layout, width, and formatting without notice. Scripts should use
`--output json` or `--output yaml`.

## Stdout / Stderr

### Tier 1 — Output stream placement

- **stdout**: Requested data, final operation results, resource listings.
- **stderr**: Prompts, warnings, progress messages, diagnostics, debug logs.

JSON and YAML output written to stdout is always parseable. No human-readable
text is mixed into structured output streams. This separation is considered
stable.

## Exit Codes

### Tier 1 — Exit code classes

| Code | Name               | Meaning                                  |
|------|--------------------|------------------------------------------|
| 0    | Success            | Operation completed successfully.        |
| 1    | General            | Unspecified error.                       |
| 2    | Usage              | Invalid command arguments.               |
| 3    | Config             | Configuration problem.                   |
| 4    | Credential         | Credential unavailable or invalid.       |
| 5    | Auth               | Authentication failed.                   |
| 6    | Authorization      | Authorization denied (safety check).     |
| 7    | Network            | Network connectivity error.              |
| 8    | TLS                | TLS/certificate error.                   |
| 9    | Incompatibility    | Provider/API version incompatibility.    |
| 10   | UnsupportedCap     | Capability not supported by provider.    |
| 11   | PartialFailure     | Partial failure in multi-profile --all.  |
| 12   | Provider           | Provider-specific error.                 |
| 130  | Interrupted        | SIGINT (Ctrl+C).                         |
| 143  | SIGTERM            | SIGTERM.                                 |

This exit code table is considered stable. New codes may be added; existing
codes will not be reassigned.

### Tier 2 — Exit code semantics

- `0` indicates the complete operation succeeded. A mutation request that
  was accepted but whose provider task later failed (when `--wait` is used)
  returns a non-zero exit code.
- `11` (PartialFailure) is returned when `--all` is used and some but not all
  profiles failed.
- When every profile fails under `--all`, exit code `11` is returned (not `0`).

## What Is NOT Covered

These interfaces are pre-1.0 and may change:

- Internal Go package APIs (everything under `internal/`)
- Provider interface signatures
- Debug and verbose log format
- Credential backend wire formats
- Build system and Makefile internals
- Golden test file content (these are test artifacts)
