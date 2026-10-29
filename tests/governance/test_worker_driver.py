"""The Desktop harness must not take over another process's sandbox."""
import importlib.util
from pathlib import Path
import unittest
from unittest.mock import Mock, patch

SPEC = importlib.util.spec_from_file_location(
    "worker_driver", Path(__file__).resolve().parents[2] / "scripts/test-worker.py")
DRIVER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(DRIVER)


class OwnershipTests(unittest.TestCase):
    def test_existing_resources_are_not_taken_over(self):
        for label, running in (("someone-else", "false"), (DRIVER.OWNER, "true")):
            with self.subTest(label=label, running=running):
                def docker(*args, **kwargs):
                    if args[0] == "inspect":
                        return label if "Labels" in args[2] else running
                    if args[0] != "build":
                        self.fail("Touched existing resource: " + repr(args))
                with patch.object(DRIVER.platform, "system", return_value="Darwin"), \
                     patch.object(DRIVER.platform, "machine", return_value="arm64"), \
                     patch.object(DRIVER, "docker", side_effect=docker), \
                     patch.object(DRIVER.subprocess, "run", return_value=Mock(returncode=0)), \
                     patch("sys.argv", ["test-worker.py", "sandbox"]):
                    with self.assertRaisesRegex(RuntimeError, "Refusing to take over"):
                        DRIVER.main()
