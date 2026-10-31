"""Safety and protocol tests for the disposable session cost driver."""
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import unittest
from unittest import mock


spec = importlib.util.spec_from_file_location(
    "session_cost_driver", Path(__file__).resolve().parents[2] / "scripts/test-session-cost.py")
driver = importlib.util.module_from_spec(spec)
spec.loader.exec_module(driver)


class ProtocolTest(unittest.TestCase):
    nonce = "a" * 32
    config = {"backend": "redis", "workload": "find", "subjects": 32, "operations": 8,
              "fixture_sha256": "a" * 64}

    def result(self):
        return {**self.config, "schema": 1, "concurrency": 8, "padding_bytes": 1024,
                "valid": True, "elapsed_ns": 42,
                "latency_ns": {key: [1] * 8 if key == "success" else [] for key in driver.COUNTS},
                "counts": {key: 8 if key == "success" else 0 for key in driver.COUNTS}}

    def line(self, result):
        return ("OJ_SESSION_COST_RESULT " + self.nonce + " " + json.dumps(result)).encode()

    def test_valid_handshake_and_duplicate_rejected(self):
        protocol = driver.Protocol(self.nonce, self.config)
        self.assertEqual(protocol.line(("OJ_SESSION_COST_READY " + self.nonce).encode()), "ready")
        self.assertEqual(protocol.line(("OJ_SESSION_COST_MEASURED " + self.nonce).encode()), "measured")
        self.assertEqual(protocol.line(self.line(self.result())), "result")
        with self.assertRaises(ValueError):
            protocol.line(self.line(self.result()))

    def test_bad_nonce_and_out_of_order(self):
        for line in (self.line(self.result()), b"OJ_SESSION_COST_READY " + b"b" * 32,
                     b"OJ_SESSION_COST_READY " + self.nonce.encode() + b" extra",
                     b"OJ_SESSION_COST_MEASURED " + self.nonce.encode()):
            with self.subTest(line=line), self.assertRaises(ValueError):
                driver.Protocol(self.nonce, self.config).line(line)
        protocol = driver.Protocol(self.nonce, self.config)
        protocol.line(("OJ_SESSION_COST_READY " + self.nonce).encode())
        with self.assertRaises(ValueError):
            protocol.line(self.line(self.result()))

    def test_nonfinite_json_and_count_mismatch(self):
        for change in (lambda x: x.update(elapsed_ns=float("nan")),
                       lambda x: x["counts"].update(success=7, error=1),
                       lambda x: x["counts"].update(unclassified_error=1),
                       lambda x: x.update(concurrency=True)):
            protocol = driver.Protocol(self.nonce, self.config)
            protocol.line(("OJ_SESSION_COST_READY " + self.nonce).encode())
            protocol.line(("OJ_SESSION_COST_MEASURED " + self.nonce).encode())
            value = self.result()
            change(value)
            with self.subTest(value=value), self.assertRaises(ValueError):
                protocol.line(self.line(value))

    def test_reject_requires_expected_rejections(self):
        config = {**self.config, "workload": "reject"}
        result = {**self.result(), **config}
        with self.assertRaises(ValueError):
            driver.validate_result(result, config)
        result["counts"].update(success=0, expected_reject=8)
        result["latency_ns"].update(success=[], expected_reject=[1] * 8)
        driver.validate_result(result, config)

    def test_latency_categories_lengths_and_values(self):
        changes = (lambda x: x["latency_ns"].pop("stale"),
                   lambda x: x["latency_ns"].update(other=[]),
                   lambda x: x["latency_ns"]["success"].pop(),
                   lambda x: x["latency_ns"]["success"].__setitem__(0, True),
                   lambda x: x["latency_ns"]["success"].__setitem__(0, 0))
        for change in changes:
            result = self.result()
            change(result)
            with self.subTest(result=result), self.assertRaises(ValueError):
                driver.validate_result(result, self.config)

    def test_fixture_hash_must_match(self):
        result = self.result()
        result["fixture_sha256"] = "b" * 64
        with self.assertRaises(ValueError):
            driver.validate_result(result, self.config)

    def test_failed_protocol_preserves_raw_output(self):
        docker = driver.Docker("unused", "owner")
        docker.command = [sys.executable, "-c", "print('OJ_SESSION_COST_RESULT bad {bad json}')"]
        record = {}
        with self.assertRaises(ValueError):
            driver.run_protocol(docker, "unused", {}, self.nonce, self.config, record, time.monotonic() + 5)
        self.assertIn("OJ_SESSION_COST_RESULT bad {bad json}", record["failure_stdout"])
        self.assertFalse(record["failure_stdout_truncated"])


class StateTest(unittest.TestCase):
    def test_compile_failure_retains_diagnostics(self):
        error = subprocess.CalledProcessError(1, ["docker", "start"], output="compiler: invalid call", stderr=b"synthetic build stderr")
        self.assertIn("compiler: invalid call", driver.error_text(error))
        self.assertIn("synthetic build stderr", driver.error_text(error))

    def test_post_completion_temp_cleanup_failure_cannot_pass(self):
        real_tempdir = tempfile.TemporaryDirectory

        class FakeDocker:
            instances = []

            def __init__(self, context, owner):
                self.cleanups = 0
                self.instances.append(self)

            def create(self, *args):
                mount = next(arg for arg in args if arg.startswith("type=bind,src=") and arg.endswith(",dst=/out"))
                self.out = Path(mount.split(",")[1].removeprefix("src="))
                return "build"

            def run(self, *args, **kwargs):
                if args[:2] == ("start", "-a"):
                    (self.out / "cost.test").write_bytes(b"binary")
                    (self.out / "fixture.json").write_bytes(b"fixture")

            def cleanup(self):
                self.cleanups += 1
                return []

        for error in (RuntimeError("cleanup failed"), KeyboardInterrupt()):
            with self.subTest(error=type(error).__name__), real_tempdir() as tmp:
                class FailingTempDir:
                    def __init__(self, *args, **kwargs):
                        self.inner = real_tempdir(*args, **kwargs)

                    def __enter__(self):
                        return self.inner.__enter__()

                    def __exit__(self, *args):
                        self.inner.__exit__(*args)
                        raise error

                output = Path(tmp) / "result.json"
                argv = ["driver", "--module-cache", tmp, "--output", str(output), "--rounds", "1"]
                image = {"id": "sha256:" + "a" * 64, "os": "linux", "architecture": "amd64"}
                with (mock.patch.object(sys, "argv", argv), mock.patch.object(driver, "Docker", FakeDocker),
                      mock.patch.object(driver, "image_info", return_value=image),
                      mock.patch.object(driver, "check_container_state"), mock.patch.object(driver, "sample"),
                      mock.patch.object(driver.tempfile, "TemporaryDirectory", FailingTempDir),
                      mock.patch("builtins.print")):
                    if isinstance(error, KeyboardInterrupt):
                        with self.assertRaises(KeyboardInterrupt):
                            driver.main()
                    else:
                        self.assertEqual(driver.main(), 1)
                result = json.loads(output.read_text())
                self.assertEqual(result["status"], "failed")
                self.assertEqual(len(result["samples"]), 14)
                self.assertTrue(all(sample["status"] == "passed" for sample in result["samples"]))
                self.assertIn(type(error).__name__, result["error"])
                self.assertEqual(FakeDocker.instances[-1].cleanups, 15)

    def test_partial_run_cannot_pass(self):
        passed = [{"status": "passed"}] * 14
        self.assertTrue(driver.completed_run(True, passed, 1, []))
        self.assertFalse(driver.completed_run(False, passed, 1, []))
        self.assertFalse(driver.completed_run(True, passed[:-1], 1, []))
        self.assertFalse(driver.completed_run(True, passed[:-1] + [{"status": "running"}], 1, []))
        self.assertFalse(driver.completed_run(True, passed, 1, ["cleanup failure"]))

    def test_keyboard_interrupt_records_failure_and_cleans_up(self):
        class FakeDocker:
            instances = []

            def __init__(self, context, owner):
                self.cleanups = 0
                self.instances.append(self)

            def create(self, *args):
                mount = next(arg for arg in args if arg.startswith("type=bind,src=") and arg.endswith(",dst=/out"))
                self.out = Path(mount.split(",")[1].removeprefix("src="))
                return "build"

            def run(self, *args, **kwargs):
                if args[:2] == ("start", "-a"):
                    (self.out / "cost.test").write_bytes(b"binary")
                    (self.out / "fixture.json").write_bytes(b"fixture")

            def cleanup(self):
                self.cleanups += 1
                return []

        with tempfile.TemporaryDirectory() as tmp:
            output = Path(tmp) / "result.json"
            argv = ["driver", "--module-cache", tmp, "--output", str(output), "--rounds", "1"]
            images = {"id": "sha256:" + "a" * 64, "os": "linux", "architecture": "amd64"}
            with (mock.patch.object(sys, "argv", argv), mock.patch.object(driver, "Docker", FakeDocker),
                  mock.patch.object(driver, "image_info", return_value=images),
                  mock.patch.object(driver, "check_container_state"),
                  mock.patch.object(driver, "sample", side_effect=KeyboardInterrupt)):
                with self.assertRaises(KeyboardInterrupt):
                    driver.main()
            result = json.loads(output.read_text())
            self.assertEqual(result["status"], "failed")
            self.assertEqual(result["samples"][0]["status"], "failed")
            self.assertIn("KeyboardInterrupt", result["error"])
            self.assertGreaterEqual(FakeDocker.instances[-1].cleanups, 1)

    def test_terminal_state_rejects_nonzero_running_and_oom(self):
        docker = driver.Docker("unused", "owner")
        for state in ({"Running": False, "OOMKilled": False, "ExitCode": 1},
                      {"Running": True, "OOMKilled": False, "ExitCode": 0},
                      {"Running": False, "OOMKilled": True, "ExitCode": 0},
                      {"Running": False, "OOMKilled": False, "ExitCode": False}):
            with self.subTest(state=state), mock.patch.object(docker, "run", return_value=json.dumps(state)):
                with self.assertRaises(RuntimeError):
                    driver.check_container_state(docker, "id", False)

    def test_resource_windows_reject_negative_delta_and_nonmonotonic_time(self):
        before = {"pg": {"usage_usec": 10, "capture_begin_monotonic_ns": 1,
                         "capture_end_monotonic_ns": 2}}
        for last in ({"usage_usec": 9, "capture_begin_monotonic_ns": 3, "capture_end_monotonic_ns": 4},
                     {"usage_usec": 11, "capture_begin_monotonic_ns": 1, "capture_end_monotonic_ns": 4}):
            with self.subTest(last=last), self.assertRaises(ValueError):
                driver.resource_windows(before, {"pg": last})


class OwnershipTest(unittest.TestCase):
    def test_only_successfully_created_ids_are_removed(self):
        docker = driver.Docker("desktop-linux", "owner")
        with mock.patch.object(docker, "run", side_effect=["network-id", "container-id",
                                                          subprocess.CalledProcessError(1, "create"),
                                                          "", ""]) as run:
            docker.network("network-name")
            docker.create("--name", "owned")
            with self.assertRaises(subprocess.CalledProcessError):
                docker.create("--name", "collision")
            self.assertEqual(docker.cleanup(), [])
        removals = [call.args for call in run.call_args_list if "rm" in call.args]
        self.assertEqual(removals, [("container", "rm", "-f", "container-id"),
                                    ("network", "rm", "network-id")])


if __name__ == "__main__":
    unittest.main()
