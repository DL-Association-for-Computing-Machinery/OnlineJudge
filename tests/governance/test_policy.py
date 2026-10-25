"""Exercise observable gates in disposable Git repositories, never this index."""

import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest


SOURCE = Path(__file__).resolve().parents[2]


class PolicyTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="oj-policy-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.env = dict(os.environ, GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull)
        for key in ("GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR"):
            self.env.pop(key, None)
        self.run_git("init", "--template=", "-b", "codex/test")
        self.run_git("config", "user.name", "Governance Test")
        self.run_git("config", "user.email", "test@example.invalid")

    def command(self, *args, stdin=None):
        return subprocess.run(args, cwd=self.root, env=self.env, input=stdin,
                              text=True, capture_output=True, check=False)

    def run_git(self, *args):
        result = self.command("git", *args)
        self.assertEqual(result.returncode, 0, result.stderr)
        return result.stdout

    def policy(self, command, *args, stdin=None, succeeds=True):
        result = self.command(sys.executable, "-B", str(SOURCE / "scripts/repo_policy.py"),
                              command, *args, stdin=stdin)
        self.assertEqual(result.returncode == 0, succeeds, result.stdout + result.stderr)
        return result

    def write(self, name, content):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content, encoding="utf-8")
        return path

    def install_fixture(self):
        shutil.copytree(SOURCE / "scripts", self.root / "scripts")
        shutil.copytree(SOURCE / ".githooks", self.root / ".githooks")

    def test_clean_repository_and_bootstrap(self):
        self.write("README.md", "# Project\n")
        self.policy("repository")
        self.policy("bootstrap-ci")

    def test_main_and_master_block_unborn_commit(self):
        for branch in ("main", "master"):
            self.run_git("symbolic-ref", "HEAD", "refs/heads/" + branch)
            self.policy("staged", succeeds=False)

    def test_task_branch_and_detached_head(self):
        self.write("a.txt", "good\n")
        self.run_git("add", "a.txt")
        self.policy("staged")
        self.run_git("commit", "-m", "test: seed disposable repository")
        self.run_git("checkout", "--detach")
        self.policy("staged", succeeds=False)

    def test_staged_secret_cannot_be_hidden_by_worktree_fix(self):
        payload = "-----BEGIN " + "PRIVATE KEY-----\nexample-placeholder\n"
        self.write("config.txt", payload)
        self.run_git("add", "config.txt")
        self.write("config.txt", "fixed worktree\n")
        result = self.policy("staged", succeeds=False)
        self.assertNotIn("example-placeholder", result.stderr)
        self.policy("repository")

    def test_worktree_secret_does_not_change_staged_snapshot(self):
        self.write("a.txt", "clean\n")
        self.run_git("add", "a.txt")
        self.write("a.txt", "-----BEGIN " + "RSA PRIVATE KEY-----\n")
        self.policy("staged")
        self.policy("repository", succeeds=False)

    def test_ignored_files_not_read_but_forced_secret_rejected(self):
        self.write(".gitignore", ".env\n")
        self.write(".env", "example placeholder\n")
        self.policy("repository")
        self.run_git("add", "-f", ".env")
        self.policy("staged", succeeds=False)

    def test_example_environment_allowed(self):
        self.write(".env.example", "DATABASE_URL=placeholder\n")
        self.policy("repository")

    def test_local_configuration_and_build_artifacts_rejected(self):
        for name in ("secrets.local.json", "cert.pem", "dist/bundle.txt", "node_modules/a.txt"):
            with self.subTest(name=name):
                path = self.write(name, "placeholder\n")
                self.policy("repository", succeeds=False)
                path.unlink()

    def test_large_file_rejected(self):
        self.write("big.txt", "a" * (1024 * 1024 + 1))
        self.policy("repository", succeeds=False)
        self.run_git("add", "big.txt")
        self.policy("staged", succeeds=False)

    def test_conflict_markers_rejected(self):
        self.write("a.txt", "<<<<<<< ours\nconflict\n=======\nother\n>>>>>>> theirs\n")
        self.policy("repository", succeeds=False)

    def test_whitespace_rejected_in_index(self):
        self.write("a.txt", "trailing space \n")
        self.run_git("add", "a.txt")
        self.policy("staged", succeeds=False)

    def test_link_target_must_exist_in_proposed_snapshot(self):
        self.write("README.md", "[Guide](docs/guide.md#section)\n")
        self.write("docs/guide.md", "# Section\n")
        self.run_git("add", "README.md")
        self.policy("repository")
        self.policy("staged", succeeds=False)
        self.run_git("add", "docs/guide.md")
        self.policy("staged")

    def test_target_deletion_breaks_unchanged_document(self):
        self.write("README.md", "[Guide](guide.md)\n")
        self.write("guide.md", "# Guide\n")
        self.run_git("add", ".")
        self.run_git("commit", "-m", "docs: seed")
        self.run_git("rm", "guide.md")
        self.policy("staged", succeeds=False)

    def test_external_links_and_anchor_only_are_not_fetched(self):
        self.write("README.md", "[web](https://example.invalid/a) [anchor](#a) [local](</tmp/a b>)\n")
        self.policy("repository")

    def test_relative_escape_and_symlink_rejected(self):
        self.write("README.md", "[outside](../outside.md)\n")
        self.policy("repository", succeeds=False)
        self.write("README.md", "# Fine\n")
        (self.root / "outside").symlink_to("/tmp")
        self.policy("repository", succeeds=False)
        self.run_git("add", "outside")
        self.policy("staged", succeeds=False)

    def test_conventional_commit_titles(self):
        for title in ("feat(auth): add login", "fix!: change contract", "docs: 补充说明", "ci: test policy"):
            path = self.write("message", title + "\n\n说明\n")
            self.policy("commit-message", str(path))
        for title in ("update things", "feat:", "feat: ", "feat(scope): " + "x" * 100, ""):
            path = self.write("message", title + "\n")
            self.policy("commit-message", str(path), succeeds=False)

    def test_push_checks_destination_including_deletion(self):
        sha = "1" * 40
        zero = "0" * 40
        for target in ("main", "master"):
            self.policy("pre-push", stdin=f"refs/heads/codex/task {sha} refs/heads/{target} {zero}\n", succeeds=False)
            self.policy("pre-push", stdin=f"(delete) {zero} refs/heads/{target} {sha}\n", succeeds=False)
        self.policy("pre-push", stdin=f"refs/heads/codex/task {sha} refs/heads/codex/task {zero}\n")
        self.policy("pre-push", stdin="malformed\n", succeeds=False)

    def test_pr_title_rejects_missing_invalid_or_multiline_values(self):
        self.env.pop("PR_TITLE", None)
        self.policy("pr-title", succeeds=False)
        for title in ("update things", "feat: ", "feat: ok\nextra", "feat: " + "x" * 100):
            self.env["PR_TITLE"] = title
            self.policy("pr-title", succeeds=False)
        self.env["PR_TITLE"] = "feat(submission): persist receipts"
        self.policy("pr-title")

    def test_pr_title_shell_characters_are_data(self):
        self.env["PR_TITLE"] = "docs: $(touch marker) `touch marker`"
        self.policy("pr-title")
        self.assertFalse((self.root / "marker").exists())

    def test_application_requires_real_ci(self):
        for name in ("go.mod", "cmd/api/main.go", "frontend/package.json", "web/App.tsx", "app.py"):
            with self.subTest(name=name):
                path = self.write(name, "placeholder\n")
                self.policy("bootstrap-ci", succeeds=False)
                path.unlink()

    def test_application_workflow_runs_real_checks(self):
        # Guard the bootstrap transition: code must not replace the fail-closed
        # gate with skip-on-missing manifests or a successful placeholder job.
        workflow = (SOURCE / ".github/workflows/governance.yml").read_text()
        self.assertNotIn("repo_policy.py bootstrap-ci", workflow)
        for step in ("name: backend", "name: frontend", "make go-check go-build",
                     "make web-check", "--frozen-lockfile"):
            self.assertIn(step, workflow)
        makefile = (SOURCE / "Makefile").read_text()
        for command in ("go vet ./...", "go test -race ./...", "./cmd/api",
                        "./cmd/worker", "web lint", "web typecheck", "web test", "web build"):
            self.assertIn(command, makefile)
        self.assertNotIn("--if-present", workflow + makefile)
        self.assertNotIn("continue-on-error", workflow)

    def test_installer_preserves_existing_configuration(self):
        self.install_fixture()
        self.run_git("config", "core.hooksPath", "custom-hooks")
        result = self.command("sh", "scripts/install-hooks.sh")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.run_git("config", "--get", "core.hooksPath").strip(), "custom-hooks")

    def test_installer_preserves_active_default_hook(self):
        self.install_fixture()
        path = self.write(".git/hooks/pre-commit", "#!/bin/sh\nexit 0\n")
        path.chmod(0o755)
        result = self.command("sh", "scripts/install-hooks.sh")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(path.read_text(), "#!/bin/sh\nexit 0\n")

    def test_installed_hooks_enforce_real_git_commits(self):
        self.install_fixture()
        for _ in range(2):
            result = self.command("sh", "scripts/install-hooks.sh")
            self.assertEqual(result.returncode, 0, result.stderr)
        self.run_git("add", ".")
        invalid = self.command("git", "commit", "-m", "invalid")
        self.assertNotEqual(invalid.returncode, 0)
        self.run_git("commit", "-m", "test: install verified governance hooks")
        self.run_git("checkout", "-b", "main")
        self.write("a.txt", "content\n")
        self.run_git("add", "a.txt")
        blocked = self.command("git", "commit", "-m", "docs: forbidden main commit")
        self.assertNotEqual(blocked.returncode, 0)
        self.assertIn("main/master", blocked.stderr)

    def test_installed_push_hook_checks_actual_local_git_push(self):
        self.install_fixture()
        installed = self.command("sh", "scripts/install-hooks.sh")
        self.assertEqual(installed.returncode, 0, installed.stderr)
        self.run_git("add", ".")
        self.run_git("commit", "-m", "test: seed push verification")
        with tempfile.TemporaryDirectory(prefix="oj-policy-remote-") as destination:
            self.run_git("init", "--bare", "--template=", destination)
            self.run_git("push", destination, "HEAD:refs/heads/codex/test")
            blocked = self.command("git", "push", destination, "HEAD:refs/heads/main")
            self.assertNotEqual(blocked.returncode, 0)
            self.assertIn("main/master", blocked.stderr)
            absent = self.command("git", "--git-dir", destination, "show-ref", "--verify", "refs/heads/main")
            self.assertNotEqual(absent.returncode, 0)


if __name__ == "__main__":
    unittest.main()
