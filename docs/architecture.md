# Architecture

## Dependency Direction

```
cmd/nodex → internal/app → internal/provider/proxmox
                ↓                ↓
          internal/config    internal/transport/httpclient
          internal/credentials
          internal/output
          internal/redact
          internal/logging
          internal/diagnostics
```

No circular dependencies. `internal/domain` contains shared types only.

## Package Layout

```
cmd/nodex/              Entry point, signal handling
internal/
  app/                  Use-case orchestration
  cli/                  Commands, flags, help, completion
  config/               Schema, paths, read/write, locking
  credentials/          Backends: keyring, file, env, stdin
  diagnostics/          Doctor checks (local + profile)
  domain/               Shared types: Node, VM, Container, Storage, Provider interfaces
  logging/              Structured logging, level management
  output/               Table, JSON, YAML formatters
  redact/               Centralized secret redaction
  provider/
    registry.go         Built-in provider registry
    capabilities.go     Capability identifiers
    proxmox/
      client/           Minimal Proxmox HTTP client
      provider.go       Provider implementation
      mapper.go         API response → domain mapping
  transport/httpclient/ HTTP client, TLS, timeouts, retry
  version/              Build metadata
```

## Provider Interfaces

```go
type Provider interface {
    Name() string
    Capabilities(context.Context) Capabilities
    Health(context.Context) HealthResult
}

type NodeReader interface {
    ListNodes(context.Context, ListOptions) ([]Node, error)
}

type VMReader interface {
    ListVMs(context.Context, ListOptions) ([]VM, error)
}

type ContainerReader interface {
    ListContainers(context.Context, ListOptions) ([]Container, error)
}

type StorageReader interface {
    ListStorage(context.Context, ListOptions) ([]Storage, error)
}
```

## Capability Identifiers

```
health.read
nodes.read
vms.read
containers.read
storage.read
```

## Proxmox Client Strategy

- Minimal HTTP client using `net/http`
- No third-party Proxmox SDK
- API token authentication via `PVEAPIToken` header
- Targets Proxmox VE 9.x (secondary: 8.4)
- Endpoints: `/nodes`, `/cluster/resources`, `/version`

## Concurrency

- List commands: one request at a time
- Doctor: concurrent local checks
- Bounded goroutines, context propagation throughout

## Redaction

Dedicated `internal/redact` package:
- All output sinks pass through redaction
- Fuzz-tested
- Table-driven tests for every secret pattern
- Release blocked if any known secret marker passes unredacted

## Threat Model (v0.1)

1. Credential exposure → never in config, logs, errors, or diagnostics
2. TLS interception → validation by default, custom CA support
3. Malicious terminal text → sanitize escape sequences
4. Unsafe permissions/symlinks → restrictive modes, atomic writes
5. Config corruption → exit 3, no auto-repair
6. Dependency/release tampering → vulnerability scanning, signed releases
7. Wrong-profile operation → explicit `--profile`, resolution order
