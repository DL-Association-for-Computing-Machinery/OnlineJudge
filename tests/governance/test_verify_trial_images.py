"""The receiving verifier must not trust a matching tar digest alone."""

import hashlib
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

from test_trial_app_bundle import REVISION, app_export


SCRIPT = Path(__file__).resolve().parents[2] / "scripts/verify_trial_images.py"
MEDIA = "application/vnd.oci.image.manifest.v1+json"


class TrialImagesTests(unittest.TestCase):
    def setUp(self):
        self.archive = Path(self.enterContext(tempfile.TemporaryDirectory())) / "runtime.tar"

    def export(self, *, tagged=False, damaged=False, extra=False, duplicate=False, alternate_layers=False, generation=1):
        files, rows, entries, roles = {}, [], [], {}

        def blob(raw, media=""):
            digest = hashlib.sha256(raw).hexdigest()
            files["blobs/sha256/" + digest] = raw
            return {"digest": "sha256:" + digest, "size": len(raw), "mediaType": media}

        for role in ("nginx", "postgres", "redis", *(("unexpected",) if extra else ())):
            config = blob(json.dumps({"os": "linux", "architecture": "arm64", "fixture": role,
                                      "generation": generation}).encode())
            layer = blob(role.encode(), "application/vnd.oci.image.layer.v1.tar")
            manifest = {"schemaVersion": 2, "config": config, "layers": [layer]}
            entries.append(blob(json.dumps(manifest).encode(), MEDIA))
            config_path = config["digest"].replace(":", "/")
            layer_path = layer["digest"].replace(":", "/")
            rows.append({"Config": "blobs/" + config_path, "Layers": ["blobs/" + layer_path],
                         "RepoTags": ["example:tag"] if tagged else None})
            if damaged and role == "redis":
                files["blobs/" + layer_path] = b"REDIS"
            if role != "unexpected":
                roles[role] = config["digest"]
        if alternate_layers:
            rows[0]["LayerSources"] = {}
        files["oci-layout"] = b'{"imageLayoutVersion":"1.0.0"}'
        files["index.json"] = json.dumps({"schemaVersion": 2, "manifests": entries}).encode()
        files["manifest.json"] = json.dumps(rows).encode()
        with tarfile.open(self.archive, "w") as archive:
            members = list(files.items())
            if duplicate:
                members.append(("manifest.json", files["manifest.json"]))
            for name, raw in members:
                info = tarfile.TarInfo(name)
                info.size = len(raw)
                archive.addfile(info, io.BytesIO(raw))
        return roles

    def verify(self, roles, *, platform="linux/arm64", digest=None):
        cmd = [sys.executable, "-B", str(SCRIPT), str(self.archive), "--platform", platform,
               "--sha256", digest or hashlib.sha256(self.archive.read_bytes()).hexdigest()]
        for role, value in roles.items():
            cmd += ["--" + role, value]
        return subprocess.run(cmd, capture_output=True, text=True, timeout=5)

    def test_expected_configurations_and_platform(self):
        roles = self.export()
        result = self.verify(roles)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout)["configuration_digests"], roles)
        for bad in (self.verify(roles, platform="linux/amd64"), self.verify(roles, digest="0" * 64),
                    self.verify({**roles, "nginx": "sha256:" + "0" * 64})):
            self.assertEqual(bad.returncode, 1)
            self.assertEqual(bad.stdout, "")

    def test_valid_outer_digest_cannot_authorize_bad_archive_inventory(self):
        for option in ("tagged", "damaged", "extra", "duplicate", "alternate_layers"):
            with self.subTest(option=option):
                roles = self.export(**{option: True})
                result = self.verify(roles)
                self.assertEqual(result.returncode, 1)
                self.assertEqual(result.stdout, "")

    def test_cold_receiver_refuses_warm_engine_before_import(self):
        with patch.object(sys, "path", [str(SCRIPT.parent), *sys.path]):
            spec = importlib.util.spec_from_file_location("cold_trial_images", SCRIPT.with_name("cold_trial_images.py"))
            module = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(module)
        roles = self.export()
        root = self.archive.parent / "engine"
        socket = self.archive.parent / "engine.sock"
        digest = hashlib.sha256(self.archive.read_bytes()).hexdigest()
        with (patch.object(Path, "lstat") as lstat, patch.object(module.subprocess, "run") as run):
            lstat.return_value.st_mode = 0o140000
            run.side_effect = [subprocess.CompletedProcess([], 0, str(root), ""),
                               subprocess.CompletedProcess([], 0, roles["nginx"], "")]
            with self.assertRaisesRegex(ValueError, "must start with empty"):
                module.receiver(socket, root, self.archive, digest, "linux/arm64", roles)
            self.assertEqual([call.args[0][3:5] for call in run.call_args_list],
                             [("info", "--format"), ("image", "ls")])

    def test_release_rejects_valid_artifacts_from_another_release(self):
        root = self.archive.parent
        app_source = root / "source"
        app = root / "app"
        other_app = root / "other-app"
        release = root / "release.json"
        app_export(app_source)

        def cli(script, *args):
            return subprocess.run([sys.executable, "-B", str(SCRIPT.with_name(script)), *args],
                                  capture_output=True, text=True, timeout=5)

        def seal_app(revision, target):
            result = cli("trial_app_bundle.py", "pack", "--export", app_source, "--revision", revision,
                         "--platform", "linux/arm64", target)
            self.assertEqual(result.returncode, 0, result.stderr)
            return result.stdout.strip()

        def seal_release(path, app_pin):
            runtime_sha = hashlib.sha256(self.archive.read_bytes()).hexdigest()
            return cli("trial_release.py", "pack", path, "--app", app, "--runtime", self.archive,
                       "--revision", REVISION, "--platform", "linux/arm64", "--app-sha256", app_pin,
                       "--runtime-sha256", runtime_sha, "--nginx", roles["nginx"], "--postgres", roles["postgres"],
                       "--redis", roles["redis"])

        roles = self.export()
        app_pin = seal_app(REVISION, app)
        result = seal_release(release, app_pin)
        self.assertEqual(result.returncode, 0, result.stderr)
        pin = result.stdout.strip()

        def verify(path=release, bundle=app, trusted=pin):
            return cli("trial_release.py", "verify", path, "--app", bundle, "--runtime", self.archive,
                       "--sha256", trusted)

        result = verify()
        self.assertEqual(result.returncode, 0, result.stderr)
        report = json.loads(result.stdout)
        self.assertEqual(report["release_sha256"], pin)
        self.assertEqual(report["revision"], REVISION)
        self.assertEqual(report["image_configurations"], roles)
        self.assertEqual(seal_release(release, app_pin).returncode, 1)
        self.assertEqual(verify(trusted="0" * 64).returncode, 1)
        seal_app("2" * 40, other_app)
        self.assertEqual(verify(bundle=other_app).returncode, 1)
        self.assertEqual(self.verify(self.export(generation=2)).returncode, 0)
        result = verify()
        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")

    def test_release_pack_rejects_revision_conflict_before_writing(self):
        roles = self.export()
        root = self.archive.parent
        source, app, release = (root / name for name in ("source", "app", "release.json"))
        app_export(source)
        result = subprocess.run([sys.executable, "-B", str(SCRIPT.with_name("trial_app_bundle.py")), "pack",
                                 "--export", source, "--revision", REVISION, "--platform", "linux/arm64", app],
                                capture_output=True, text=True, timeout=5, check=True)
        result = subprocess.run(
            [sys.executable, "-B", str(SCRIPT.with_name("trial_release.py")), "pack", release, "--app", app,
             "--runtime", self.archive, "--platform", "linux/arm64", "--revision", "2" * 40,
             "--app-sha256", result.stdout.strip(), "--runtime-sha256", hashlib.sha256(self.archive.read_bytes()).hexdigest(),
             *[arg for role, digest in roles.items() for arg in ("--" + role, digest)]],
            capture_output=True, text=True, timeout=5)
        self.assertEqual(result.returncode, 1)
        self.assertFalse(release.exists())
