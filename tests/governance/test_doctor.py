"""Exercise preflight failures without starting Docker or touching user resources."""

import importlib.util
import json
import os
from pathlib import Path
import socket
import subprocess
import tempfile
import unittest
from unittest.mock import patch


ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("doctor", ROOT / "scripts/doctor.py")
doctor = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(doctor)


class DoctorTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="oj-doctor-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        (self.root / ".githooks").mkdir()
        for name in ("pre-commit", "commit-msg", "pre-push"):
            p = self.root / ".githooks" / name
            p.write_text("#!/bin/sh\nexit 0\n")
            p.chmod(0o755)
        self.responses = {
            ("symbolic-ref", "--quiet", "--short", "HEAD"): (0, "codex/example\n"),
            ("rev-parse", "--verify", "refs/remotes/origin/main"): (0, "a" * 40),
            ("merge-base", "--is-ancestor", "origin/main", "HEAD"): (0, ""),
            ("status", "--porcelain=v1", "-z"): (0, ""),
            ("config", "--get", "core.hooksPath"): (0, ".githooks\n"),
            ("info", "--format", doctor.INFO_FORMAT): (0, json.dumps({"OSType": "linux", "NCPU": 4, "MemTotal": 8 * 2**30})),
            ("compose", "version", "--short"): (0, "5.5.1\n"),
        }
        self.commands = []

    def fake_run(self, command, root):
        self.commands.append(command)
        args = tuple(command[1:] if command[0] == "git" else command[3:])
        code, output = self.responses[args]
        return subprocess.CompletedProcess(command, code, output, "sensitive error omitted")

    def inspect(self, port=0):
        with patch.object(doctor, "run", side_effect=self.fake_run), patch.object(doctor.shutil, "which", return_value="/tool"):
            return doctor.inspect(self.root, "Darwin", port)

    def test_healthy_checks_are_read_only_and_context_explicit(self):
        checks = self.inspect()
        self.assertEqual(doctor.exit_code(checks, strict=True), 0)
        for command in self.commands:
            self.assertFalse(set(command) & {"fetch", "switch", "install", "run", "up", "build", "down"})
            if command[0] == "docker":
                self.assertEqual(command[:3], ["docker", "--context", "desktop-linux"])

    def test_branch_hooks_engine_and_compose_failures(self):
        for key, bad in (
            (("symbolic-ref", "--quiet", "--short", "HEAD"), (0, "main")),
            (("symbolic-ref", "--quiet", "--short", "HEAD"), (1, "")),
            (("config", "--get", "core.hooksPath"), (0, "/other/hooks")),
            (("info", "--format", doctor.INFO_FORMAT), (1, "")),
            (("info", "--format", doctor.INFO_FORMAT), (0, "null")),
            (("compose", "version", "--short"), (0, "1.29.2")),
            (("rev-parse", "--verify", "refs/remotes/origin/main"), (128, "")),
        ):
            original = self.responses[key]
            self.responses[key] = bad
            with self.subTest(key=key, bad=bad):
                checks = self.inspect()
                self.assertEqual(doctor.exit_code(checks), 1)
                self.assertNotIn("sensitive error", json.dumps(checks))
            self.responses[key] = original

    def test_dirty_old_base_and_occupied_port_warn_without_repair(self):
        self.responses[("status", "--porcelain=v1", "-z")] = (0, " M user.txt\0")
        self.responses[("merge-base", "--is-ancestor", "origin/main", "HEAD")] = (1, "")
        with socket.socket() as listener:
            listener.bind(("127.0.0.1", 0))
            listener.listen(1)
            checks = self.inspect(listener.getsockname()[1])
        self.assertEqual({c["name"] for c in checks if c["status"] == "WARN"}, {"worktree", "base", "port"})
        self.assertEqual(doctor.exit_code(checks), 0)
        self.assertEqual(doctor.exit_code(checks, strict=True), 1)

    def test_missing_tool_and_timeout_do_not_expose_errors(self):
        with patch.object(doctor, "run", return_value=None), patch.object(doctor.shutil, "which", return_value=None):
            self.assertEqual(doctor.exit_code(doctor.inspect(self.root, "Darwin", 0)), 1)
        with patch.object(doctor.subprocess, "run", side_effect=subprocess.TimeoutExpired("docker", 10)):
            self.assertIsNone(doctor.run(["docker", "info"], self.root))

    def test_invalid_port_is_rejected(self):
        for port in ("0", "65536", "bad"):
            result = subprocess.run([os.sys.executable, str(ROOT / "scripts/doctor.py"), "--port", port], capture_output=True)
            self.assertEqual(result.returncode, 2)


if __name__ == "__main__":
    unittest.main()
