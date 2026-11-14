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
        self.observations = Path(directory.name) / "observations"
        self.observations.mkdir()
        self.record = {
            "format": "oj.m1-restore-evidence.v1",
            "source_domain": "source-host",
            "backup_domain": "backup-host",
            "restore_domain": "restore-host",
            "offsite_verified_at": "2026-11-12T13:00:00Z",
            "fault_declared_at": "2026-11-13T12:00:00Z",
            "checks": {key: {"passed": True, "at": at}
                       for key, at in {
                           "source_isolated": "2026-11-13T11:59:59Z",
                           "materials_verified": "2026-11-13T14:00:00Z",
                           "old_credentials_rejected": "2026-11-13T15:00:00Z",
                           "service_restored": "2026-11-13T15:00:00Z",
                           "historical_submission_rejudged": "2026-11-13T16:00:00Z",
                       }.items()},
        }
        for key, check in self.record["checks"].items():
            raw = f"synthetic observation: {key}\n".encode()
            (self.observations / (key + ".txt")).write_bytes(raw)
            check["evidence_sha256"] = hashlib.sha256(raw).hexdigest()
        self.snapshot("2026-11-12T12:00:00Z")

    def snapshot(self, captured):
        self.manifest.write_text(json.dumps({"format": "oj.logical-backup.v1", "captured_at": captured}))
        self.record["manifest_sha256"] = hashlib.sha256(self.manifest.read_bytes()).hexdigest()

    def invoke(self, *args):
        self.record_file.write_text(json.dumps(self.record))
        return subprocess.run([sys.executable, str(SCRIPT), str(self.record_file), str(self.manifest),
                               str(self.observations), *args],
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

    def test_mismatched_missing_and_redirected_observations_are_refused(self):
        path = self.observations / "materials_verified.txt"
        original = path.read_bytes()
        for case in ("tampered", "missing", "symlink", "reused"):
            with self.subTest(case=case):
                path.unlink(missing_ok=True)
                if case == "tampered":
                    path.write_bytes((self.observations / "source_isolated.txt").read_bytes())
                elif case == "symlink":
                    copy = self.observations / "copy.txt"
                    copy.write_bytes(original)
                    path.symlink_to(copy)
                elif case == "reused":
                    duplicate = (self.observations / "source_isolated.txt").read_bytes()
                    path.write_bytes(duplicate)
                    self.record["checks"]["materials_verified"]["evidence_sha256"] = hashlib.sha256(duplicate).hexdigest()
                result = self.invoke()
                self.assertEqual(result.returncode, 2)
                self.assertEqual(result.stdout, "")

    def test_old_credentials_must_be_checked_before_return_to_service(self):
        self.record["checks"]["old_credentials_rejected"]["at"] = "2026-11-13T15:00:00.000000001Z"
        result = self.invoke()
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertEqual(json.loads(result.stdout)["failures"], ["old_credentials_checked_after_restore"])

    def p6_fixture(self):
        self.record.update(format="oj.p6-restore-evidence.v1", origin_scope="local")
        for name, at in (("offsite_copy_verified", self.record["offsite_verified_at"]),
                         ("source_still_isolated", "2026-11-13T16:01:00Z")):
            raw = f"synthetic P6 observation: {name}\n".encode()
            (self.observations / (name + ".txt")).write_bytes(raw)
            self.record["checks"][name] = dict(at=at, passed=True, evidence_sha256=hashlib.sha256(raw).hexdigest())
        return dict(format="oj.p6-restore-topology.v1", registered_at="2026-11-12T12:30:00Z", domains={
            f"{name}-host": dict(host_domain=f"{name}-physical", storage_domain=f"{name}-storage")
            for name in ("source", "backup", "restore")
        })

    def p6_args(self, doc):
        topology = self.record_file.with_name("topology.json")
        topology.write_text(json.dumps(doc))
        digest = hashlib.sha256(topology.read_bytes()).hexdigest()
        self.record["topology_sha256"] = digest
        return ("--topology", str(topology), "--topology-sha256", digest)

    def test_p6_requires_topology_and_isolation_through_rejudge(self):
        args = self.p6_args(self.p6_fixture())
        self.assertEqual(self.invoke().returncode, 2)
        result = self.invoke(*args)
        self.assertEqual(result.returncode, 0, result.stderr)
        report = json.loads(result.stdout)
        self.assertEqual(report["origin_scope"], "local")
        self.assertEqual(report["rto_seconds"], 14_400)
        self.record["checks"]["source_still_isolated"]["at"] = "2026-11-13T15:59:59Z"
        self.record["checks"]["offsite_copy_verified"]["at"] = "2026-11-12T13:00:01Z"
        result = self.invoke(*args)
        self.assertEqual(result.returncode, 1)
        self.assertEqual(json.loads(result.stdout)["failures"],
                         ["offsite_observation_time_mismatch", "source_isolation_end_too_early"])

    def test_p6_different_labels_do_not_hide_shared_host_or_storage(self):
        topology = self.p6_fixture()
        topology["domains"]["backup-host"]["storage_domain"] = "SOURCE-STORAGE"
        topology["domains"]["restore-host"]["host_domain"] = "SOURCE-PHYSICAL"
        result = self.invoke(*self.p6_args(topology))
        self.assertEqual(result.returncode, 1)
        self.assertEqual(json.loads(result.stdout)["failures"],
                         ["source_host_domain_shared", "source_storage_domain_shared"])

    def test_p6_rejects_edited_or_late_topology_and_missing_offsite_proof(self):
        topology = self.p6_fixture()
        args = self.p6_args(topology)
        with Path(args[1]).open("a") as stream:
            stream.write("\n")
        self.assertEqual(self.invoke(*args).returncode, 2)
        topology["registered_at"] = "2026-11-12T13:00:01Z"
        self.assertEqual(self.invoke(*self.p6_args(topology)).returncode, 2)
        topology["registered_at"] = "2026-11-12T12:30:00Z"
        (self.observations / "offsite_copy_verified.txt").unlink()
        self.assertEqual(self.invoke(*self.p6_args(topology)).returncode, 2)
