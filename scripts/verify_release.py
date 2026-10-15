#!/usr/bin/env python3
"""Check release notices, checksum coverage, and offline source builds."""

import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import subprocess
import tarfile
import tempfile
import zipfile


TARGETS = [(os_name, arch) for os_name in ("linux", "darwin", "windows")
           for arch in ("amd64", "arm64")]
NOTICES = {"LICENSE", "THIRD_PARTY_LICENSES.md", "SOURCE.md"}


def validate_names(names):
    seen = set()
    for name in names:
        path = PurePosixPath(name)
        if (not path.parts or path.is_absolute() or ".." in path.parts or "\\" in name
                or ":" in name):
            raise ValueError(f"Unsafe archive path: {name}")
        normalized = str(path)
        if normalized in seen:
            raise ValueError(f"Duplicate archive path: {name}")
        seen.add(normalized)
    return seen


def require_files(names, required):
    missing = required - names
    if missing:
        raise ValueError(f"Missing archive files: {', '.join(sorted(missing))}")


def notice_matches(archived, repository_file):
    expected = repository_file.read_bytes()
    normalize = lambda content: content.replace(b"\r\n", b"\n").replace(b"\r", b"\n")
    return normalize(archived) == normalize(expected)


def module_versions_from_vendor(path):
    modules = {}
    for line in path.read_text().splitlines():
        if not line.startswith("# "):
            continue
        fields = line[2:].split()
        if len(fields) != 2 or not fields[1].startswith("v"):
            continue
        module, version = fields
        if module in modules:
            raise ValueError(f"Duplicate vendored module: {module}")
        modules[module] = version
    if not modules:
        raise ValueError("No modules found in vendor/modules.txt")
    return modules


def module_versions_from_licenses(path):
    modules = {}
    for line in path.read_text().splitlines():
        match = re.fullmatch(r"## (\S+) (v\S+)", line)
        if not match:
            continue
        module, version = match.groups()
        if module in modules:
            raise ValueError(f"Duplicate third-party license entry: {module}")
        modules[module] = version
    return modules


def verify_vendor_license_coverage(vendor_modules, licenses):
    vendored = module_versions_from_vendor(vendor_modules)
    licensed = module_versions_from_licenses(licenses)
    missing = vendored.keys() - licensed.keys()
    extra = licensed.keys() - vendored.keys()
    mismatched = {module for module in vendored.keys() & licensed.keys()
                  if vendored[module] != licensed[module]}
    if missing or extra or mismatched:
        details = []
        if missing:
            details.append("missing license entries: " + ", ".join(sorted(missing)))
        if extra:
            details.append("license entries absent from vendor: " + ", ".join(sorted(extra)))
        if mismatched:
            details.append("version mismatch: " + ", ".join(
                f"{module} (vendor {vendored[module]}, licenses {licensed[module]})"
                for module in sorted(mismatched)))
        raise ValueError("Third-party license inventory does not match vendored modules; "
                         + "; ".join(details))


def inspect_sbom(path):
    try:
        document = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        raise ValueError(f"Invalid SPDX SBOM {path.name}: {error}") from error
    if not isinstance(document, dict) or document.get("spdxVersion") != "SPDX-2.3":
        raise ValueError(f"Not an SPDX 2.3 document: {path.name}")
    if document.get("dataLicense") != "CC0-1.0" or not document.get("documentNamespace"):
        raise ValueError(f"Incomplete SPDX metadata: {path.name}")
    creation = document.get("creationInfo")
    creators = creation.get("creators", []) if isinstance(creation, dict) else []
    if not any(isinstance(creator, str) and creator.lower().startswith("tool: syft-")
               for creator in creators):
        raise ValueError(f"SBOM does not identify Syft as its creator: {path.name}")
    packages = document.get("packages")
    if not isinstance(packages, list) or not packages:
        raise ValueError(f"SBOM has no package inventory: {path.name}")


def inspect_binary(path, executable, repository):
    if path.suffix == ".zip":
        with zipfile.ZipFile(path) as archive:
            names = validate_names(archive.namelist())
            require_files(names, NOTICES | {"README.md", executable})
            for notice in NOTICES:
                if not notice_matches(archive.read(notice), repository / notice):
                    raise ValueError(f"Stale notice in {path.name}: {notice}")
    else:
        with tarfile.open(path, "r:gz") as archive:
            members = archive.getmembers()
            names = validate_names(member.name for member in members)
            require_files(names, NOTICES | {"README.md", executable})
            for member in members:
                if not (member.isfile() or member.isdir()):
                    raise ValueError(f"Unexpected archive entry: {member.name}")
            for notice in NOTICES:
                with archive.extractfile(notice) as content:
                    if not notice_matches(content.read(), repository / notice):
                        raise ValueError(f"Stale notice in {path.name}: {notice}")


def extract_source(path, destination, repository):
    with tarfile.open(path, "r:gz") as archive:
        members = archive.getmembers()
        names = validate_names(member.name for member in members)
        roots = {PurePosixPath(name).parts[0] for name in names}
        if len(roots) != 1:
            raise ValueError("Source archive must have one top-level directory")
        prefix = roots.pop()
        required = NOTICES | {"go.mod", "go.sum", "vendor/modules.txt",
                              "cmd/nodex/main.go", ".goreleaser.yaml",
                              ".github/workflows/release.yml"}
        require_files(names, {f"{prefix}/{name}" for name in required})
        for member in members:
            if not (member.isfile() or member.isdir()):
                raise ValueError(f"Unexpected source entry: {member.name}")
        # Copy only ordinary files/directories; no symlinks or special entries.
        for member in members:
            target = destination.joinpath(*PurePosixPath(member.name).parts)
            if member.isdir():
                target.mkdir(parents=True, exist_ok=True)
            else:
                target.parent.mkdir(parents=True, exist_ok=True)
                with archive.extractfile(member) as content:
                    target.write_bytes(content.read())
        source = destination / prefix
        for notice in NOTICES:
            if not notice_matches((source / notice).read_bytes(), repository / notice):
                raise ValueError(f"Stale source notice: {notice}")
        return source


def verify(dist, repository):
    sources = list(dist.glob("nodex_*_source.tar.gz"))
    if len(sources) != 1:
        raise ValueError("Expected exactly one corresponding-source archive")
    source_archive = sources[0]
    stem = source_archive.name.removesuffix("_source.tar.gz")
    expected = [source_archive]
    for goos, arch in TARGETS:
        extension = "zip" if goos == "windows" else "tar.gz"
        binary = dist / f"{stem}_{goos}_{arch}.{extension}"
        inspect_binary(binary, "nodex.exe" if goos == "windows" else "nodex", repository)
        expected.append(binary)
        sbom = dist / f"{binary.name}.spdx.json"
        inspect_sbom(sbom)
        expected.append(sbom)
    checksums = {}
    for line in (dist / "checksums.txt").read_text().splitlines():
        digest, name = line.split(maxsplit=1)
        name = name.lstrip("*")
        if name in checksums:
            raise ValueError(f"Duplicate checksum entry: {name}")
        checksums[name] = digest
    for path in expected:
        if hashlib.sha256(path.read_bytes()).hexdigest() != checksums.get(path.name):
            raise ValueError(f"Missing or incorrect checksum: {path.name}")
    with tempfile.TemporaryDirectory(prefix="nodex-source-check-") as temporary:
        workspace = Path(temporary)
        source = extract_source(source_archive, workspace, repository)
        verify_vendor_license_coverage(source / "vendor/modules.txt",
                                      source / "THIRD_PARTY_LICENSES.md")
        for goos, arch in TARGETS:
            env = dict(os.environ, GOOS=goos, GOARCH=arch, CGO_ENABLED="0",
                       GOPROXY="off", GOSUMDB="off", GOTOOLCHAIN="local",
                       GOMODCACHE=str(workspace / "empty-module-cache"),
                       GOWORK="off", GOFLAGS="")
            subprocess.run(["go", "build", "-mod=vendor", "-trimpath", "-o",
                            str(workspace / f"nodex-{goos}-{arch}"), "./cmd/nodex"],
                           cwd=source, env=env, check=True)
            print(f"Verified offline source build: {goos}/{arch}", flush=True)
    print("Verified all six binary archives, SPDX SBOMs, notices, source, and checksum coverage.")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dist", type=Path, default=Path("dist"))
    options = parser.parse_args()
    verify(options.dist.resolve(), Path(__file__).resolve().parents[1])
