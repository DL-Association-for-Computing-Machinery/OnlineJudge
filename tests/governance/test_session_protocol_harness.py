"""Safety checks for the session protocol Docker harness without a Docker daemon."""
import importlib.util
from pathlib import Path
import unittest
from unittest.mock import Mock

SCRIPT = Path(__file__).resolve().parents[2] / "scripts/test-session-protocol.py"
spec = importlib.util.spec_from_file_location("session_protocol_harness", SCRIPT)
harness = importlib.util.module_from_spec(spec)
spec.loader.exec_module(harness)


class HarnessTests(unittest.TestCase):
    def test_cleanup_only_removes_matching_owner(self):
        docker = harness.Docker("default", "mine")
        docker.containers = ["mine", "other"]
        docker.networks = ["net"]

        def run(*args, **_kwargs):
            if args[1] == "inspect":
                owner = "other" if args[-1] == "other" else "mine"
                return '{"oj.session-protocol.owner":"' + owner + '"}'
            return ""

        docker.run = Mock(side_effect=run)
        errors = docker.cleanup()
        self.assertEqual(len(errors), 1)
        self.assertIn("owner label mismatch", errors[0])
        calls = [call.args for call in docker.run.call_args_list]
        self.assertIn(("container", "rm", "-f", "mine"), calls)
        self.assertIn(("network", "rm", "net"), calls)
        self.assertNotIn(("container", "rm", "-f", "other"), calls)

    def test_nonzero_and_oom_are_failures(self):
        docker = Mock()
        for state in ('{"Running":false,"OOMKilled":false,"ExitCode":1}',
                      '{"Running":false,"OOMKilled":true,"ExitCode":0}'):
            docker.run.return_value = state
            with self.assertRaises(RuntimeError):
                harness.check_state(docker, "test", False)
        docker.run.return_value = '{"Running":false,"OOMKilled":false,"ExitCode":0}'
        harness.check_state(docker, "test", False)

    def test_explicit_default_context_is_not_inherited(self):
        self.assertEqual(harness.Docker("default", "x").command, ["docker", "--context", "default"])
        self.assertEqual(harness.Docker("", "x").command, ["docker"])

    def test_shell_copies_only_required_sources_and_does_not_print_pg_logs(self):
        script = SCRIPT.with_suffix(".sh").read_text()
        copy_lines = [line.strip() for line in script.splitlines() if line.startswith("cp ")]
        self.assertEqual(copy_lines, [
            'cp /workspace/go.mod /workspace/go.sum "$oj_protocol_dir/workspace/"',
            'cp -R /workspace/db /workspace/internal /workspace/tests "$oj_protocol_dir/workspace/"',
        ])
        self.assertNotIn("cat ", script)
        self.assertIn("golangci-lint run --config /workspace/.golangci.yml --build-tags=integration", script)
        self.assertIn('Protocol test or PostgreSQL setup failed (exit $status).', script)


if __name__ == "__main__":
    unittest.main()
