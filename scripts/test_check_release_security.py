import contextlib
import io
import subprocess
import unittest
from unittest import mock

import check_release_security as gate


HEAD = "a" * 40


def analyses(head=HEAD):
    return [{"category": category, "commit_sha": head, "tool": {"name": "CodeQL"}}
            for category in sorted(gate.REQUIRED_CATEGORIES)]


def alert(severity, tags=None):
    return {"number": 42, "rule": {"security_severity_level": severity, "tags": tags or []}}


class ReleaseSecurityGateTests(unittest.TestCase):
    def test_clean_and_low_medium_alerts_pass(self):
        gate.validate(HEAD, analyses(), [alert("low", ["security"]), alert("medium", ["security"]), alert(None)])

    def test_high_and_critical_alerts_block(self):
        for severity in ("high", "critical"):
            with self.subTest(severity=severity), self.assertRaisesRegex(gate.GateError, "42"):
                gate.validate(HEAD, analyses(), [alert(severity, ["security"])])

    def test_missing_or_unknown_security_severity_blocks(self):
        for item in (alert(None, ["security"]), alert("unexpected"), {"rule": None}):
            with self.subTest(item=item), self.assertRaises(gate.GateError):
                gate.validate(HEAD, analyses(), [item])

    def test_missing_stale_or_wrong_tool_analysis_blocks(self):
        wrong_tool = analyses()
        wrong_tool[0]["tool"] = {"name": "another tool"}
        for items in ([], analyses()[:1], analyses("b" * 40), wrong_tool):
            with self.subTest(items=items), self.assertRaises(gate.GateError):
                gate.validate(HEAD, items, [])

    def test_latest_scan_must_match_head(self):
        stale = analyses("b" * 40)
        with self.assertRaises(gate.GateError):
            gate.validate(HEAD, stale + analyses(), [])

    def test_expected_commit_must_be_current_main(self):
        gate.validate(HEAD, analyses(), [], HEAD)
        for expected in ("b" * 40, "bad"):
            with self.subTest(expected=expected), self.assertRaises(gate.GateError):
                gate.validate(HEAD, analyses(), [], expected)

    def test_api_failure_and_bad_pagination_fail_closed(self):
        with mock.patch.object(gate.subprocess, "run", side_effect=subprocess.CalledProcessError(1, "gh")):
            with self.assertRaises(gate.GateError):
                gate.gh_json("test")
        with mock.patch.object(gate.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, stdout='{"not":"pages"}')):
            with self.assertRaises(gate.GateError):
                gate.gh_json("test", paginate=True)

    def test_paginated_results_include_all_pages(self):
        pages = [subprocess.CompletedProcess([], 0, stdout="[" + ",".join(["{}"] * 100) + "]"),
                 subprocess.CompletedProcess([], 0, stdout='[{"number":42}]')]
        with mock.patch.object(gate.subprocess, "run", side_effect=pages):
            result = gate.gh_json("test?per_page=100", paginate=True)
        self.assertEqual(len(result), 101)
        self.assertEqual(result[-1]["number"], 42)

    def test_main_change_during_check_blocks(self):
        replies = [{"sha": HEAD}, analyses(), [], {"sha": "b" * 40}]
        with mock.patch.object(gate, "gh_json", side_effect=replies), mock.patch("sys.argv", ["check"]), contextlib.redirect_stderr(io.StringIO()):
            self.assertEqual(gate.main(), 1)


if __name__ == "__main__":
    unittest.main()
