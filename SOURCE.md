# Source code and licenses

Nodex is distributed under GPL version 3. See `LICENSE` for its terms and
`THIRD_PARTY_LICENSES.md` for dependency notices.

For the version printed by `nodex version`, open the matching release at:

https://github.com/geoffmcc/nodex/releases

Download `nodex_<version>_source.tar.gz` from that release, alongside the binary
archives. It contains the tagged Nodex source, build configuration, embedded
playbooks and callback, and a `vendor/` directory containing the Go dependencies
and their upstream notices. The release's signed `checksums.txt` includes this
source archive. GitHub's automatic **Source code** archives contain the repository
alone; use the explicitly attached `_source.tar.gz` archive for vendored dependencies.

## Build from the source archive

Extract the archive and enter its top-level directory. Install the Go version
pinned in `.github/workflows/release.yml`. Go toolchains and their source archives
are available at https://go.dev/dl/.

On Linux or macOS:

```sh
CGO_ENABLED=0 GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local \
  go build -mod=vendor -trimpath -o nodex ./cmd/nodex/
```

On Windows PowerShell:

```powershell
$env:CGO_ENABLED = '0'
$env:GOPROXY = 'off'
$env:GOSUMDB = 'off'
$env:GOTOOLCHAIN = 'local'
go build -mod=vendor -trimpath -o nodex.exe ./cmd/nodex/
```

These commands need no module downloads and build for the host platform. Set
`GOOS` to `linux`, `darwin`, or `windows` and `GOARCH` to `amd64` or `arm64` to
cross-compile. Without the version linker flags the executable reports a
development version; `.goreleaser.yaml` records the release flags, and the
release tag identifies the exact source commit. These instructions build the
program but do not claim byte-for-byte reproduction of a signed release.

The Go toolchain and optional tools such as Ansible and OpenSSH are installed
separately. Their executables are not included in Nodex's binary archives.
Redistributors should retain the license notices and provide the matching source
and build materials when distributing their own builds.
