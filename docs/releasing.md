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

Before creating a tag, check that `main` is the commit to release, both CodeQL
analyses are current for that commit, and no high or critical code-scanning
alerts remain open on `main`:

```bash
python3 scripts/check_release_security.py --expected-commit "$(git rev-parse HEAD)"
```

This needs authenticated `gh` access with code-scanning read permission. A
missing or stale analysis, unavailable API, or open high/critical security
alert blocks the check. The release workflow repeats it before building; a tag
push must target the current `main`. This check does not prevent someone from
creating a tag, so run it **before** tagging to avoid consuming a version.
For an existing-tag recovery dispatch, the gate still checks current `main`, but
does not require the historical tag to equal it. This is not a security review
of historical tagged source.

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

## Updating dependency notices

Dependabot groups related `golang.org/x/*` updates, but it does not update
`THIRD_PARTY_LICENSES.md`. A Go module update may also change transitive
versions, so review the full module graph rather than only the PR title.

For every changed production module, inspect the exact new module source
(`go mod download -json module@version` reports its local directory). Compare
its license and patent texts with the existing inventory and check its used
source directories for nested licenses or notices. Update the inventory's
version headings and any changed texts; add or remove entries when the set of
production modules changes. Do not update headings alone without reviewing the
underlying notices. For Go toolchain changes, review the included Go and
standard-library vendor notices as well.

To check the resulting inventory against the vendored production modules
without adding a vendor directory to the checkout, run:

```bash
vendor_dir=$(mktemp -d)
go mod vendor -o "$vendor_dir/vendor"
python3 - "$vendor_dir/vendor/modules.txt" <<'PYCODE'
import sys
from pathlib import Path
from scripts.verify_release import verify_vendor_license_coverage
verify_vendor_license_coverage(Path(sys.argv[1]), Path("THIRD_PARTY_LICENSES.md"))
print("Vendored module versions match the license inventory.")
PYCODE
```

Remove the temporary directory when finished. This check verifies module
coverage and versions; it does not verify the legal text or find nested notices.
The full snapshot verification above remains necessary before release.

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
