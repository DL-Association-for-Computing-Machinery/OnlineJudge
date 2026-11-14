from datetime import datetime, timedelta, timezone
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


SCRIPT = Path(__file__).resolve().parents[2] / "scripts/assess-p6-overload.py"


def records():
    def request(key, sent, done, status, ident=None, code=None):
        return dict(key=key, request_sha256="a" * 64, started_ms=sent, finished_ms=done,
                    status=status, code=code, id=ident)

    def accepted(key, ident, at, terminal, verdict):
        return dict(key=key, id=ident, accepted_ms=at, queued_ms=10, terminal_ms=terminal,
                    read_ms=terminal + 5, verdict=verdict)

    identity = dict(release_sha256="b" * 64, topology_sha256="c" * 64, config_sha256="d" * 64)
    client = dict(schema="oj.p6-overload.v1", origin_scope="local", global_limit=2, rounds=[], **identity)
    server = dict(schema="oj.p6-overload-ledger.v1", rounds=[], **identity)
    for i in range(3):
        rid = f"round-{i}"
        client["rounds"].append(dict(id=rid, started_utc_ms=1 + i * 320_000, stopped_ms=100, ended_ms=320_000,
            attempts=[request("a", 1, 3, 202, "id-a"), request("b", 4, 6, 202, "id-b"),
                      request("c", 20, 25, 429, code="BACKLOG_LIMIT"), request("a", 30, 31, 200, "id-a"),
                      request("e", 40, 45, 429, code="BACKLOG_LIMIT"),
                      request("c", 201, 210, 202, "id-c"), request("d", 300_000, 300_100, 202, "id-d")]))
        server["rounds"].append(dict(id=rid, saturated_ms=10, unfinished=2, drain_ms=200, oom_kills=0,
            abnormal_exits=0, peaks=dict(cpu_millicores=1000, memory_bytes=100, disk_bytes=100, connections=4),
            ledger=[accepted("a", "id-a", 2, 150, "AC"), accepted("b", "id-b", 5, 200, "WA"),
                    accepted("c", "id-c", 203, 250, "AC"), accepted("d", "id-d", 300_001, 300_150, "AC")]))
        for row in client["rounds"][-1]["attempts"] + server["rounds"][-1]["ledger"]:
            if row["id"] is not None:
                row["id"] = f"{i}-{row['id']}"
    return client, server


def utc(ms):
    return (datetime(1970, 1, 1, tzinfo=timezone.utc) + timedelta(milliseconds=ms)).isoformat(
        timespec="milliseconds").replace("+00:00", "Z")


def observations(client, server):
    result = []
    for a, b in zip(client["rounds"], server["rounds"]):
        start = a["started_utc_ms"]
        until = utc(start + a["ended_ms"])
        result.append({"schema": "oj.p6-training-acceptance.v1", "from": utc(start), "until": until,
                       "observed_at": until, "accepted": [
                           dict(id=r["id"], accepted_at=utc(start + r["accepted_ms"]), material_version="fixture-v1")
                           for r in b["ledger"]]})
    return result


class OverloadTests(unittest.TestCase):
    def assess(self, client, server, bind=True, exports=()):
        with tempfile.TemporaryDirectory() as path:
            ledger = Path(path) / "server.json"
            raw = json.dumps(server).encode()
            ledger.write_bytes(raw)
            client["ledger_sha256"] = hashlib.sha256(raw).hexdigest() if bind else "f" * 64
            record = Path(path) / "client.json"
            record.write_text(json.dumps(client))
            files = []
            for i, item in enumerate(exports):
                path_ = Path(path) / f"raw-{i}.json"
                path_.write_text(json.dumps(item))
                files.append(str(path_))
            return subprocess.run([sys.executable, "-B", str(SCRIPT), str(record), str(ledger),
                                   *(["--acceptance", *files] if files else [])],
                                  capture_output=True, text=True, timeout=10)

    def test_three_rounds_pass_at_exact_recovery_deadline(self):
        result = self.assess(*records())
        self.assertEqual(result.returncode, 0, result.stderr)
        report = json.loads(result.stdout)
        self.assertEqual(report["scope"], "recorded_overload_only")
        self.assertEqual(report["origin_scope"], "local")
        for round_ in report["rounds"]:
            self.assertEqual(round_["recovery_ms"], 300_000)
            self.assertEqual(round_["drain_after_stop_ms"], 100)
            self.assertEqual(round_["accepted"], 4)
            self.assertIsNone(round_["independent_acceptance"])
            self.assertEqual(round_["timings"]["rejected"]["p95_ms"], 5)

    def test_early_write_and_hidden_acceptance_cannot_pass_reconciliation(self):
        client, server = records()
        ledger = server["rounds"][1]["ledger"]
        ledger[2]["accepted_ms"] = 23  # Before 429 completes, not during the subsequent 202.
        ledger.append(dict(ledger[2], key="e", id="id-e", accepted_ms=44))
        result = self.assess(client, server)
        self.assertEqual(result.returncode, 1, result.stderr)
        failures = json.loads(result.stdout)["rounds"][1]["failures"]
        self.assertIn("rejection_wrote_or_rejected_replay", failures)
        self.assertIn("submission_reconciliation", failures)

    def test_each_round_must_show_fresh_recovery_and_health(self):
        client, server = records()
        client["rounds"][0]["attempts"][-1]["finished_ms"] += 1
        server["rounds"][1].update(unfinished=1, oom_kills=1, drain_ms=199)
        server["rounds"][2]["ledger"][1]["read_ms"] = None
        client["rounds"][2]["attempts"][3]["id"] = "wrong-replay-id"
        result = self.assess(client, server)
        self.assertEqual(result.returncode, 1, result.stderr)
        reports = json.loads(result.stdout)["rounds"]
        self.assertEqual([item["failures"] for item in reports], [
            ["late_or_missing_recovery"], ["drain_unproven", "global_queue_not_full", "process_health"],
            ["missing_replay_during_pressure", "missing_terminal_readback", "replay_changed_receipt"],
        ])

    def test_ambiguous_or_unbound_evidence_is_refused(self):
        for fault in ("digest", "overlap", "body", "config"):
            with self.subTest(fault=fault):
                client, server = records()
                if fault == "overlap":
                    client["rounds"][2]["started_utc_ms"] -= 1
                if fault == "body":
                    client["rounds"][0]["attempts"][3]["request_sha256"] = "f" * 64
                if fault == "config":
                    server["config_sha256"] = "f" * 64
                result = self.assess(client, server, bind=fault != "digest")
                self.assertEqual(result.returncode, 2, result.stderr)
                self.assertEqual(result.stdout, "")

    def test_full_queue_observation_alone_does_not_prove_overload(self):
        client, server = records()
        # The queue was full at observation, but one job finished before
        # either rejected request or the replay reached the service.
        server["rounds"][0]["ledger"][0].update(terminal_ms=15, read_ms=16)
        result = self.assess(client, server)
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertEqual(json.loads(result.stdout)["rounds"][0]["failures"], [
            "missing_controlled_rejection", "missing_rejected_key_recovery", "missing_replay_during_pressure",
        ])

    def test_trial_requires_exact_database_windows(self):
        client, server = records()
        client["origin_scope"] = "trial"
        self.assertEqual(self.assess(client, server).returncode, 2)
        raw = observations(client, server)
        raw[0]["accepted"][0]["accepted_at"] = "1970-01-01T00:00:00.003999999Z"
        result = self.assess(client, server, exports=raw)
        self.assertEqual(result.returncode, 0, result.stderr)
        evidence = json.loads(result.stdout)["rounds"][0]["independent_acceptance"]
        self.assertEqual(evidence["accepted"], 4)
        self.assertEqual(evidence["sha256"], hashlib.sha256(json.dumps(raw[0]).encode()).hexdigest())
        self.assertEqual(self.assess(client, server, exports=raw[::-1]).returncode, 2)
        raw[2]["accepted"][0]["id"] = raw[0]["accepted"][0]["id"]
        self.assertEqual(self.assess(client, server, exports=raw).returncode, 2)

    def test_raw_receipts_detect_omissions_and_shifted_saturation_times(self):
        client, server = records()
        raw = observations(client, server)
        raw[0]["accepted"].append(dict(raw[0]["accepted"][0], id="unmatched-commit"))
        raw[1]["accepted"].pop()
        # Move the first receipt by one millisecond. The client attempt interval
        # still matches the hand-edited ledger, but raw database time disagrees.
        raw[2]["accepted"][0]["accepted_at"] = utc(client["rounds"][2]["started_utc_ms"] + 3)
        result = self.assess(client, server, exports=raw)
        self.assertEqual(result.returncode, 1, result.stderr)
        for round_, key in zip(json.loads(result.stdout)["rounds"], (
                "unreconciled_database_receipts", "ledger_receipts_missing_from_database", "acceptance_time_mismatches")):
            self.assertEqual(round_["failures"], ["independent_acceptance_reconciliation"])
            self.assertEqual(round_["independent_acceptance"][key], 1)
        self.assertNotIn("unmatched-commit", result.stdout)
