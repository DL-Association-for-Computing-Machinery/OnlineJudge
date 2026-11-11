"""Regression checks for offline source integrity and the host export boundary."""

import argparse
import copy
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile
import unittest
from unittest.mock import patch


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts"))
import problem_source_common as common
import problem_source_export as export
import validate_library_checker as runner

spec = importlib.util.spec_from_file_location("prepare_lc", ROOT / "scripts/prepare-library-checker.py")
prepare = importlib.util.module_from_spec(spec)
spec.loader.exec_module(prepare)


class TemporaryCase(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)


class SourceTests(TemporaryCase):
    def test_pinned_lock_has_ten_complete_sources_and_licenses(self):
        lock = common.load_lock(ROOT / "config/problem-sources/library-checker.json")
        self.assertEqual(lock["commit"], "73876f1cbcf082d2b22722924b924d3d65a2f00e")
        self.assertEqual(len(lock["problems"]), 10)
        self.assertIn("common/testlib.h", lock["files"])
        self.assertIn("LICENSE", lock["files"])
        for p in lock["problems"]:
            self.assertIn(p["path"] + "/checker.cpp", lock["files"])

    def test_portable_paths_reject_traversal_aliases_and_platform_separators(self):
        for name in (".", "..", "../x", "/x", "a/../x", "a//x", "./x", "a/", "a\\x", "a:x"):
            with self.subTest(name=name), self.assertRaises(ValueError):
                common.safe_path(name)

    def test_snapshot_checks_hash_before_copy_and_does_not_copy_extra_code(self):
        (self.root / "good.cpp").write_bytes(b"source")
        (self.root / "unlocked.cpp").write_bytes(b"extra")
        target = self.root / "snapshot"
        lock = {"files": {"good.cpp": {"sha256": common.digest(b"source"), "size": 6}}}
        common.snapshot(self.root, target, lock)
        self.assertEqual(list(p.name for p in target.iterdir()), ["good.cpp"])
        (self.root / "good.cpp").write_bytes(b"broken")
        with self.assertRaisesRegex(ValueError, "differs from lock"):
            common.snapshot(self.root, self.root / "other", lock)
        self.assertFalse((self.root / "other").exists())

    def test_reader_rejects_final_and_directory_symlinks_and_size_limit(self):
        (self.root / "real").mkdir()
        (self.root / "real/f").write_bytes(b"abc")
        (self.root / "file").symlink_to(self.root / "real/f")
        (self.root / "dir").symlink_to(self.root / "real", target_is_directory=True)
        for name in ("file", "dir/f"):
            with self.subTest(name=name), self.assertRaises(OSError):
                common.read_regular(self.root, name)
        with self.assertRaises(ValueError):
            common.read_regular(self.root, "real/f", 2)

    def test_exact_case_inventory_uses_config_order_and_only_example_group_is_sample(self):
        (self.root / "p").mkdir()
        (self.root / "p/info.toml").write_text(
            'title="Task"\ntimelimit=1\n[[tests]]\nname="example.in"\nnumber=1\n'
            '[[tests]]\nname="hack.in"\nnumber=1\n')
        hashes = {name + ext: common.digest(b"data")
                  for name in ("example_00", "hack_00") for ext in (".in", ".out")}
        (self.root / "p/hash.json").write_text(json.dumps(hashes))
        lock = {"files": {}, "problems": [{"path": "p", "title_zh": "题目"}]}
        expected, catalog = common.expected_materials(self.root, lock)
        self.assertEqual(len(expected), 4)
        self.assertEqual(catalog[0]["cases"], [{"id": "example_00", "sample": True},
                                               {"id": "hack_00", "sample": False}])
        hashes["unexpected_00.in"] = common.digest(b"data")
        (self.root / "p/hash.json").write_text(json.dumps(hashes))
        with self.assertRaisesRegex(ValueError, "configuration"):
            common.expected_materials(self.root, lock)


class ExportTests(TemporaryCase):
    def make_archive(self, members):
        path = self.root / "export.tar"
        with tarfile.open(path, "w", format=tarfile.USTAR_FORMAT) as tar:
            for info, data in members:
                if isinstance(info, str):
                    info = tarfile.TarInfo(info)
                info.size = len(data)
                tar.addfile(info, io.BytesIO(data))
        target = self.root / "result"
        target.mkdir(exist_ok=True)
        return path, target

    def entries(self):
        return [("./source/LICENSE", b"license"), ("./validation.json", b'{}')]

    def expected(self):
        return {"source/LICENSE": {"sha256": common.digest(b"license"), "size": 7}}

    def test_exact_regular_export_and_private_modes(self):
        archive, target = self.make_archive(self.entries())
        report, size = export.extract(archive, target, self.expected())
        self.assertEqual(report, {})
        self.assertEqual(size, 9)
        self.assertEqual((target / "source/LICENSE").stat().st_mode & 0o777, 0o600)

    def test_damaged_or_missing_pinned_material_and_unknown_entries_fail(self):
        variants = [self.entries()[:1], self.entries()[1:],
                    [("./source/LICENSE", b"damaged")] + self.entries()[1:],
                    self.entries() + [("extra", b"untrusted")]]
        for i, entries in enumerate(variants):
            with self.subTest(i=i):
                archive, _ = self.make_archive(entries)
                target = self.root / ("result" + str(i))
                target.mkdir()
                with self.assertRaises(ValueError):
                    export.extract(archive, target, self.expected())

    def test_unsafe_archive_members_are_rejected_without_escape(self):
        variants = []
        for name in ("../escaped", "/escaped", "./source/../escaped", "source//LICENSE"):
            variants.append((tarfile.TarInfo(name), b"x"))
        for kind in (tarfile.SYMTYPE, tarfile.LNKTYPE, tarfile.FIFOTYPE):
            info = tarfile.TarInfo("source/LICENSE")
            info.type, info.linkname = kind, "../../escaped"
            variants.append((info, b""))
        for i, member in enumerate(variants):
            archive, _ = self.make_archive([member])
            target = self.root / ("bad" + str(i))
            target.mkdir()
            with self.subTest(i=i), self.assertRaises(ValueError):
                export.extract(archive, target, self.expected())
        self.assertFalse((self.root / "escaped").exists())

    def test_duplicate_and_case_colliding_entries_fail(self):
        for i, name in enumerate(("./source/LICENSE", "./source/license")):
            archive, _ = self.make_archive(self.entries() + [(name, b"license")])
            target = self.root / ("duplicates" + str(i))
            target.mkdir()
            with self.assertRaisesRegex(ValueError, "duplicate"):
                export.extract(archive, target, self.expected())

    def test_large_file_and_metadata_are_bounded_before_read(self):
        with self.assertRaises(ValueError):
            export.BoundedArchive(io.BytesIO()).read((1 << 20) + 1)
        archive, target = self.make_archive(self.entries())
        with patch.object(export, "MAX_DATA_FILE", 6), self.assertRaisesRegex(ValueError, "budget"):
            export.extract(archive, target, self.expected())

    def test_false_validation_or_wrong_revision_cannot_complete(self):
        lock = {"commit": "pinned"}
        catalog = [{"source_id": "library-checker:x", "cases": [{}]}]
        report = {"schema": "oj.source-validation.v1", "commit": "pinned", "problems": []}
        with self.assertRaises(ValueError):
            export.validate_report(report, lock, catalog)
        report["commit"] = "other"
        with self.assertRaisesRegex(ValueError, "identity"):
            export.validate_report(report, lock, catalog)

    def test_new_preparation_requires_complete_bound_controls_but_old_imports_remain_valid(self):
        lock = {"commit": "pinned"}
        catalog = [{"source_id": "library-checker:aplusb", "cases": [{}]}]
        result = {"source_id": "library-checker:aplusb", "cases": 1, "input_validator": "passed-all",
                  "reference_checker": "passed-all", "invalid_output_checker_exit": 1}
        old = {"schema": "oj.source-validation.v1", "commit": "pinned", "problems": [result]}
        export.validate_report(old, lock, catalog)
        with self.assertRaisesRegex(ValueError, "identity"):
            export.validate_report(old, lock, catalog, require_controls=True)

        report = copy.deepcopy(old)
        report["schema"] = "oj.source-validation.v2"
        report["problems"][0]["input_controls"] = [
            {"id": name, "input_sha256": common.digest(payload), "exit_code": code}
            for name, payload, code in (
                ("valid-boundary", b"0 1000000000\n", 0),
                ("outside-domain", b"0 1000000001\n", 3),
                ("truncated", b"0 1000000000", 3),
                ("trailing-data", b"0 1000000000\n0\n", 3),
            )
        ]
        export.validate_report(report, lock, catalog, require_controls=True)
        for change in ("missing", "wrong-input", "untyped-exit", "crash"):
            with self.subTest(change=change):
                damaged = copy.deepcopy(report)
                controls = damaged["problems"][0]["input_controls"]
                if change == "missing":
                    controls.pop()
                elif change == "wrong-input":
                    controls[1]["input_sha256"] = controls[0]["input_sha256"]
                elif change == "untyped-exit":
                    controls[0]["exit_code"] = False
                else:
                    controls[1]["exit_code"] = -9
                with self.assertRaisesRegex(ValueError, "incomplete"):
                    export.validate_report(damaged, lock, catalog, require_controls=True)


class InputControlTests(unittest.TestCase):
    def test_accepting_invalid_input_or_a_crash_does_not_count_as_rejection(self):
        for exit_code in (0, -9):
            with self.subTest(exit_code=exit_code), patch.object(
                runner, "checked", side_effect=[subprocess.CompletedProcess([], code) for code in (0, exit_code)],
            ) as checked, self.assertRaisesRegex(ValueError, "outside-domain"):
                runner.run_input_controls(Path("/fixture/verifier"), "library-checker:convolution_mod")
            self.assertEqual(checked.call_count, 2)
            self.assertEqual(checked.call_args.kwargs["input"], b"1 1\n0\n998244353\n")


class DriverTests(TemporaryCase):
    def test_docker_diagnostics_have_a_real_child_file_limit_and_bounded_display(self):
        command = [sys.executable, "-c", "import os; os.write(2, b'x'*(2<<20)); os.write(2, b'x')"]
        with patch.object(prepare, "DOCKER", command):
            with self.assertRaises(RuntimeError) as failure:
                prepare.docker("probe", stdout=subprocess.DEVNULL)
        self.assertIn("Docker probe failed", str(failure.exception))
        self.assertLess(len(str(failure.exception)), 8300)

    def test_existing_output_is_refused_before_any_docker_call(self):
        args = argparse.Namespace(source=self.root, image="local", output=self.root)
        with patch.object(prepare, "docker") as docker, self.assertRaises(ValueError):
            prepare.run(args)
        docker.assert_not_called()

    def test_image_declared_volume_is_rejected(self):
        args = argparse.Namespace(source=self.root, image="local", output=self.root / "output")
        response = subprocess.CompletedProcess([], 0, stdout=json.dumps([
            {"Config": {"Volumes": {"/data": {}}}, "Id": "sha256:test"}]).encode())
        with patch.object(prepare, "docker", return_value=response), self.assertRaisesRegex(ValueError, "volumes"):
            prepare.run(args)
        self.assertFalse(args.output.exists())

    def test_timeout_and_cancellation_remove_only_owned_container(self):
        for i, failure in enumerate((subprocess.TimeoutExpired("exec", 3600), KeyboardInterrupt())):
            args = argparse.Namespace(source=self.root, image="local", output=self.root / ("output" + str(i)))
            calls = []
            container_id = "a" * 64
            def fake_docker(*command, **kwargs):
                calls.append(command)
                data = b""
                if command[:2] == ("image", "inspect"):
                    data = json.dumps([{"Config": {"Volumes": None}, "Id": "sha256:local",
                                        "Architecture": "arm64"}]).encode()
                elif command[0] == "create":
                    data = container_id.encode()
                elif command[0] == "inspect":
                    data = json.dumps([{"Id": container_id, "Config": {"Labels": {"owner": prepare.OWNER}}}]).encode()
                elif command[0] == "exec":
                    raise failure
                return subprocess.CompletedProcess(command, 0, stdout=data)
            with patch.object(prepare, "docker", side_effect=fake_docker), \
                    patch.object(prepare, "snapshot"), patch.object(prepare, "expected_materials", return_value=({}, [])), \
                    patch.object(prepare, "isolated"), self.assertRaises(type(failure)):
                prepare.run(args)
            self.assertIn(("rm", "--force", container_id), calls)
            self.assertTrue((args.output / "INCOMPLETE").exists())
            self.assertFalse((args.output / "materials").exists())
            creation = next(c for c in calls if c[0] == "create")
            self.assertIn("none", creation)
            self.assertIn("65532:65532", creation)
            self.assertNotIn("--privileged", creation)

    def test_cleanup_refuses_another_owners_container(self):
        data = json.dumps([{"Id": "a" * 64, "Config": {"Labels": {"owner": "other"}}}]).encode()
        with patch.object(prepare, "docker", return_value=subprocess.CompletedProcess([], 0, stdout=data)) as docker:
            with self.assertRaisesRegex(ValueError, "unowned"):
                prepare.remove_owned("oj-source-test")
        self.assertEqual(docker.call_count, 1)

    def test_export_or_cleanup_failure_never_completes_candidate(self):
        for failure in ("export", "cleanup", None):
            args = argparse.Namespace(source=self.root, image="local", output=self.root / str(failure))
            container_id = "a" * 64
            def fake_docker(*command, **kwargs):
                data = b""
                if command[:2] == ("image", "inspect"):
                    data = json.dumps([{"Config": {}, "Id": "sha256:local", "Architecture": "arm64"}]).encode()
                elif command[0] == "create":
                    data = container_id.encode()
                elif command[0] == "inspect":
                    data = json.dumps([{"Config": {"Labels": {"owner": prepare.OWNER}}}]).encode()
                elif command[0] == "exec" and "tar" in command and failure == "export":
                    raise RuntimeError("export failed")
                return subprocess.CompletedProcess(command, 0, stdout=data)
            def cleanup(name):
                self.assertEqual(name, container_id)
                self.assertTrue((args.output / "INCOMPLETE").exists())
                self.assertFalse((args.output / "materials").exists())
                if failure == "cleanup":
                    raise RuntimeError("cleanup failed")
            with patch.object(prepare, "docker", side_effect=fake_docker), \
                    patch.object(prepare, "snapshot"), patch.object(prepare, "expected_materials", return_value=({}, [])), \
                    patch.object(prepare, "isolated"), patch.object(prepare, "remove_owned", side_effect=cleanup), \
                    patch.object(prepare, "extract", return_value=({}, 0)), patch.object(prepare, "validate_report"):
                if failure:
                    with self.assertRaises(RuntimeError):
                        prepare.run(args)
                else:
                    prepare.run(args)
            self.assertEqual((args.output / "INCOMPLETE").exists(), bool(failure))
            self.assertEqual((args.output / "materials").exists(), not failure)


if __name__ == "__main__":
    unittest.main()
