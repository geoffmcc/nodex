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
go install github.com/anchore/syft/cmd/syft@v1.54.1
goreleaser check
syft version
goreleaser release --snapshot --clean --skip=sign,publish
python3 scripts/verify_release.py
```

Snapshot builds do not create tags or publish releases. Set
`SOURCE_DATE_EPOCH` for reproducible archive timestamps.

The verifier checks notices in every binary and source archive, compares each
binary's SPDX module list and digest with the embedded Go build information and
actual executable, verifies SBOM checksum entries and vendored-module license
coverage, then extracts the source and builds all six targets with an empty
module cache and `GOPROXY=off`, `GOSUMDB=off`, and `GOTOOLCHAIN=local`. The Go
toolchain must already be installed. Run this with the release workflow's pinned
Go version.
CI runs the same snapshot verification on Linux, macOS, and Windows. Each runner
checks all six target archives and executes the native archive's version command;
the verifier also cross-builds the vendored source for all six targets. The
tagged release workflow checks the real draft artifacts, checksum signature,
and provenance before publication.

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

## Recover a failed release without a new version

Do not move or recreate a release tag. Inspect the failed step and reuse the
same tag. If the failed run left a draft, confirm it is still a draft before
deleting that draft release; retain the Git tag. Never delete a published
release as part of this recovery path.

For a transient failure with no workflow changes, dispatch from the existing
tag so the checksum certificate keeps the documented tag identity:

```bash
gh workflow run release.yml --ref v0.8.0 --field tag=v0.8.0
```

Replace both version values with the existing release tag. A tag-ref dispatch
whose input names a different tag is rejected before building. Newline-bearing
or otherwise malformed tag inputs are also rejected.

If the release workflow itself needs a fix, merge that fix first and dispatch
from the reviewed branch while keeping the original source tag as input:

```bash
gh workflow run release.yml --ref main --field tag=v0.8.0
```

The checkout still comes from the existing tag; the executing workflow and its
signing identity come from the selected branch. The workflow verifies the
exact identity, without treating repository names or refs as regular
expressions. For branch recovery, record the workflow run and expected
identity in the draft release notes. The README's tag-only signer regexp
intentionally will not accept branch recovery; consumers must review that
recovery run and explicitly verify its branch identity instead, for example:

```bash
cosign verify-blob --bundle checksums.txt.sigstore.json \
  --certificate-identity 'https://github.com/geoffmcc/nodex/.github/workflows/release.yml@refs/heads/main' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
```

This is an explicit recovery exception, not a reason to broaden normal signer
verification. Keep downloads and local rehearsal output under `/tmp`, separate
from the source checkout. Artifact provenance verification requires a GitHub
CLI version that provides `gh attestation verify`.

## Historical corrections

The seven older releases (v0.1.2, v0.1.3, v0.2.0, v0.3.0, v0.3.1, v0.4.1, and
v0.4.2) were corrected with version-specific supplemental source archives,
third-party license notices, and separate checksum manifests. The original
release assets remain unchanged.
