"""Offline acceptance must validate the received bytes before loading images."""
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import sys
import tarfile
import tempfile
import unittest
from unittest.mock import call, patch

SCRIPTS = Path(__file__).resolve().parents[2] / "scripts"


def load(name, file):
    spec = importlib.util.spec_from_file_location(name, SCRIPTS / file)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


BUNDLE = load("offline_judge_bundle", "offline_judge_bundle.py")
with patch.dict(sys.modules, {"offline_judge_bundle": BUNDLE}):
    RUNNER = load("ubuntu_worker", "test-ubuntu-worker.py")


def miniature_archive(path, tagged=False, damage=False, architecture="arm64"):
    def encoded(value):
        return json.dumps(value).encode()

    files = {}

    def add_blob(data):
        digest = hashlib.sha256(data).hexdigest()
        name = "blobs/sha256/" + digest
        files[name] = data
        return name, {"digest": "sha256:" + digest, "size": len(data)}

    rows, descriptors = [], []
    for i in range(2):
        config_name, config = add_blob(encoded({"os": "linux", "architecture": architecture, "example": i}))
        layer_name, layer = add_blob(bytes([i, 1, 2]))
        _, image = add_blob(encoded({"config": config, "layers": [layer]}))
        if i == 1:
            native = {**image, "platform": {"os": "linux", "architecture": "arm64"}}
            _, image = add_blob(encoded({"manifests": [native]}))
        if damage:
            files[layer_name] = bytes([i, 1, 3])
        descriptors.append(image)
        rows.append({"Config": config_name, "Layers": [layer_name], "RepoTags": ["example:test"] if tagged else None})
    files.update({"oci-layout": encoded({"imageLayoutVersion": "1.0.0"}),
                  "index.json": encoded({"manifests": descriptors}), "manifest.json": encoded(rows)})
    with tarfile.open(path, "w") as archive:
        for name, data in files.items():
            member = tarfile.TarInfo(name)
            member.size = len(data)
            archive.addfile(member, io.BytesIO(data))
    return tuple(item["digest"] for item in descriptors)


class OfflineBundleTests(unittest.TestCase):
    def setUp(self):
        root = Path(self.enterContext(tempfile.TemporaryDirectory()))
        self.bundle, self.images, self.bin = root / "bundle", root / "images.tar", root / "bin"
        self.bin.mkdir()
        for name in BUNDLE.BINARIES:
            (self.bin / name).write_bytes(b"test artifact")
        self.enterContext(patch.object(BUNDLE, "IMAGES", miniature_archive(self.images)))
        self.revision = "0" * 39 + "1"
        self.trusted = BUNDLE.pack(self.bin, self.images, self.revision, self.bundle)

    def test_verify_copied_release_and_refuse_existing_output(self):
        self.assertEqual(BUNDLE.verify(self.bundle, self.trusted), self.revision)
        with self.assertRaises(FileExistsError):
            BUNDLE.pack(self.bin, self.images, self.revision, self.bundle)
        self.assertEqual(BUNDLE.verify(self.bundle, self.trusted), self.revision)

    def test_same_size_damage_never_reaches_docker(self):
        artifact = self.bundle / ".local/bin/worker"
        artifact.write_bytes(b"TEST artifact")
        with patch.object(RUNNER, "docker") as docker:
            with self.assertRaisesRegex(ValueError, "file content mismatch"):
                RUNNER.load_bundle(self.bundle, self.trusted)
            docker.assert_not_called()

    def test_cold_reception_does_not_import_into_existing_store(self):
        with patch.object(RUNNER, "docker", return_value=RUNNER.IMAGES[0]) as docker:
            with self.assertRaisesRegex(ValueError, "requires an empty Docker image store"):
                RUNNER.load_bundle(self.bundle, self.trusted, require_empty=True)
        docker.assert_called_once_with("image", "ls", "--all", "--no-trunc", "--format", "{{.ID}}")

    def test_cold_reception_accepts_only_the_two_imported_images(self):
        listing = call("image", "ls", "--all", "--no-trunc", "--format", "{{.ID}}")
        for images in (RUNNER.IMAGES[:1], (*RUNNER.IMAGES, "sha256:" + "f" * 64), RUNNER.IMAGES):
            with self.subTest(images=images), patch.object(RUNNER, "docker", side_effect=[
                "", "", "\n".join(images), *(image + " linux/arm64" for image in RUNNER.IMAGES),
            ]) as docker:
                if images == RUNNER.IMAGES:
                    RUNNER.load_bundle(self.bundle, self.trusted, require_empty=True)
                    self.assertEqual(docker.call_count, 5)
                else:
                    with self.assertRaisesRegex(ValueError, "inventory differs"):
                        RUNNER.load_bundle(self.bundle, self.trusted, require_empty=True)
                    self.assertEqual(docker.call_count, 3)
                self.assertEqual(docker.call_args_list[:3], [listing,
                    call("image", "load", "--input", str(self.bundle / BUNDLE.ARCHIVE), timeout=180), listing])

    def test_source_digest_is_required_and_links_or_extra_files_refused(self):
        with self.assertRaisesRegex(ValueError, "manifest digest mismatch"):
            BUNDLE.verify(self.bundle, "a" * 64)
        extra = self.bundle / "extra"
        extra.touch()
        with self.assertRaisesRegex(ValueError, "unexpected paths"):
            BUNDLE.verify(self.bundle, self.trusted)
        extra.unlink()
        target = self.bundle / ".local/bin/api"
        target.unlink()
        target.symlink_to(self.bin / "api")
        with self.assertRaisesRegex(ValueError, "private and owned"):
            BUNDLE.verify(self.bundle, self.trusted)
        target.unlink()
        (self.bin / "api").chmod(0o700)
        os.link(self.bin / "api", target)
        with self.assertRaisesRegex(ValueError, "private and owned"):
            BUNDLE.verify(self.bundle, self.trusted)

    def test_builder_refuses_tagged_archive_before_sealing(self):
        miniature_archive(self.images, tagged=True)
        invalid = self.bundle.with_name("invalid")
        with self.assertRaisesRegex(ValueError, "pinned untagged images"):
            BUNDLE.pack(self.bin, self.images, self.revision, invalid)
        self.assertFalse((invalid / BUNDLE.MANIFEST).exists())

    def test_builder_refuses_damaged_pinned_image_layer(self):
        miniature_archive(self.images, damage=True)
        invalid = self.bundle.with_name("damaged-image")
        with self.assertRaisesRegex(ValueError, "pinned image blob digest mismatch"):
            BUNDLE.pack(self.bin, self.images, self.revision, invalid)
        self.assertFalse((invalid / BUNDLE.MANIFEST).exists())

    def test_builder_refuses_wrong_native_platform(self):
        invalid = self.bundle.with_name("wrong-platform")
        with patch.object(BUNDLE, "IMAGES", miniature_archive(self.images, architecture="amd64")):
            with self.assertRaisesRegex(ValueError, "wrong platform"):
                BUNDLE.pack(self.bin, self.images, self.revision, invalid)
        self.assertFalse((invalid / BUNDLE.MANIFEST).exists())
