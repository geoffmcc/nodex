import io
from pathlib import Path
import tarfile
import tempfile
import unittest
import zipfile

from verify_release import (NOTICES, extract_source, inspect_binary,
                            validate_names, verify_vendor_license_coverage)


class ReleaseArchiveTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        for name in NOTICES:
            (self.root / name).write_text(f"current {name}\n")

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


if __name__ == "__main__":
    unittest.main()
