"""Test CI routing with real Git diffs and fail-closed required check results."""

import importlib.util
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import unittest


SOURCE = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("ci_scope", SOURCE / "scripts/ci_scope.py")
scope = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(scope)


class ScopeTests(unittest.TestCase):
    def test_workflow_keeps_required_gates_and_conditions_wired(self):
        workflow = (SOURCE / ".github/workflows/governance.yml").read_text()
        # Keep this dependency-free; actionlint separately validates YAML/expressions.
        jobs = dict(re.findall(r"^  ([a-z-]+):\n(.*?)(?=^  [a-z-]+:|\Z)",
                               workflow.split("jobs:\n", 1)[1], re.M | re.S))
        for suite in scope.SUITES:
            self.assertIn("needs: governance", jobs[suite + "-tests"])
            self.assertIn(f"if: needs.governance.outputs.{suite} == 'true'", jobs[suite + "-tests"])
            self.assertIn(f"{suite}: ${{{{ steps.scope.outputs.{suite} }}}}", jobs["governance"])
        self.assertIn("fetch-depth: 0", jobs["governance"])
        self.assertIn("python3 scripts/ci_scope.py plan", jobs["governance"])
        self.assertIn("if: always()", jobs["required"])
        self.assertIn("needs: [governance, backend-tests, frontend-tests, integration-tests, containers, security]", jobs["required"])
        self.assertIn("fetch-depth: 0", jobs["security"])
        self.assertNotIn("if:", jobs["security"])
        self.assertIn("needs: governance", jobs["containers"])
        self.assertIn("if: needs.governance.outputs.backend == 'true' || needs.governance.outputs.frontend == 'true'", jobs["containers"])
        self.assertIn("suite: [backend, frontend, integration]", jobs["required"])
        self.assertIn("name: ${{ matrix.suite }}", jobs["required"])
        self.assertIn("CI_NEEDS: ${{ toJSON(needs) }}", jobs["required"])
        self.assertIn("python3 scripts/ci_scope.py gate", jobs["required"])
        self.assertNotRegex(workflow, r"(?m)^\s+(paths|paths-ignore|continue-on-error):")

    def test_documentation_and_rules_only(self):
        plan = scope.classify(["README.md", "AGENTS.md", "docs/ci-cd.md",
                               ".github/pull_request_template.md", "web/README.md"])
        self.assertEqual(plan, dict.fromkeys(scope.SUITES, False))

    def test_frontend_and_backend_boundaries(self):
        for path in ("web/src/App.tsx", "web/pnpm-lock.yaml", "web/.node-version"):
            self.assertEqual(scope.classify([path]),
                             {"backend": False, "frontend": True, "integration": False})
        for path in ("cmd/api/main.go", "internal/worker/run.go", "go.sum",
                     "db/migrations/001.sql", "sqlc.yaml", "tests/integration/a_test.go",
                     "tests/sandbox/fixture.cpp", "internal/worker/fixture.md", "scripts/test-postgres.sh"):
            self.assertEqual(scope.classify([path]),
                             {"backend": True, "frontend": False, "integration": True})

    def test_governance_only_and_shared_or_unknown_paths(self):
        self.assertFalse(any(scope.classify(["scripts/repo_policy.py",
                                            "tests/governance/test_policy.py",
                                            ".githooks/pre-commit"]).values()))
        for path in ("Dockerfile", "compose.yaml", "scripts/toolchain.sh", ".github/workflows/governance.yml", "scripts/ci_scope.py",
                     "tests/governance/test_ci_scope.py", ".github/actions/setup/action.yml",
                     ".github/repository/main-ruleset.json", "Dockerfile", "unknown.txt",
                     "docs/run.sh", "docs/fixture.sql"):
            with self.subTest(path=path):
                self.assertTrue(all(scope.classify([path]).values()))

    def test_mixed_and_empty_changes(self):
        self.assertTrue(all(scope.classify(["web/src/App.tsx", "go.mod"]).values()))
        self.assertTrue(all(scope.classify([]).values()))

    def test_required_gate_rejects_missing_failed_cancelled_or_unexpected_skip(self):
        for suite in scope.SUITES:
            for selected in ("true", "false", "", "yes"):
                for result in ("success", "failure", "cancelled", "skipped", "", "neutral"):
                    outputs = dict.fromkeys(scope.SUITES, "false")
                    outputs[suite] = selected
                    containers = "success" if any(outputs[name] == "true" for name in ("backend", "frontend")) else "skipped"
                    needs = {"governance": {"result": "success", "outputs": outputs},
                             "security": {"result": "success"},
                             "containers": {"result": containers}, suite + "-tests": {"result": result}}
                    expected = (selected, result) in (("true", "success"), ("false", "skipped"))
                    with self.subTest(suite=suite, selected=selected, result=result):
                        self.assertEqual(scope.gate_passes(suite, needs), expected)
            for result in ("failure", "cancelled", "skipped", ""):
                needs = {"governance": {"result": result, "outputs": {suite: "false"}},
                         suite + "-tests": {"result": "skipped"}}
                self.assertFalse(scope.gate_passes(suite, needs))
        self.assertFalse(scope.gate_passes("backend", {}))

    def test_gate_command_exits_nonzero_on_failure(self):
        for selected, result, expected in (("true", "success", 0), ("false", "skipped", 0),
                                           ("true", "failure", 1), ("true", "skipped", 1)):
            needs = {"governance": {"result": "success", "outputs": {
                         "backend": selected, "frontend": "false", "integration": "false"}},
                     "containers": {"result": "success" if selected == "true" else "skipped"},
                     "security": {"result": "success"},
                     "backend-tests": {"result": result}}
            env = dict(os.environ, CI_SUITE="backend", CI_NEEDS=json.dumps(needs))
            env.pop("GITHUB_STEP_SUMMARY", None)
            process = subprocess.run([sys.executable, "-B", str(SOURCE / "scripts/ci_scope.py"), "gate"],
                                     env=env, capture_output=True)
            self.assertEqual(process.returncode, expected)

    def test_container_failure_blocks_every_required_context(self):
        for backend, frontend in (("true", "false"), ("false", "true"), ("true", "true"), ("false", "false")):
            outputs = {"backend": backend, "frontend": frontend, "integration": backend}
            expected = "success" if "true" in (backend, frontend) else "skipped"
            needs = {"governance": {"result": "success", "outputs": outputs},
                     "security": {"result": "success"}}
            needs.update({name + "-tests": {"result": "success" if outputs[name] == "true" else "skipped"}
                          for name in scope.SUITES})
            for result in ("success", "skipped", "failure", "cancelled", "neutral", ""):
                needs["containers"] = {"result": result}
                for suite in scope.SUITES:
                    self.assertEqual(scope.gate_passes(suite, needs), result == expected)
            del needs["containers"]
            self.assertFalse(any(scope.gate_passes(suite, needs) for suite in scope.SUITES))
        needs["containers"] = {"result": "skipped"}
        del outputs["frontend"]
        self.assertFalse(scope.gate_passes("backend", needs))

    def test_security_must_succeed_even_for_documentation(self):
        needs = {"governance": {"result": "success", "outputs": dict.fromkeys(scope.SUITES, "false")},
                 "containers": {"result": "skipped"}}
        needs.update({suite + "-tests": {"result": "skipped"} for suite in scope.SUITES})
        for result in ("success", "failure", "cancelled", "skipped", "neutral", ""):
            needs["security"] = {"result": result}
            for suite in scope.SUITES:
                self.assertEqual(scope.gate_passes(suite, needs), result == "success")
        del needs["security"]
        self.assertFalse(scope.gate_passes("backend", needs))


class DiffTests(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory(prefix="oj-ci-scope-")
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name)
        self.env = dict(os.environ, GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull)
        self.env.pop("GITHUB_STEP_SUMMARY", None)
        for key in ("GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR"):
            self.env.pop(key, None)
        self.git("init", "--template=", "-b", "main")
        self.git("config", "user.name", "CI Test")
        self.git("config", "user.email", "ci@example.invalid")
        self.write("README.md", "seed")
        self.base = self.commit()

    def git(self, *args):
        return subprocess.check_output(["git", *args], cwd=self.root, env=self.env).decode().strip()

    def write(self, name, content):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content)

    def commit(self):
        self.git("add", ".")
        self.git("commit", "-qm", "test: fixture")
        return self.git("rev-parse", "HEAD")

    def plan(self, event, kind="pull_request"):
        event_file = self.root / ".git/event.json"
        output_file = self.root / ".git/output"
        event_file.write_text(json.dumps(event))
        output_file.write_text("")
        result = subprocess.run([sys.executable, "-B", str(SOURCE / "scripts/ci_scope.py"), "plan"],
                                cwd=self.root, env=dict(self.env, GITHUB_EVENT_NAME=kind,
                                                       GITHUB_EVENT_PATH=str(event_file),
                                                       GITHUB_OUTPUT=str(output_file)),
                                capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        return dict(line.split("=", 1) for line in output_file.read_text().splitlines())

    def pr(self, base, head):
        return self.plan({"pull_request": {"base": {"sha": base}, "head": {"sha": head}}})

    def test_whole_pr_includes_earlier_code_commit_after_docs_commit(self):
        self.write("web/src/App.tsx", "code")
        self.commit()
        self.write("docs/note.md", "docs")
        result = self.pr(self.base, self.commit())
        self.assertEqual(result, {"backend": "false", "frontend": "true", "integration": "false"})

    def test_base_only_changes_do_not_expand_pr_scope(self):
        self.git("checkout", "-qb", "topic")
        self.write("docs/note.md", "docs")
        head = self.commit()
        self.git("checkout", "-q", "main")
        self.write("web/src/App.tsx", "base only")
        result = self.pr(self.commit(), head)
        self.assertEqual(set(result.values()), {"false"})

    def test_deletion_and_rename_out_of_code_with_unusual_names(self):
        self.write("web/src/space\nname.ts", "same")
        base = self.commit()
        (self.root / "docs").mkdir()
        self.git("mv", "web/src/space\nname.ts", "docs/moved.md")
        self.assertEqual(self.pr(base, self.commit())["frontend"], "true")
        self.write("internal/old.go", "code")
        base = self.commit()
        (self.root / "internal/old.go").unlink()
        self.assertEqual(self.pr(base, self.commit())["backend"], "true")

    def test_push_uses_entire_push_range(self):
        self.write("web/src/App.tsx", "code")
        self.commit()
        self.write("docs/note.md", "docs")
        self.assertEqual(self.plan({"before": self.base, "after": self.commit()}, "push")["frontend"], "true")

    def test_unavailable_history_manual_unknown_and_empty_are_full(self):
        for event, kind in (({}, "workflow_dispatch"), ({}, "unknown"),
                            ({"before": "0" * 40, "after": self.base}, "push"),
                            ({"before": "f" * 40, "after": self.base}, "push"),
                            ({"before": "--help", "after": self.base}, "push")):
            self.assertEqual(set(self.plan(event, kind).values()), {"true"})
        self.assertEqual(set(self.pr(self.base, self.base).values()), {"true"})


if __name__ == "__main__":
    unittest.main()
