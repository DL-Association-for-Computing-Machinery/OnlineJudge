"""Check that evidence describes actual input structure, roles and identities."""

import importlib.util
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch


SCRIPTS = Path(__file__).resolve().parents[2] / "scripts"
sys.path.insert(0, str(SCRIPTS))
from library_checker_coverage import observe
from problem_draft_files import canonical, digest, write_new

spec = importlib.util.spec_from_file_location("coverage_audit", SCRIPTS / "audit-library-checker-coverage.py")
audit = importlib.util.module_from_spec(spec)
spec.loader.exec_module(audit)


class Observations(unittest.TestCase):
    def test_lookup_and_overwrite_follow_the_operation_order(self):
        observed = observe("associative_array", b"5\n1 0\n0 0 0\n1 0\n0 0 4\n1 0\n")
        self.assertEqual(observed, {"key=0", "value=0", "lookup_missing", "lookup_existing", "overwrite"})
        self.assertEqual(observe("associative_array", b"1\n1 0\n"), {"q=1", "key=0", "lookup_missing"})

    def test_range_boundaries_do_not_count_updates_as_queries(self):
        raw = b"3 3\n1 0 2\n0 0 3\n1 0 2\n1 2 3\n"
        observed = observe("point_add_range_sum", raw)
        self.assertIn("query_after_update", observed)
        self.assertNotIn("whole_range", observed)
        self.assertIn("one_element_range", observed)
        self.assertIn("ends_at_n", observed)
        with self.assertRaisesRegex(ValueError, "missing query"):
            observe("point_add_range_sum", raw.rsplit(b"\n", 2)[0] + b"\n")

    def test_string_period_and_convolution_sides_are_distinct(self):
        self.assertEqual(observe("zalgorithm", b"aaaa\n"), {"all_equal_n_gt_1"})
        self.assertEqual(observe("zalgorithm", b"abab\n"), {"two_equal_halves_nonconstant", "alternating"})
        self.assertEqual(observe("zalgorithm", b"aabbaabb\n"), {"two_equal_halves_nonconstant"})
        self.assertEqual(observe("convolution_mod", b"1 2\n998244352\n0 7\n"),
                         {"n=1", "unequal_lengths", "one_singleton", "left_value=998244352",
                          "right_value=0", "right_one_nonzero"})

    def test_verified_report_separates_roles_and_rejects_changed_bytes(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "bundle"
            root.mkdir(mode=0o700)
            objects = {}

            def put(path, value):
                ref = write_new(root, path, value)
                objects[path] = {k: ref[k] for k in ("sha256", "size")}
                return ref

            sid = "library-checker:aplusb"
            name = "drafts/aplusb.json"
            sample = put("input/sample", b"1000000000 1000000000\n")
            hidden = put("input/hidden", b"0 3\n")
            draft = {"schema": "oj.problem-draft.v1", "state": "private-draft",
                     "validation": {"status": "not-run", "publication_blockers": ["test"]},
                     "provenance": {"source_id": sid, "source_lock_sha256": audit.SOURCE_LOCK, "original_files": {}},
                     "statement": put("statement.md", b"Add two integers.\n"), "examples": ["case-0001"],
                     "test_cases": [
                         {"id": "case-0001", "sample": True, "input": sample,
                          "answer": put("answer/sample", b"2000000000\n")},
                         {"id": "case-0002", "sample": False, "input": hidden,
                          "answer": put("answer/hidden", b"3\n")}]}
            draft["version"] = digest(canonical(draft))
            put(name, canonical(draft))
            index = canonical({"schema": "oj.private-draft-bundle.v1", "state": "private-unpublished",
                               "problems": [{"source_id": sid, "draft": name, "version": draft["version"],
                                             "test_cases": 2, "examples": 1}], "files": objects})
            write_new(root, "index.json", index)
            with self.assertRaisesRegex(ValueError, "reviewed bundle indexes"):
                audit.audit(root)
            with patch.object(audit, "V6_INDEX", digest(index)):
                report = audit.audit(root)["problems"][0]["rules"]
                self.assertEqual(report["sum=2000000000"]["sample"]["first"]["input_sha256"], sample["sha256"])
                self.assertEqual(report["sum=2000000000"]["non_sample"], {"count": 0, "first": None})
                self.assertEqual(report["a=0"]["non_sample"]["first"]["case"], "case-0002")
                (root / hidden["path"]).write_bytes(b"1 2\n")
                with self.assertRaisesRegex(ValueError, "integrity"):
                    audit.audit(root)
