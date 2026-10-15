# Releasing Nodex

Releases are tag-driven and are not created by pull requests. The release
workflow builds Linux, Windows, and macOS `amd64` and `arm64` binaries with
`CGO_ENABLED=0`, `-trimpath`, embedded version/commit/date metadata, SHA-256
checksums, and SPDX SBOM artifacts. The workflow creates a draft release so an
operator can inspect artifacts before publishing. The checksum manifest is
signed with a Sigstore bundle and the workflow requires build provenance
attestation permissions; a release is not considered publishable until both
are verified.

Every release also includes `nodex_<version>_source.tar.gz`, containing the
tagged repository plus vendored dependency source and upstream license notices.
GoReleaser creates it from the exact release commit and includes it in the signed
checksum manifest. Its preparation hook writes dependencies to the ignored
`.release-source/vendor/` directory; this does not change normal local builds
to vendor mode. Binary archives include `LICENSE`, `THIRD_PARTY_LICENSES.md`, and
`SOURCE.md`. Release notes link directly to the corresponding source archive.

## Validate Locally

Use the pinned GoReleaser version from `.github/workflows/release.yml` and run:

```bash
go mod verify
go install github.com/goreleaser/goreleaser/v2@v2.10.0
go install github.com/anchore/syft/cmd/syft@v1.30.0
goreleaser check
syft version
goreleaser release --snapshot --clean --skip=sign
python3 scripts/verify_release.py
```

Snapshot builds do not create tags or publish releases. Set
`SOURCE_DATE_EPOCH` for reproducible archive timestamps.

The verifier checks notices in every binary and source archive, verifies checksum
coverage, then extracts the source and builds all six targets with an empty module
cache and `GOPROXY=off`, `GOSUMDB=off`, and `GOTOOLCHAIN=local`. The Go toolchain
must already be installed. Run this with the release workflow's pinned Go version.
CI runs the same check on snapshot archives (without signing or SBOM generation);
the release workflow checks the real draft artifacts before publication.

Source archives use committed files. Commit the intended changes before a
snapshot build; uncommitted source changes are not included by `git archive`.

## Verify Artifacts

Verify `checksums.txt` with `sha256sum -c checksums.txt` (or the platform
equivalent), inspect `nodex version --output json`, and validate the attached
SPDX document before installation. For a published draft, verify
`checksums.txt.sigstore.json` with `cosign verify-blob --bundle` using the
GitHub Actions certificate identity and OIDC issuer
`https://token.actions.githubusercontent.com`. Also verify the GitHub build
provenance attestation for the checksum manifest before installation.

Before publishing the draft, confirm all seven archives (six binary archives
plus source), six binary SBOMs, checksums, and the signature bundle are attached.
Confirm the source link in the release notes points to the same tag. Preserve
these source materials while distributing the binaries. Re-review dependency
and file-level notices whenever module versions or the Go toolchain change.

## Corrections to older releases

Preserve old tags and signed archives. Add separately named, version-specific
supplemental notices and source archives with their own checksum manifest, and
add prominent links and a correction date to the original release description.
Historical source archives must contain that tag's code and dependencies; label
any added licensing documentation as a later supplement. Check the actual
historical toolchain's notices as well as its module versions.

Do not overwrite original archives or `checksums.txt`: they are already signed
and may have been downloaded. Mark corrected releases as superseding the older
ones. If incomplete binary downloads need to be withdrawn, treat that as an
explicit release-management action, preserving their source and correction
materials. An immutable release may require a separate correction release with
links instead of additional assets.
