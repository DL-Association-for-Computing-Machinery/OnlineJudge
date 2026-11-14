import copy
from datetime import datetime, timedelta, timezone
import hashlib
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
                   submissions=[dict(p, id=f"{i}-{p['id']}") for p in posts],
                   observed_submissions=[{"key": p["key"], "id": f"{i}-{p['id']}"} for p in posts],
                   peaks=dict(cpu_millicores=1000, memory_bytes=4096, disk_bytes=4096, connections=8))
              for i in range(3)]
    return dict(schema="oj.p6-mixed-load.v1", origin_scope="local", release_sha256="a" * 64,
                topology_sha256="b" * 64, versions=versions, rounds=rounds)


def utc(ms):
    return (datetime(1970, 1, 1, tzinfo=timezone.utc) + timedelta(milliseconds=ms)).isoformat(
        timespec="milliseconds").replace("+00:00", "Z")


def acceptance(round_):
    start = round_["run_started_utc_ms"]
    end = utc(start + 900_000)
    return {"schema": "oj.p6-training-acceptance.v1", "from": utc(start), "until": end,
            "observed_at": end, "accepted": [
                {"id": p["id"], "accepted_at": utc(start + p["start_ms"]), "material_version": p["version"]}
                for p in round_["submissions"] if p["id"] is not None]}


class MixedLoadTests(unittest.TestCase):
    def assess(self, doc, observations=()):
        with tempfile.TemporaryDirectory() as path:
            source = Path(path) / "observations.json"
            source.write_text(json.dumps(doc))
            args = []
            for i, raw in enumerate(observations):
                target = Path(path) / f"raw-{i}.json"
                target.write_text(json.dumps(raw))
                args.append(str(target))
            return subprocess.run([sys.executable, "-B", str(SCRIPT), str(source), *(["--acceptance", *args] if args else [])],
                                  capture_output=True, text=True, timeout=10)

    def test_exact_thresholds_pass_without_claiming_trial_acceptance(self):
        doc = record()
        for sample in doc["rounds"][0]["reads"][:90]:
            sample["ok"] = False
        result = self.assess(doc)
        self.assertEqual(result.returncode, 0, result.stderr)
        report = json.loads(result.stdout)
        self.assertEqual(report["origin_scope"], "local")
        self.assertIsNone(report["rounds"][0]["independent_acceptance"])
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

    def test_trial_requires_and_binds_three_raw_exports(self):
        doc = record()
        doc["origin_scope"] = "trial"
        self.assertEqual(self.assess(doc).returncode, 2)
        exports = [acceptance(round_) for round_ in doc["rounds"]]
        result = self.assess(doc, exports)
        self.assertEqual(result.returncode, 0, result.stderr)
        first = json.loads(result.stdout)["rounds"][0]["independent_acceptance"]
        self.assertEqual(first, dict(sha256=hashlib.sha256(json.dumps(exports[0]).encode()).hexdigest(),
                                     accepted=90, unreconciled_database_receipts=0,
                                     client_receipts_missing_from_database=0, material_mismatches=0))
        result = self.assess(doc, exports[::-1])
        self.assertEqual(result.returncode, 2)
        self.assertEqual(result.stdout, "")

    def test_extra_missing_and_wrong_material_receipts_fail_reconciliation(self):
        doc = record()
        exports = [acceptance(round_) for round_ in doc["rounds"]]
        exports[0]["accepted"].append(dict(exports[0]["accepted"][0], id="committed-without-client-reply"))
        exports[1]["accepted"].pop()
        exports[2]["accepted"][0]["material_version"] = "unregistered-version"
        result = self.assess(doc, exports)
        self.assertEqual(result.returncode, 1, result.stderr)
        for index, field in enumerate(("unreconciled_database_receipts", "client_receipts_missing_from_database", "material_mismatches")):
            row = json.loads(result.stdout)["rounds"][index]
            self.assertEqual(row["failures"], ["independent_acceptance_reconciliation"])
            self.assertEqual(row["independent_acceptance"][field], 1)
        self.assertNotIn("committed-without-client-reply", result.stdout)

    def test_exact_window_and_duplicate_receipt_are_checked(self):
        doc = record()
        for fault in ("outside", "duplicate", "cross-round", "premature"):
            with self.subTest(fault=fault):
                exports = [acceptance(round_) for round_ in doc["rounds"]]
                first = exports[0]
                if fault == "outside":
                    first["accepted"][0]["accepted_at"] = first["until"]
                elif fault == "duplicate":
                    first["accepted"].append(first["accepted"][0])
                elif fault == "cross-round":
                    exports[1]["accepted"][0]["id"] = first["accepted"][0]["id"]
                else:
                    first["observed_at"] = "1970-01-01T00:15:00.000999999Z"
                result = self.assess(doc, exports)
                self.assertEqual(result.returncode, 2)
                self.assertEqual(result.stdout, "")
