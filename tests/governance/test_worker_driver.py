"""The Desktop harness must not take over another process's sandbox."""
import importlib.util
import argparse
import json
from pathlib import Path
import unittest
import tempfile
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


class BrowserArgumentsTests(unittest.TestCase):
    def test_browser_options_are_not_silently_ignored(self):
        args = argparse.Namespace(suite="all", playwright_module=Path("unused"), chromium_executable=None, output_dir=None)
        with self.assertRaisesRegex(ValueError, "require browser-loop"):
            DRIVER.browser_environment(args)

    def test_browser_requires_existing_tools_and_a_new_external_output(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            module = root / "module"
            module.mkdir()
            (module / "package.json").write_text(json.dumps({"name": "playwright"}))
            node = root / "node"
            node.write_text("synthetic existing executable")
            node.chmod(0o700)
            args = argparse.Namespace(suite="browser-loop", playwright_module=module, chromium_executable=None, output_dir=root / "output")
            with patch.object(DRIVER.shutil, "which", return_value=None):
                with self.assertRaisesRegex(ValueError, "existing host Node"):
                    DRIVER.browser_environment(args)
            with patch.object(DRIVER.shutil, "which", return_value=str(node)):
                env = DRIVER.browser_environment(args)
                self.assertEqual(set(env), {"OJ_PLAYWRIGHT_MODULE", "OJ_BROWSER_NODE", "OJ_BROWSER_OUTPUT"})
                self.assertEqual(args.output_dir.stat().st_mode & 0o777, 0o700)
                with self.assertRaisesRegex(ValueError, "new directory"):
                    DRIVER.browser_environment(args)
                args.output_dir = DRIVER.ROOT / "must-not-create-browser-output"
                with self.assertRaisesRegex(ValueError, "outside this repository"):
                    DRIVER.browser_environment(args)
