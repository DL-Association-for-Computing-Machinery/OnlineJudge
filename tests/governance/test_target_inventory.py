"""Keep unavailable Linux resource evidence distinct from visible limits."""

import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch


SCRIPT = Path(__file__).resolve().parents[2] / "scripts/target-inventory.py"
SPEC = importlib.util.spec_from_file_location("inventory", SCRIPT)
inventory = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(inventory)


class InventoryTests(unittest.TestCase):
    def test_cgroup_mount_root_and_ancestors(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp).resolve()
            leaf = root / "worker"
            leaf.mkdir()
            (root / "memory.max").write_text("1073741824\n")
            (leaf / "memory.max").write_text("2147483648\n")
            (leaf / "pids.max").write_text("max\n")
            (leaf / "cpu.max").write_text("bad\n")
            (root / "pids.max").write_text("0\n")
            actual_read = inventory.read

            def read(path):
                if path == Path("/proc/self/cgroup"):
                    return "0::/service/worker\n"
                if path == Path("/proc/self/mountinfo"):
                    return f"21 3 0:4 /service {root} ro - cgroup2 cgroup ro\n"
                return actual_read(path)

            with patch.object(inventory, "CG", root), patch.object(inventory, "read", read):
                self.assertEqual(inventory.cgroup_dir(), leaf)
                child, parent = inventory.constraints(leaf)
                self.assertEqual(child["memory.max"], {"state": "bounded", "limit": 2147483648})
                self.assertEqual(parent["memory.max"], {"state": "bounded", "limit": 1073741824})
                self.assertEqual(child["pids.max"], {"state": "unbounded"})
                self.assertEqual(parent["pids.max"], {"state": "bounded", "limit": 0})
                self.assertEqual(child["cpu.max"], {"state": "invalid"})
                self.assertEqual(parent["cpu.max"], {"state": "unavailable"})

    def test_unrecognized_hierarchy_is_not_unbounded(self):
        with patch.object(inventory, "read", return_value="2:cpu:/scope\n"):
            self.assertIsNone(inventory.cgroup_dir())

    def test_missing_and_permission_denied_volumes_are_unavailable(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp)
            self.assertEqual(inventory.volume(path / "missing"), {"state": "unavailable"})
            with patch.object(Path, "is_dir", side_effect=PermissionError):
                self.assertEqual(inventory.volume(path), {"state": "unavailable"})
            self.assertFalse((path / "missing").exists())

    @unittest.skipUnless(sys.platform.startswith("linux"), "Linux inventory")
    def test_cli_reports_volume_roles_without_input_paths(self):
        with tempfile.TemporaryDirectory() as tmp:
            missing = str(Path(tmp) / "uncreated")
            cmd = [sys.executable, "-B", str(SCRIPT), "--materials-dir", tmp,
                   "--scratch-dir", tmp, "--database-dir", missing]
            result = subprocess.run(cmd, capture_output=True, check=True, text=True, timeout=10)
            out = json.loads(result.stdout)
            self.assertEqual(out["schema"], "oj.target-inventory.v1")
            self.assertEqual(out["volumes"]["database"], {"state": "unavailable"})
            self.assertGreater(out["volumes"]["materials"]["total_bytes"], 0)
            self.assertNotIn(tmp, result.stdout)
            self.assertFalse(Path(missing).exists())
