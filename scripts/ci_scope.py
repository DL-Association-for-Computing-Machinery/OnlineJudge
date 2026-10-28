#!/usr/bin/env python3
"""Route existing CI suites from complete Git diffs; uncertainty runs all suites."""

import json
import os
from pathlib import Path
import re
import subprocess
import sys


SUITES = ("backend", "frontend", "integration")
SHA = re.compile(r"[0-9a-f]{40}")


def classify(paths):
    """Only known independent paths may omit a suite; unknown paths run all."""
    plan = dict.fromkeys(SUITES, False)
    if not paths:
        return dict.fromkeys(SUITES, True)
    for path in paths:
        # Routing and its own tests can affect every gate, unlike ordinary docs.
        if path in {"scripts/ci_scope.py", "tests/governance/test_ci_scope.py"}:
            return dict.fromkeys(SUITES, True)
        if path in {"README.md", "AGENTS.md", "LICENSE", ".github/pull_request_template.md"} or (
            path.startswith("docs/") and path.endswith(".md")
        ) or path == "web/README.md":
            continue
        if path.startswith((".githooks/", "tests/governance/")) or path in {
            "scripts/repo_policy.py", "scripts/install-hooks.sh",
        }:
            continue
        if path.startswith("web/"):
            plan["frontend"] = True
        elif path.startswith(("cmd/", "internal/", "db/", "tests/integration/", "tests/sandbox/")) or path in {
            "go.mod", "go.sum", "sqlc.yaml", "scripts/test-postgres.sh",
        }:
            plan["backend"] = plan["integration"] = True
        else:
            return dict.fromkeys(SUITES, True)
    return plan


def changed_paths(kind, event):
    """Use PR merge-base or complete push range, including both sides of renames."""
    if kind == "pull_request":
        base = event["pull_request"]["base"]["sha"]
        head = event["pull_request"]["head"]["sha"]
        separator = "..."
    elif kind == "push":
        base, head = event["before"], event["after"]
        separator = ".."
    else:
        raise ValueError("Full validation event")
    if not all(isinstance(value, str) and SHA.fullmatch(value) and value != "0" * 40
               for value in (base, head)):
        raise ValueError("Missing immutable diff endpoints")
    # --no-renames preserves the deleted source path even when code moves to docs.
    result = subprocess.run(
        ["git", "diff", "--name-only", "--no-renames", "-z", base + separator + head, "--"],
        check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
    )
    return [os.fsdecode(path) for path in result.stdout.split(b"\0") if path]


def gate_passes(suite, needs):
    """A skip is valid only with a successful classifier explicitly selecting false."""
    governance = needs.get("governance", {})
    if suite not in SUITES or governance.get("result") != "success":
        return False
    selected = governance.get("outputs", {}).get(suite)
    result = needs.get(suite + "-tests", {}).get("result")
    return (selected, result) in (("true", "success"), ("false", "skipped"))


def plan_event():
    try:
        event = json.loads(Path(os.environ["GITHUB_EVENT_PATH"]).read_text())
        paths = changed_paths(os.environ["GITHUB_EVENT_NAME"], event)
        plan = classify(paths)
        reason = f"Complete Git diff: {len(paths)} paths" if paths else "Empty diff: full validation"
    except (KeyError, ValueError, TypeError, OSError, subprocess.CalledProcessError):
        plan = dict.fromkeys(SUITES, True)
        reason = "Manual/unknown event or unavailable diff: full validation"
    lines = [f"{suite}={str(plan[suite]).lower()}" for suite in SUITES]
    with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as output:
        output.write("\n".join(lines) + "\n")
    summary = "### CI scope\n\n" + reason + "\n\n" + "\n".join(
        f"- {suite}: {'run' if plan[suite] else 'not affected; tests omitted'}" for suite in SUITES
    ) + "\n"
    print(summary)
    if os.environ.get("GITHUB_STEP_SUMMARY"):
        with open(os.environ["GITHUB_STEP_SUMMARY"], "a", encoding="utf-8") as output:
            output.write(summary)


def main():
    if sys.argv[1:] == ["plan"]:
        plan_event()
    elif sys.argv[1:] == ["gate"]:
        suite = os.environ["CI_SUITE"]
        needs = json.loads(os.environ["CI_NEEDS"])
        if not gate_passes(suite, needs):
            raise ValueError("Classifier or required suite did not complete as planned")
        selected = needs["governance"]["outputs"][suite] == "true"
        message = f"{suite}: {'tests passed' if selected else 'not affected; tests omitted'}"
        print(message)
        if os.environ.get("GITHUB_STEP_SUMMARY"):
            with open(os.environ["GITHUB_STEP_SUMMARY"], "a", encoding="utf-8") as output:
                output.write(message + "\n")
    else:
        raise ValueError("Expected plan or gate")


if __name__ == "__main__":
    try:
        main()
    except (KeyError, ValueError, TypeError, OSError) as error:
        print(f"CI scope failed: {error}", file=sys.stderr)
        sys.exit(1)
