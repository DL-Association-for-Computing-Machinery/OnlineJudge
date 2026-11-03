"""Behavior checks for private, self-contained problem draft adaptation."""

import importlib.util
import json
import os
from pathlib import Path
import shutil
import sys
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts"))
import library_checker_statement as statement
import problem_draft_files as files
import problem_source_common as common

spec = importlib.util.spec_from_file_location("import_lc", ROOT / "scripts/import-library-checker.py")
importer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(importer)


class StatementTests(unittest.TestCase):
    def test_english_params_formula_and_samples_preserve_source_meaning(self):
        template = ('## @{keyword.statement}\n@{lang.en}\nCompute $A+@{param.N}$.\n'
                    '@{lang.ja}\nJapanese\n@{lang.end}\n@{example.example_00}\n')
        markdown, examples = statement.convert(template, {"N": 1000}, {"example_00": ("1 2\n", "3")})
        self.assertIn("Compute $A+1000$.", markdown)
        self.assertNotIn("Japanese", markdown)
        self.assertNotIn("@{", markdown)
        self.assertIn("```text\n1 2\n```", markdown)
        self.assertEqual(examples, ["example_00"])

    def test_sample_cannot_end_its_own_code_fence(self):
        text, _ = statement.convert("@{example.x}", {}, {"x": ("```\n", "x")})
        self.assertIn("````text\n```\n````", text)

    def test_unknown_malformed_language_and_parameter_changes_fail_closed(self):
        templates = ["@{unknown.x}", "@{param.MISSING}", "@{param.N}",
                     "@{lang.end}\n", "@{lang.en}\n", "@{lang.ja}\n",
                     "@{lang.en}\n@{lang.en}\n", "@{example.private_00}",
                     "@{lang.en}\n@{lang.ja}\n@{unknown.x}\n@{lang.end}\n",
                     "@{lang.en}\n@{lang.ja}\n@{bad\n@{lang.end}\n",
                     "prefix@{lang.en}\n"]
        for text in templates:
            with self.subTest(text=text), self.assertRaises(ValueError):
                statement.convert(text, {"N": True}, {})

    def test_missing_duplicate_or_private_examples_rejected(self):
        for text in ("no example", "@{example.x}\n@{example.x}", "@{example.private}"):
            with self.subTest(text=text), self.assertRaises(ValueError):
                statement.convert(text, {}, {"x": ("1", "2")})


class DraftTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.repo = self.root / "repo"
        self.corpus = self.root / "corpus"
        self.source = self.corpus / "source"
        self.source.mkdir(parents=True)
        self.output = self.root / "output"
        lock = {"schema": "oj.library-checker-source.v1", "commit": "a" * 40,
                "repository": "https://github.com/yosupo06/library-checker-problems",
                "files": {}, "problems": []}
        def raw(name, data):
            target = self.source / name
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(data)
            lock["files"][name] = {"size": len(data), "sha256": common.digest(data)}
        for name in ("LICENSE", "generate.py", "common/testlib.h"):
            raw(name, b"original source/notice\n")
        for i in range(10):
            base = "p" + str(i)
            lock["problems"].append({"path": base, "title_zh": "题目" + str(i)})
            raw(base + "/info.toml", b'title="Task"\ntimelimit=2.0\n'
                b'[[tests]]\nname="example.in"\nnumber=1\n'
                b'[[tests]]\nname="random.cpp"\nnumber=1\n[params]\nN=9\n')
            raw(base + "/task.md", b'@{lang.en}\n$N=@{param.N}$\n@{lang.ja}\nJP\n@{lang.end}\n@{example.example_00}\n')
            hashes = {}
            for case in ("example_00", "random_00"):
                for folder, suffix in (("in", ".in"), ("out", ".out")):
                    data = (case + suffix + str(i)).encode()
                    target = self.corpus / "problems" / base / folder / (case + suffix)
                    target.parent.mkdir(parents=True, exist_ok=True)
                    target.write_bytes(data)
                    hashes[case + suffix] = common.digest(data)
            raw(base + "/hash.json", files.canonical(hashes))
            for name in ("checker.cpp", "verifier.cpp", "sol/correct.cpp"):
                raw(base + "/" + name, b"private program; never execute")
        path = self.repo / "config/problem-sources"
        path.mkdir(parents=True)
        self.lock_bytes = files.canonical(lock)
        (path / "library-checker.json").write_bytes(self.lock_bytes)
        _, catalog = common.expected_materials(self.source, lock)
        self.catalog = {"schema": "oj.private-source-corpus.v1", "status": "private-unpublished",
                        "repository": lock["repository"], "commit": lock["commit"],
                        "source_lock_sha256": common.digest(self.lock_bytes), "problems": catalog}
        self.report = {"schema": "oj.source-validation.v1", "commit": lock["commit"],
                       "elapsed_seconds": 1, "problems": [
                           {"source_id": p["source_id"], "cases": 2, "input_validator": "passed-all",
                            "reference_checker": "passed-all", "invalid_output_checker_exit": 1}
                           for p in catalog]}
        self.save_metadata()
        self.patch = patch.object(importer, "ROOT", self.repo)
        self.patch.start()
        self.addCleanup(self.patch.stop)

    def save_metadata(self):
        (self.corpus / "catalog.json").write_bytes(files.canonical(self.catalog))
        (self.corpus / "validation.json").write_bytes(files.canonical(self.report))

    def run_import(self, output=None):
        return importer.run(self.corpus, output or self.output)

    def test_self_contained_relocatable_drafts_keep_private_roles_and_block_publication(self):
        index = self.run_import()
        self.assertEqual(len(index["problems"]), 10)
        bundle = self.output / "bundle"
        draft = json.loads((bundle / "drafts/p0.json").read_bytes())
        self.assertEqual(draft["examples"], ["case-0001"])
        self.assertEqual([c["sample"] for c in draft["test_cases"]], [True, False])
        self.assertEqual(draft["judging"]["case_sensitive"], True)
        self.assertEqual(draft["judging"]["numeric_equivalence"], False)
        self.assertIsNone(draft["judging"]["limits"])
        self.assertEqual(draft["validation"]["status"], "not-run")
        self.assertIn("authorized-non-author-review", draft["validation"]["publication_blockers"])
        data = draft["test_cases"][0]["answer"]
        self.assertEqual((bundle / data["path"]).read_bytes(), b"example_00.out0")
        for path in [self.output, *self.output.rglob("*")]:
            self.assertEqual(path.stat().st_mode & 0o777, 0o700 if path.is_dir() else 0o600)
        self.assertFalse((self.output / "INCOMPLETE").exists())
        self.corpus.rename(self.root / "original-unavailable")
        moved = self.root / "relocated"
        shutil.copytree(bundle, moved)
        self.assertEqual(files.verify_bundle(moved), index)
        for path in moved.rglob("*.json"):
            self.assertNotIn(str(self.root), path.read_text())

    def test_versions_do_not_depend_on_output_or_validation_elapsed_time(self):
        first = self.run_import()["problems"]
        self.report["elapsed_seconds"] = 999
        self.save_metadata()
        second = self.run_import(self.root / "again")["problems"]
        self.assertEqual(first, second)

    def test_changed_statement_changes_version(self):
        before = self.run_import()["problems"]
        source = self.source / "p0/task.md"
        source.write_bytes(source.read_bytes() + b"Changed content\n")
        lock_path = self.repo / "config/problem-sources/library-checker.json"
        lock = json.loads(lock_path.read_bytes())
        lock["files"]["p0/task.md"] = {"sha256": common.digest(source.read_bytes()), "size": source.stat().st_size}
        lock_path.write_bytes(files.canonical(lock))
        self.catalog["source_lock_sha256"] = common.digest(lock_path.read_bytes())
        self.save_metadata()
        after = self.run_import(self.root / "changed")["problems"]
        self.assertNotEqual(before[0]["version"], after[0]["version"])

    def test_inventory_is_derived_from_verified_bytes_during_source_changes(self):
        original = importer.expected_materials
        metadata = self.source / "p0/info.toml"
        raw = metadata.read_bytes()
        def changing_source(snapshot, lock):
            metadata.write_bytes(raw.replace(b"timelimit=2.0", b"timelimit=999.0"))
            try:
                return original(snapshot, lock)
            finally:
                metadata.write_bytes(raw)
        with patch.object(importer, "expected_materials", side_effect=changing_source):
            self.run_import()
        draft = json.loads((self.output / "bundle/drafts/p0.json").read_bytes())
        self.assertEqual(draft["provenance"]["upstream_time_limit_seconds"], 2.0)

    def test_source_directory_alias_and_source_damage_are_rejected(self):
        actual = self.source / "p0"
        actual.rename(self.source / "real")
        actual.symlink_to(self.source / "real", target_is_directory=True)
        with self.assertRaises(OSError):
            self.run_import()
        actual.unlink()
        (self.source / "real").rename(actual)
        (actual / "task.md").write_bytes(b"corrupted")
        with self.assertRaisesRegex(ValueError, "metadata differs"):
            self.run_import()
        self.assertFalse(self.output.exists())

    def test_damaged_private_test_is_not_imported_and_keeps_incomplete(self):
        (self.corpus / "problems/p9/out/random_00.out").write_bytes(b"damaged")
        with self.assertRaisesRegex(ValueError, "pinned hash"):
            self.run_import()
        self.assertTrue((self.output / "INCOMPLETE").exists())
        self.assertFalse((self.output / "bundle").exists())

    def test_existing_output_or_overlap_never_overwrites_input(self):
        for output in (self.corpus, self.source / "new", self.repo / "new"):
            with self.subTest(output=output), self.assertRaises(ValueError):
                self.run_import(output)
        self.assertTrue((self.source / "p0/task.md").exists())

    def test_extra_missing_symlink_fifo_and_directory_are_refused(self):
        target = self.corpus / "problems/p0/out/random_00.out"
        data = target.read_bytes()
        for kind in ("missing", "extra", "symlink", "fifo", "directory"):
            target.unlink()
            if kind == "extra":
                target.write_bytes(data)
                (self.corpus / "unexpected").write_bytes(b"x")
            elif kind == "symlink":
                target.symlink_to(self.source / "LICENSE")
            elif kind == "fifo":
                os.mkfifo(target)
            elif kind == "directory":
                target.mkdir()
            with self.subTest(kind=kind), self.assertRaises((ValueError, OSError)):
                self.run_import()
            self.assertFalse(self.output.exists())
            if kind == "extra":
                (self.corpus / "unexpected").unlink()
            if target.is_dir():
                target.rmdir()
            elif target.exists() or target.is_symlink():
                target.unlink()
            target.write_bytes(data)

    def test_wrong_catalog_or_report_and_incomplete_source_refused(self):
        for key in ("source_lock_sha256", "commit", "problems", "status"):
            saved = self.catalog[key]
            self.catalog[key] = None
            self.save_metadata()
            with self.subTest(key=key), self.assertRaises(ValueError):
                self.run_import()
            self.catalog[key] = saved
        self.save_metadata()
        self.report["problems"][0]["reference_checker"] = "failed"
        self.save_metadata()
        with self.assertRaisesRegex(ValueError, "validation results"):
            self.run_import()
        (self.corpus.parent / "INCOMPLETE").write_bytes(b"not completed")
        with self.assertRaisesRegex(ValueError, "incomplete"):
            self.run_import()
        self.assertFalse(self.output.exists())

    def test_copy_budget_and_write_or_cancel_failure_never_complete(self):
        for i, failure in enumerate((OSError("disk full"), KeyboardInterrupt())):
            output = self.root / ("fail" + str(i))
            with patch.object(importer, "copy_checked", side_effect=failure), self.assertRaises(type(failure)):
                self.run_import(output)
            self.assertTrue((output / "INCOMPLETE").exists())
            self.assertFalse((output / "bundle").exists())
        with patch.object(files, "MAX_EXPORT", 10), self.assertRaisesRegex(ValueError, "budget"):
            self.run_import()
        self.assertTrue((self.output / "INCOMPLETE").exists())

    def test_relocated_bundle_detects_material_corruption(self):
        self.run_import()
        bundle = self.output / "bundle"
        obj = next((bundle / "objects/sha256").iterdir())
        obj.write_bytes(b"corrupt")
        with self.assertRaisesRegex(ValueError, "integrity"):
            files.verify_bundle(bundle)

    def test_index_cannot_omit_duplicate_or_misdescribe_drafts(self):
        self.run_import()
        bundle = self.output / "bundle"
        path = bundle / "index.json"
        original = path.read_bytes()
        for mutation in ("empty", "duplicate", "count", "identity", "examples"):
            index = json.loads(original)
            if mutation == "empty":
                index["problems"] = []
            elif mutation == "duplicate":
                index["problems"].append(index["problems"][0])
            elif mutation == "count":
                index["problems"][0]["test_cases"] = 999
            elif mutation == "identity":
                index["problems"][0]["source_id"] = "wrong"
            else:
                index["problems"][0]["examples"] = 999
            path.write_bytes(files.canonical(index))
            with self.subTest(mutation=mutation), self.assertRaises(ValueError):
                files.verify_bundle(bundle)
        path.write_bytes(original)

    def test_incomplete_and_nonprivate_bundle_are_not_complete(self):
        self.run_import()
        bundle = self.output / "bundle"
        (self.output / "INCOMPLETE").write_bytes(b"pending")
        with self.assertRaisesRegex(ValueError, "incomplete"):
            files.verify_bundle(bundle)
        (self.output / "INCOMPLETE").unlink()
        (bundle / "drafts/p0.json").chmod(0o644)
        with self.assertRaisesRegex(ValueError, "private"):
            files.verify_bundle(bundle)


if __name__ == "__main__":
    unittest.main()
