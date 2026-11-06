#!/usr/bin/env python3
"""Fail closed unless main has current CodeQL scans and no high security alerts."""

import argparse
import json
import os
import re
import subprocess
import sys


REQUIRED_CATEGORIES = {
    ".github/workflows/security.yml:codeql/language:go",
    ".github/workflows/security.yml:codeql/language:python",
}
SHA = re.compile(r"^[0-9a-f]{40}$")
REPO = re.compile(r"^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$")


class GateError(Exception):
    pass


def gh_json(path, paginate=False):
    items = []
    for page in range(1, 1001):
        endpoint = path + ("&page=" + str(page) if paginate else "")
        try:
            result = subprocess.run(["gh", "api", endpoint], capture_output=True, text=True, timeout=120, check=True)
            value = json.loads(result.stdout)
        except (OSError, subprocess.SubprocessError, ValueError) as error:
            # Do not print command output: it may contain sensitive API details.
            raise GateError("GitHub API query failed or returned invalid JSON") from error
        if not paginate:
            return value
        if not isinstance(value, list):
            raise GateError("GitHub API returned an invalid paginated response")
        items.extend(value)
        if len(value) < 100:
            return items
    raise GateError("GitHub API returned too many pages")


def validate(head, analyses, alerts, expected=None):
    if not isinstance(head, str) or not SHA.fullmatch(head):
        raise GateError("GitHub returned an invalid main commit")
    if expected is not None and (not SHA.fullmatch(expected) or expected != head):
        raise GateError("release commit does not match the current main commit")
    latest = {}
    for analysis in analyses:
        if not isinstance(analysis, dict):
            raise GateError("GitHub returned malformed analysis metadata")
        category = analysis.get("category")
        if category in REQUIRED_CATEGORIES and category not in latest:
            latest[category] = analysis
    for category in sorted(REQUIRED_CATEGORIES):
        analysis = latest.get(category)
        if not analysis or analysis.get("commit_sha") != head or (analysis.get("tool") or {}).get("name") != "CodeQL":
            raise GateError("main is missing a current CodeQL analysis for " + category)
    blocking = []
    for alert in alerts:
        if not isinstance(alert, dict) or not isinstance(alert.get("rule"), dict):
            raise GateError("GitHub returned malformed alert metadata")
        rule = alert["rule"]
        severity = rule.get("security_severity_level")
        tags = rule.get("tags")
        if not isinstance(tags, list):
            raise GateError("GitHub returned malformed alert tags")
        if severity not in (None, "low", "medium", "high", "critical"):
            raise GateError("GitHub returned an unknown security severity")
        if severity is None and "security" in tags:
            raise GateError("an open security alert has no security severity")
        if severity in ("high", "critical"):
            blocking.append(str(alert.get("number", "unknown")))
    if blocking:
        raise GateError("open high or critical code-scanning alerts: " + ", ".join(blocking))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", default=os.environ.get("GITHUB_REPOSITORY", "geoffmcc/nodex"))
    parser.add_argument("--expected-commit", help="require main to match this full commit SHA")
    args = parser.parse_args()
    try:
        if not REPO.fullmatch(args.repo):
            raise GateError("invalid repository name")
        prefix = "repos/" + args.repo
        head = gh_json(prefix + "/commits/main").get("sha")
        ref = "ref=refs%2Fheads%2Fmain&per_page=100"
        analyses = gh_json(prefix + "/code-scanning/analyses?" + ref, paginate=True)
        alerts = gh_json(prefix + "/code-scanning/alerts?state=open&" + ref, paginate=True)
        validate(head, analyses, alerts, args.expected_commit)
        if gh_json(prefix + "/commits/main").get("sha") != head:
            raise GateError("main changed during the security check; retry")
    except (GateError, AttributeError) as error:
        print("Release security gate failed: " + str(error), file=sys.stderr)
        return 1
    print("Release security gate passed for main at " + head)
    return 0


if __name__ == "__main__":
    sys.exit(main())
