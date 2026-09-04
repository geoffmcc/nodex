# Releasing Nodex

Releases are tag-driven and are not created by pull requests. The release
workflow builds Linux, Windows, and macOS `amd64` and `arm64` binaries with
`CGO_ENABLED=0`, `-trimpath`, embedded version/commit/date metadata, SHA-256
checksums, and SPDX SBOM artifacts. The workflow creates a draft release so an
operator can inspect artifacts before publishing.

## Validate Locally

Use the pinned GoReleaser version from `.github/workflows/release.yml` and run:

```bash
go mod verify
go install github.com/goreleaser/goreleaser/v2@v2.10.0
goreleaser check
goreleaser release --snapshot --clean
```

Snapshot builds do not create tags or publish releases. Set
`SOURCE_DATE_EPOCH` for reproducible archive timestamps.

## Verify Artifacts

Verify `checksums.txt` with `sha256sum -c checksums.txt` (or the platform
equivalent), inspect `nodex version --output json`, and validate the attached
SPDX document before installation. Upgrade and downgrade by replacing the
binary only after verifying its checksum and signature/provenance supplied by
the publishing environment.
