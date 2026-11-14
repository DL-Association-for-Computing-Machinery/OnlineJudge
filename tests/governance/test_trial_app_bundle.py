"""Exercise application transport and rejection without running bundle files."""

from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest


SCRIPT = Path(__file__).resolve().parents[2] / "scripts/trial_app_bundle.py"
REVISION = "1" * 40


class TrialAppBundleTests(unittest.TestCase):
    def setUp(self):
        root = Path(self.enterContext(tempfile.TemporaryDirectory()))
        self.source, self.bundle, self.received = (root / name for name in ("export", "bundle", "received"))
        (self.source / "bin").mkdir(parents=True)
        (self.source / "site/assets").mkdir(parents=True)
        elf = bytearray(64)
        elf[:7] = b"\x7fELF\x02\x01\x01"
        elf[16:20] = b"\x02\x00\xb7\x00"
        for name in ("api", "worker", "worker-api", "migrate", "materialstore", "mail-worker"):
            (self.source / "bin" / name).write_bytes(elf)
        for name in ("site/index.html", "site/THIRD-PARTY-NOTICES.txt", "site/assets/index-a.js"):
            (self.source / name).write_text("synthetic\n")

    def invoke(self, *args):
        return subprocess.run([sys.executable, "-B", str(SCRIPT), *args],
                              capture_output=True, text=True, timeout=5)

    def pack(self, platform="linux/arm64"):
        return self.invoke("pack", "--export", self.source, "--revision", REVISION,
                           "--platform", platform, self.bundle)

    def receive(self):
        result = self.pack()
        self.assertEqual(result.returncode, 0, result.stderr)
        shutil.copytree(self.bundle, self.received)
        return result.stdout.strip()

    def verify(self, digest, platform="linux/arm64"):
        return self.invoke("verify", "--manifest-sha256", digest, "--platform", platform, self.received)

    def assert_refused(self, result):
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertEqual(result.stdout, "")

    def test_copied_bundle_and_independent_digest(self):
        digest = self.receive()
        result = self.verify(digest)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.strip(), REVISION)
        self.assert_refused(self.verify("0" * 64))
        self.assert_refused(self.verify(digest, "linux/amd64"))
        self.assert_refused(self.pack())
        self.assertEqual(self.verify(digest).returncode, 0)

    def test_same_size_damage_extra_file_and_symlink(self):
        digest = self.receive()
        asset = self.received / "site/assets/index-a.js"
        original = asset.read_bytes()
        asset.write_bytes(original.upper())
        self.assert_refused(self.verify(digest))
        asset.write_bytes(original)
        extra = asset.with_name("extra.js")
        extra.write_bytes(original)
        self.assert_refused(self.verify(digest))
        extra.unlink()
        asset.unlink()
        asset.symlink_to(self.source / "site/assets/index-a.js")
        self.assert_refused(self.verify(digest))

    def test_sender_rejects_wrong_binary_platform_or_local_config(self):
        self.assert_refused(self.pack("linux/amd64"))
        self.assertFalse((self.bundle / "manifest.json").exists())
        (self.source / "site/config.local").write_text("local fixture")
        result = self.invoke("pack", "--export", self.source, "--revision", REVISION,
                             "--platform", "linux/arm64", self.received)
        self.assert_refused(result)
        self.assertFalse(self.received.exists())
