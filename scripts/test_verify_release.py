import io
import json
import os
from pathlib import Path
import tarfile
import tempfile
import unittest
from types import SimpleNamespace
from unittest.mock import patch
import zipfile

from verify_release import (NOTICES, extract_source, inspect_binary, inspect_sbom,
                            notice_matches, verify_native_version,
                            validate_names, verify_vendor_license_coverage)


class ReleaseArchiveTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        for name in NOTICES:
            (self.root / name).write_text(f"current {name}\n")

    def write_sbom(self, path, modules=None, binary_digest="abc123", go_version="go1.27.2"):
        modules = modules or {
            "github.com/geoffmcc/nodex": "v0.4.2",
            "example.org/dependency": "v1.2.3",
        }
        packages = [
            {"name": name, "versionInfo": version, "externalRefs": [
                {"referenceCategory": "PACKAGE-MANAGER", "referenceType": "purl",
                 "referenceLocator": f"pkg:golang/{name}@{version}"}
            ]}
            for name, version in modules.items()
        ]
        packages.extend([
            {"name": "stdlib", "versionInfo": go_version,
             "externalRefs": [{"referenceCategory": "PACKAGE-MANAGER",
                               "referenceType": "purl",
                               "referenceLocator": f"pkg:golang/stdlib@{go_version[2:]}"}]},
            {"name": "dist/nodex", "versionInfo": f"sha256:{binary_digest}"},
        ])
        document = {
            "spdxVersion": "SPDX-2.3",
            "dataLicense": "CC0-1.0",
            "documentNamespace": "https://example.invalid/sbom",
            "creationInfo": {"creators": ["Tool: syft-[not provided]"]},
            "packages": packages,
        }
        path.write_text(json.dumps(document))

    def source(self, omit=None, extra=None):
        files = {name: (self.root / name).read_bytes() for name in NOTICES}
        files.update({name: b"source" for name in (
            "go.mod", "go.sum", "vendor/modules.txt", "cmd/nodex/main.go",
            ".goreleaser.yaml", ".github/workflows/release.yml")})
        files.pop(omit, None)
        path = self.root / "source.tar.gz"
        with tarfile.open(path, "w:gz") as archive:
            for name, content in files.items():
                entry = tarfile.TarInfo(f"source/{name}")
                entry.size = len(content)
                archive.addfile(entry, io.BytesIO(content))
            if extra:
                archive.addfile(extra)
        return path

    def test_source_retains_notices_and_vendor(self):
        source = extract_source(self.source(), self.root / "extracted", self.root)
        self.assertEqual((source / "vendor/modules.txt").read_bytes(), b"source")
        self.assertEqual((source / "LICENSE").read_bytes(), (self.root / "LICENSE").read_bytes())

    def test_missing_vendor_fails(self):
        with self.assertRaisesRegex(ValueError, "vendor/modules.txt"):
            extract_source(self.source(omit="vendor/modules.txt"), self.root / "extracted", self.root)

    def test_missing_notice_fails(self):
        with self.assertRaisesRegex(ValueError, "THIRD_PARTY_LICENSES"):
            extract_source(self.source(omit="THIRD_PARTY_LICENSES.md"), self.root / "extracted", self.root)

    def test_symlink_fails_before_extraction(self):
        link = tarfile.TarInfo("source/link")
        link.type = tarfile.SYMTYPE
        link.linkname = "../../outside"
        with self.assertRaisesRegex(ValueError, "Unexpected source entry"):
            extract_source(self.source(extra=link), self.root / "extracted", self.root)
        self.assertFalse((self.root / "extracted").exists())

    def test_unsafe_and_duplicate_names_fail(self):
        for name in ("", ".", "../outside", "/outside", "source/../../outside", "C:/outside", "source\\file"):
            with self.subTest(name=name), self.assertRaises(ValueError):
                validate_names([name])
        with self.assertRaisesRegex(ValueError, "Duplicate"):
            validate_names(["source/LICENSE", "source/./LICENSE"])

    def test_stale_binary_notice_fails(self):
        path = self.root / "binary.zip"
        with zipfile.ZipFile(path, "w") as archive:
            for name in NOTICES | {"README.md", "nodex.exe"}:
                archive.writestr(name, "outdated")
        with self.assertRaisesRegex(ValueError, "Stale notice"):
            inspect_binary(path, "nodex.exe", self.root)

    def test_notice_line_endings_are_platform_independent(self):
        notice = self.root / "LICENSE"
        notice.write_bytes(b"current LICENSE\r\n")
        self.assertTrue(notice_matches(b"current LICENSE\n", notice))

    def test_vendor_and_license_inventory_must_match(self):
        vendor = self.root / "modules.txt"
        licenses = self.root / "THIRD_PARTY_LICENSES.md"
        vendor.write_text("# example.org/one v1.2.3\n## explicit; go 1.27.0\n"
                          "# example.org/two v4.5.6\n")
        licenses.write_text("## Third-Party Licenses\n\n"
                            "## example.org/one v1.2.3\n\n"
                            "## example.org/two v4.5.6\n")
        verify_vendor_license_coverage(vendor, licenses)

    def test_missing_license_entry_fails(self):
        vendor = self.root / "modules.txt"
        licenses = self.root / "THIRD_PARTY_LICENSES.md"
        vendor.write_text("# example.org/module v1.2.3\n")
        licenses.write_text("## Third-Party Licenses\n")
        with self.assertRaisesRegex(ValueError, "missing license entries"):
            verify_vendor_license_coverage(vendor, licenses)

    def test_license_version_mismatch_fails(self):
        vendor = self.root / "modules.txt"
        licenses = self.root / "THIRD_PARTY_LICENSES.md"
        vendor.write_text("# example.org/module v1.2.3\n")
        licenses.write_text("## example.org/module v1.2.2\n")
        with self.assertRaisesRegex(ValueError, "version mismatch"):
            verify_vendor_license_coverage(vendor, licenses)

    def test_spdx_sbom_requires_tool_and_package_inventory(self):
        path = self.root / "nodex-linux-amd64.spdx.json"
        self.write_sbom(path)
        inspect_sbom(path, {
            "github.com/geoffmcc/nodex": "v0.4.2",
            "example.org/dependency": "v1.2.3",
        }, "go1.27.2", {"CGO_ENABLED": "0"}, "nodex", "abc123")

    def test_spdx_sbom_without_package_inventory_fails(self):
        path = self.root / "nodex-linux-amd64.spdx.json"
        self.write_sbom(path)
        document = json.loads(path.read_text())
        document["packages"] = []
        path.write_text(json.dumps(document))
        with self.assertRaisesRegex(ValueError, "no package inventory"):
            inspect_sbom(path, {}, "go1.27.2", {"CGO_ENABLED": "0"}, "nodex", "abc123")

    def test_spdx_sbom_must_match_binary_dependencies(self):
        path = self.root / "nodex-linux-amd64.spdx.json"
        self.write_sbom(path, modules={
            "github.com/geoffmcc/nodex": "v0.4.2",
        })
        with self.assertRaisesRegex(ValueError, "Go module inventory mismatch"):
            inspect_sbom(path, {
                "github.com/geoffmcc/nodex": "v0.4.2",
                "example.org/dependency": "v1.2.3",
            }, "go1.27.2", {"CGO_ENABLED": "0"}, "nodex", "abc123")

    def test_spdx_sbom_must_match_binary_digest(self):
        path = self.root / "nodex-linux-amd64.spdx.json"
        self.write_sbom(path)
        with self.assertRaisesRegex(ValueError, "executable digest"):
            inspect_sbom(path, {
                "github.com/geoffmcc/nodex": "v0.4.2",
                "example.org/dependency": "v1.2.3",
            }, "go1.27.2", {"CGO_ENABLED": "0"}, "nodex", "wrongdigest")

    @patch("verify_release.subprocess.run")
    def test_native_version_smoke_makes_binary_executable(self, run):
        def check_executable(command, **kwargs):
            if os.name != "nt":
                self.assertTrue(os.access(command[0], os.X_OK))
            return SimpleNamespace(stdout="Nodex 1.2.3\nCommit: abc123\n")

        run.side_effect = check_executable
        verify_native_version(b"test binary", "nodex", "1.2.3", "abc123")

    def test_non_spdx_document_fails(self):
        path = self.root / "archive.tar.gz.spdx.json"
        path.write_text('{"spdxVersion":"SPDX-2.2"}')
        with self.assertRaisesRegex(ValueError, "Not an SPDX 2.3"):
            inspect_sbom(path, {}, "go1.27.2", {"CGO_ENABLED": "0"}, "nodex", "abc123")


if __name__ == "__main__":
    unittest.main()
