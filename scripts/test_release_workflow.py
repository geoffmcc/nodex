"""Exercise the actual Linux release shell gates without tags or publishing."""

import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest


WORKFLOW = Path(__file__).resolve().parents[1] / ".github/workflows/release.yml"


def run_block(name):
    """Read a named step's literal shell block; no YAML dependency is needed."""
    lines = WORKFLOW.read_text().splitlines()
    start = lines.index("      - name: " + name)
    for index in range(start + 1, len(lines)):
        if lines[index].startswith("      - "):
            break
        if lines[index] == "        run: |":
            body = []
            for line in lines[index + 1:]:
                if line and not line.startswith("          "):
                    break
                body.append(line[10:] if line else "")
            return "\n".join(body) + "\n"
    raise AssertionError("missing literal run block: " + name)


@unittest.skipIf(sys.platform == "win32" or not shutil.which("bash"),
                 "release shell gates execute on a Linux runner")
class ReleaseWorkflowTests(unittest.TestCase):
    def shell(self, name, values, path=None):
        with tempfile.TemporaryDirectory(prefix="nodex-release-gate-") as directory:
            output = Path(directory) / "output"
            output.touch()
            env = dict(os.environ, GITHUB_OUTPUT=str(output), **values)
            if path is not None:
                env["PATH"] = str(path) + os.pathsep + env["PATH"]
            result = subprocess.run(["bash", "-c", run_block(name)],
                                    cwd=directory, env=env, capture_output=True,
                                    text=True, timeout=10)
            return result, output.read_text()

    def resolve(self, tag, ref="refs/tags/v0.8.0", ref_type="tag", push=False):
        return self.shell("Resolve release tag", {
            "PUSH_TAG": tag if push else "",
            "DISPATCH_TAG": "" if push else tag,
            "RELEASE_REF": ref,
            "RELEASE_REF_TYPE": ref_type,
        })

    def test_tag_push_and_matching_tag_dispatch(self):
        for push in (False, True):
            with self.subTest(push=push):
                result, output = self.resolve("v0.8.0", push=push)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(output, "tag=v0.8.0\n")

    def test_branch_dispatch_keeps_recovery_of_existing_tag(self):
        result, output = self.resolve("v0.8.0", "refs/heads/main", "branch")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(output, "tag=v0.8.0\n")

    def test_wrong_selected_tag_rejected_before_output(self):
        result, output = self.resolve("v0.8.0", "refs/tags/v0.7.0")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("does not match", result.stdout)
        self.assertEqual(output, "")

    def test_malformed_and_multiline_tags_rejected_before_output(self):
        for tag in ("", "0.8.0", "v0.8", "v0.8.0-rc.1", " v0.8.0",
                    "v0.8.0 ", "v00.8.0", "v0.08.0", "v0.8.00", "v0.8.0\n", "v0.8.0\nother=value",
                    "other=value\nv0.8.0", "v0.8.0; echo unexpected"):
            with self.subTest(tag=tag):
                result, output = self.resolve(tag, "refs/heads/main", "branch")
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(output, "")

    def test_signer_identity_uses_exact_executing_ref(self):
        for ref in ("refs/tags/v0.8.0", "refs/heads/main",
                    "refs/heads/fix/release.v2+retry"):
            with self.subTest(ref=ref):
                result, output = self.shell("Resolve expected signer identity", {
                    "REF": ref, "REPO": "https://github.com/geoffmcc/nodex",
                })
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(output,
                                 "value=https://github.com/geoffmcc/nodex/"
                                 ".github/workflows/release.yml@" + ref + "\n")

    def test_signature_verifier_receives_literal_identity(self):
        identity = ("https://github.com/geoffmcc/nodex/.github/workflows/"
                    "release.yml@refs/heads/fix/release.v2+retry")
        with tempfile.TemporaryDirectory(prefix="nodex-cosign-fixture-") as directory:
            command = Path(directory) / "cosign"
            command.write_text("#!/bin/sh\nprintf '%s\\n' \"$@\"\n")
            command.chmod(0o755)
            result, _ = self.shell("Verify release signature bundle", {
                "EXPECTED_IDENTITY": identity,
            }, path=directory)
            self.assertEqual(result.returncode, 0, result.stderr)
            arguments = result.stdout.splitlines()
            self.assertIn("--certificate-identity", arguments)
            self.assertEqual(arguments[arguments.index("--certificate-identity") + 1],
                             identity)
            self.assertNotIn("--certificate-identity-regexp", arguments)

    def test_security_gate_checks_tag_push_and_allows_historical_recovery(self):
        with tempfile.TemporaryDirectory(prefix="nodex-security-gate-fixture-") as directory:
            root = Path(directory)
            git = root / "git"
            git.write_text('#!/bin/sh\nif [ "$1" = show ]; then printf "# policy\\n"; '
                           'elif [ "$1" = rev-parse ]; then printf "%s\\n" "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"; '
                           'else exit 1; fi\n')
            git.chmod(0o755)
            python = root / "python3"
            python.write_text('#!/bin/sh\nprintf "%s\\n" "$@"\n')
            python.chmod(0o755)
            for event in ("push", "workflow_dispatch"):
                with self.subTest(event=event):
                    result, _ = self.shell("Check release security baseline", {
                        "WORKFLOW_COMMIT": "b" * 40, "RUNNER_TEMP": directory,
                        "GITHUB_EVENT_NAME": event, "GITHUB_REPOSITORY": "geoffmcc/nodex",
                    }, path=directory)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    args = result.stdout.splitlines()
                    self.assertIn("--repo", args)
                    self.assertEqual("--expected-commit" in args, event == "push")
                    if event == "push":
                        self.assertEqual(args[args.index("--expected-commit") + 1], "a" * 40)


if __name__ == "__main__":
    unittest.main()
