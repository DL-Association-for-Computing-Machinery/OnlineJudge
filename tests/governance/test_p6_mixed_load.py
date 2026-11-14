import copy
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


SCRIPT = Path(__file__).resolve().parents[2] / "scripts/assess-p6-mixed-load.py"


def record():
    kinds = ("statement", "result", "history")
    versions = {"a": "small", "b": "medium", "c": "heaviest"}
    reads = [{"subject": f"user-{i}", "kind": kinds[i % 3], "start_ms": slot * 10_000 + i,
              "elapsed_ms": 1000, "ok": True} for slot in range(90) for i in range(100)]
    posts = [{"key": f"key-{i}", "subject": f"user-{i}", "version": "abc"[i % 3],
              "start_ms": i // 30 * 300_000 + i % 30 * 1000, "confirmation_ms": 1000,
              "id": f"id-{i}", "queue_ms": 20, "terminal_ms": 60_000, "verdict": "WA"}
             for i in range(90)]
    rounds = [dict(id=f"round-{i}", run_started_utc_ms=1 + i * 900_000, reads=copy.deepcopy(reads),
                   submissions=copy.deepcopy(posts), observed_submissions=[{"key": p["key"], "id": p["id"]} for p in posts],
                   peaks=dict(cpu_millicores=1000, memory_bytes=4096, disk_bytes=4096, connections=8))
              for i in range(3)]
    return dict(schema="oj.p6-mixed-load.v1", origin_scope="local", release_sha256="a" * 64,
                topology_sha256="b" * 64, versions=versions, rounds=rounds)


class MixedLoadTests(unittest.TestCase):
    def assess(self, doc):
        with tempfile.TemporaryDirectory() as path:
            source = Path(path) / "observations.json"
            source.write_text(json.dumps(doc))
            return subprocess.run([sys.executable, "-B", str(SCRIPT), str(source)],
                                  capture_output=True, text=True, timeout=10)

    def test_exact_thresholds_pass_without_claiming_trial_acceptance(self):
        doc = record()
        for sample in doc["rounds"][0]["reads"][:90]:
            sample["ok"] = False
        result = self.assess(doc)
        self.assertEqual(result.returncode, 0, result.stderr)
        report = json.loads(result.stdout)
        self.assertEqual(report["origin_scope"], "local")
        self.assertEqual(report["rounds"][0]["measures"]["read"],
                         dict(attempts=9000, samples=8910, p50_ms=1000, p95_ms=1000, p99_ms=1000,
                              errors=90, error_rate=.01))

    def test_one_failed_round_cannot_be_averaged_away(self):
        doc = record()
        row = doc["rounds"][1]
        row["reads"].pop()
        for sample in row["reads"][:91]:
            sample["ok"] = False
        row["submissions"][0].update(terminal_ms=180_001, verdict="SE")
        row["observed_submissions"][1]["id"] = "wrong-id"
        result = self.assess(doc)
        self.assertEqual(result.returncode, 1, result.stderr)
        failed = json.loads(result.stdout)["rounds"][1]["failures"]
        self.assertEqual(set(failed), {"read_error_rate", "read_schedule", "system_error",
                                       "missing_or_late_terminal", "submission_reconciliation"})

    def test_duplicate_schedule_or_overlapping_rounds_are_invalid(self):
        for fault in ("duplicate", "overlap"):
            with self.subTest(fault=fault):
                doc = record()
                if fault == "duplicate":
                    doc["rounds"][0]["reads"][1] = doc["rounds"][0]["reads"][0]
                else:
                    doc["rounds"][1]["run_started_utc_ms"] -= 1
                result = self.assess(doc)
                self.assertEqual(result.returncode, 2, result.stderr)
                self.assertEqual(result.stdout, "")

    def test_confirmation_and_terminal_thresholds_are_independent(self):
        doc = record()
        first, second, third = doc["rounds"]
        for item in first["submissions"][:5]:
            item["confirmation_ms"] = 1001
        for item in second["submissions"][:5]:
            item["terminal_ms"] = 60_001
        item = third["submissions"][0]
        item.update(id=None, queue_ms=None, terminal_ms=None, verdict=None)
        third["observed_submissions"].pop(0)
        result = self.assess(doc)
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertEqual([row["failures"] for row in json.loads(result.stdout)["rounds"]],
                         [["confirmation_p95"], ["terminal_p95"], ["confirmation_error_rate"]])
