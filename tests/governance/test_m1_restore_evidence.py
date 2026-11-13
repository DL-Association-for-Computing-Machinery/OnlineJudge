import hashlib
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


SCRIPT = Path(__file__).resolve().parents[2] / "scripts/assess-m1-restore.py"


class RestoreEvidenceTest(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.manifest = Path(directory.name) / "manifest.json"
        self.record_file = Path(directory.name) / "evidence.json"
        self.record = {
            "format": "oj.m1-restore-evidence.v1",
            "source_domain": "source-host",
            "backup_domain": "backup-host",
            "restore_domain": "restore-host",
            "offsite_verified_at": "2026-11-12T13:00:00Z",
            "fault_declared_at": "2026-11-13T12:00:00Z",
            "checks": {key: {"passed": True, "at": at, "evidence_sha256": "a" * 64}
                       for key, at in {
                           "source_isolated": "2026-11-13T11:59:59Z",
                           "materials_verified": "2026-11-13T14:00:00Z",
                           "old_credentials_rejected": "2026-11-13T15:00:00Z",
                           "service_restored": "2026-11-13T15:00:00Z",
                           "historical_submission_rejudged": "2026-11-13T16:00:00Z",
                       }.items()},
        }
        self.snapshot("2026-11-12T12:00:00Z")

    def snapshot(self, captured):
        self.manifest.write_text(json.dumps({"format": "oj.logical-backup.v1", "captured_at": captured}))
        self.record["manifest_sha256"] = hashlib.sha256(self.manifest.read_bytes()).hexdigest()

    def invoke(self):
        self.record_file.write_text(json.dumps(self.record))
        return subprocess.run([sys.executable, str(SCRIPT), str(self.record_file), str(self.manifest)],
                              check=False, capture_output=True, text=True, timeout=5)

    def test_exact_limits_are_eligible_for_review(self):
        self.record["restore_domain"] = self.record["backup_domain"]
        result = self.invoke()
        self.assertEqual(result.returncode, 0, result.stderr)
        report = json.loads(result.stdout)
        self.assertTrue(report["eligible_for_review"])
        self.assertEqual((report["rpo_seconds"], report["rto_seconds"]), (86400, 14400))
        self.assertEqual(report["scope"], "recorded_evidence_only")

    def test_over_limit_and_shared_domain_and_failed_rejudge(self):
        self.snapshot("2026-11-12T11:59:59.999999999Z")
        self.record["backup_domain"] = "SOURCE-HOST"
        self.record["checks"]["historical_submission_rejudged"].update(at="2026-11-13T16:00:00.000000001Z", passed=False)
        result = self.invoke()
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertEqual(set(json.loads(result.stdout)["failures"]), {
            "rpo_exceeded", "rto_exceeded", "source_failure_domain_shared", "historical_submission_rejudged",
        })

    def test_restore_out_of_order_cannot_pass(self):
        self.record["checks"]["source_isolated"]["at"] = "2026-11-13T15:30:00Z"
        self.record["checks"]["materials_verified"]["at"] = "2026-11-13T15:45:00Z"
        result = self.invoke()
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertEqual(set(json.loads(result.stdout)["failures"]), {
            "source_isolation_too_late", "restore_observations_out_of_order",
        })

    def test_unverified_offsite_or_changed_manifest_cannot_pass(self):
        self.record["offsite_verified_at"] = "2026-11-13T12:00:00.000000001Z"
        result = self.invoke()
        self.assertEqual(result.returncode, 2)
        self.assertEqual(result.stdout, "")
        self.record["offsite_verified_at"] = "2026-11-12T13:00:00Z"
        with self.manifest.open("a") as file:
            file.write("\n")
        result = self.invoke()
        self.assertEqual(result.returncode, 2)
        self.assertEqual(result.stdout, "")
