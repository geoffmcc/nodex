# Contributing to Nodex

Thank you for improving Nodex. This project is a Go CLI for local infrastructure inspection, currently focused on a read-only Proxmox provider.

## Prerequisites

- Go `1.25.12`
- `make`
- Git
- Optional: `golangci-lint` if you want to run the configured extended lint set locally

The CI workflow runs `go build`, `go vet`, `go test`, and a `gofmt -s` check on Ubuntu, macOS, and Windows.

## Set up a development checkout

```bash
git clone https://github.com/geoffmcc/nodex.git
cd nodex
go test ./...
```

Build the CLI:

```bash
make build
./nodex version
```

On Windows PowerShell, run the built executable as `./nodex.exe version` when building with a Windows Go toolchain.

## Development commands

```bash
make build      # go build with version ldflags; writes ./nodex
make test       # go test ./...
make test-race  # go test -race ./...
make vet        # go vet ./...
make fmt        # gofmt -s -w .
make lint       # gofmt check + go vet
make clean      # remove ./nodex
```

If `golangci-lint` is available, you can also run it against `.golangci.yml`:

```bash
golangci-lint run
```

This is configured in the repository but is not currently part of the GitHub Actions workflow.

## Testing notes

- Unit tests isolate configuration and home directories with temporary paths.
- CLI end-to-end tests use an in-process mock provider rather than a live Proxmox server.
- Do not add tests that require a real Proxmox host unless they are explicitly skipped or gated behind a clear opt-in mechanism.
- Use fictional domains such as `example.com` or `example.invalid` and fake credentials in tests and documentation.

## Documentation expectations

When changing user-visible behavior, update the relevant documentation in the same change:

- `README.md` for onboarding and high-level behavior.
- `docs/cli-reference.md` for commands, flags, output, and exit codes.
- `docs/configuration.md` for configuration, credentials, TLS, and paths.
- `docs/architecture.md` for package boundaries or provider architecture changes.
- `docs/nodex.1` when CLI syntax or behavior changes.

Verify command examples against the actual CLI. Global flags must appear before the command name, for example `nodex --output json node list`.

## Pull requests

Before opening a pull request, run at least:

```bash
make lint
make test
```

Include a concise description of the behavior change, the verification performed, and any compatibility or security considerations.

## Security-sensitive changes

Credential handling, TLS behavior, output redaction, terminal sanitization, and provider request construction are security-sensitive areas. Avoid logging secrets, avoid real infrastructure details in fixtures or docs, and prefer tests that prove sensitive values are redacted or omitted.
