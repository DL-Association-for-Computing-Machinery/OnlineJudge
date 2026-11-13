import importlib.util
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("measure_pr_ci", ROOT / "scripts/measure-pr-ci.py")
metrics = importlib.util.module_from_spec(spec)
spec.loader.exec_module(metrics)


class DeliveryMetricsTest(unittest.TestCase):
    def test_counts_retries_and_selects_latest_run_for_final_head(self):
        pr = {"number": 8, "head": {"ref": "feature", "repo": {"id": 1}, "sha": "final"},
              "created_at": "2026-01-01", "closed_at": "2026-02-01"}
        base = {"pull_requests": [], "head_repository": {"id": 1}, "head_branch": "feature",
                "created_at": "2026-01-02", "run_started_at": "2026-01-02",
                "head_sha": "final", "run_attempt": 1, "status": "completed"}
        runs = [
            dict(base, id=1, head_sha="old", conclusion="cancelled"),
            dict(base, id=2, conclusion="success", run_attempt=2),
            dict(base, id=3, status="in_progress", conclusion=None),
            dict(base, id=4, conclusion="success", head_repository={"id": 2}),
            dict(base, id=5, conclusion="success", pull_requests=[{"number": 9}]),
            dict(base, id=6, conclusion="success", created_at="2026-02-02"),
        ]
        summary = metrics.summarize(runs, pr)
        self.assertEqual(summary["latest_outcomes"], {"cancelled": 1, "success": 1, "in_progress": 1})
        self.assertEqual(summary["manual_retries"], 1)
        self.assertEqual(summary["inferred_associations"], 3)
        self.assertEqual(summary["latest_final_head_run"]["id"], 3)


class SessionSuitesTest(unittest.TestCase):
    def test_waits_for_both_suites_and_propagates_either_failure(self):
        with tempfile.TemporaryDirectory() as directory:
            scripts = Path(directory) / "scripts"
            scripts.mkdir()
            runner = scripts / "test-ci-session-suites.sh"
            runner.write_bytes((ROOT / "scripts/test-ci-session-suites.sh").read_bytes())
            (scripts / "test-session-protocol.py").write_text(
                "import sys\n"
                "from pathlib import Path\n"
                "from time import monotonic, sleep\n"
                "failing, _, suite = sys.argv[1:]\n"
                "Path(suite).touch()\n"
                "other = 'email-lifecycle' if suite == 'ordinary' else 'ordinary'\n"
                "deadline = monotonic() + 5\n"
                "while not Path(other).exists() and monotonic() < deadline:\n"
                "    sleep(0.01)\n"
                "assert Path(other).exists(), 'suites did not overlap'\n"
                "print(f'completed {suite}')\n"
                "sys.exit(3 if failing == suite else 0)\n",
                encoding="utf-8",
            )
            for failing in ("none", "ordinary", "email-lifecycle"):
                with self.subTest(failing=failing):
                    for suite in ("ordinary", "email-lifecycle"):
                        (Path(directory) / suite).unlink(missing_ok=True)
                    run = subprocess.run(["bash", str(runner), failing], text=True,
                                         capture_output=True, timeout=10)
                    self.assertEqual(run.returncode, int(failing != "none"), run.stdout + run.stderr)
                    self.assertIn("completed ordinary", run.stdout)
                    self.assertIn("completed email-lifecycle", run.stdout)
