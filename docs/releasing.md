# Releasing Nodex

Releases are tag-driven and are not created by pull requests. The release
workflow builds Linux, Windows, and macOS `amd64` and `arm64` binaries with
`CGO_ENABLED=0`, `-trimpath`, embedded version/commit/date metadata, SHA-256
checksums, and SPDX SBOM artifacts. The workflow creates a draft release so an
operator can inspect artifacts before publishing. The checksum manifest is
signed with a Sigstore bundle and the workflow requires build provenance
attestation permissions; a release is not considered publishable until both
are verified.

## Validate Locally

Use the pinned GoReleaser version from `.github/workflows/release.yml` and run:

```bash
go mod verify
go install github.com/goreleaser/goreleaser/v2@v2.10.0
go install github.com/anchore/syft/cmd/syft@v1.30.0
goreleaser check
syft version
goreleaser release --snapshot --clean --skip=sign
```

Snapshot builds do not create tags or publish releases. Set
`SOURCE_DATE_EPOCH` for reproducible archive timestamps.

## Verify Artifacts

Verify `checksums.txt` with `sha256sum -c checksums.txt` (or the platform
equivalent), inspect `nodex version --output json`, and validate the attached
SPDX document before installation. For a published draft, verify
`checksums.txt.sigstore.json` with `cosign verify-blob --bundle` using the
GitHub Actions certificate identity and OIDC issuer
`https://token.actions.githubusercontent.com`. Also verify the GitHub build
provenance attestation for the checksum manifest before installation.
