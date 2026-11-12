#!/usr/bin/env python3
"""Seal and verify the fixed Linux arm64 judging experiment payload."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import stat
import tarfile


SANDBOX = "sha256:cd40ffdb32619806a8d97e59fe03e890fe4066835f57efa6ae13f919ef3262c5"
POSTGRES = "sha256:4ea9e5ed06591da7ea23eb65465e8d3187fe79f4d5ec3ae976d29a33b013e77a"
IMAGES = (SANDBOX, POSTGRES)
BINARIES = ("api", "worker", "worker-api", "migrate", "sandbox.test")
SCHEMA = "oj.offline-judge-bundle.v1"
MANIFEST = "manifest.json"
ARCHIVE = "images.tar"
FILES = {ARCHIVE: 0o600, **{f".local/bin/{name}": 0o700 for name in BINARIES}}
DIRECTORIES = (".local", ".local/bin", "tests", "tests/sandbox")
SHA = re.compile(r"[0-9a-f]{64}")
REVISION = re.compile(r"[0-9a-f]{40}")
BLOB = re.compile(r"blobs/sha256/[0-9a-f]{64}")
MAX_ARCHIVE = 2 << 30
MAX_BINARY = 128 << 20
MAX_METADATA = 64 << 10


def unique_pairs(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate metadata key")
        result[key] = value
    return result


def decode(data):
    return json.loads(data, object_pairs_hook=unique_pairs)


def check_image_archive(path):
    """Check untagged Docker save metadata without extracting or importing it."""
    with tarfile.open(path, "r:") as archive:
        items = {}
        for member in archive:
            name = member.name
            if name in items or len(items) >= 256:
                raise ValueError("duplicate or oversized image archive inventory")
            if name in {"blobs", "blobs/sha256"}:
                valid = member.isdir()
            elif name in {"oci-layout", "index.json", "manifest.json"}:
                valid = member.isreg() and 0 < member.size <= MAX_METADATA
            else:
                valid = BLOB.fullmatch(name) and member.isreg() and member.size > 0
            if not valid:
                raise ValueError("unexpected member in the untagged image archive")
            items[name] = member

        def metadata(name):
            if name not in items or items[name].size > MAX_METADATA:
                raise ValueError("missing Docker image archive metadata")
            with archive.extractfile(items[name]) as stream:
                return decode(stream.read(MAX_METADATA + 1))

        def blob(descriptor, small=False):
            digest = descriptor["digest"]
            name = "blobs/" + digest.replace(":", "/")
            item = items.get(name)
            if not BLOB.fullmatch(name) or item is None or item.size != descriptor["size"]:
                raise ValueError("missing or inconsistent pinned image blob")
            with archive.extractfile(item) as stream:
                if "sha256:" + hashlib.file_digest(stream, "sha256").hexdigest() != digest:
                    raise ValueError("pinned image blob digest mismatch")
            return metadata(name) if small else name

        index = metadata("index.json")
        entries = index.get("manifests") if isinstance(index, dict) else None
        if not isinstance(entries, list) or len(entries) != len(IMAGES) or any(
            not isinstance(entry, dict) or entry.get("digest") not in IMAGES
            or not isinstance(entry.get("annotations", {}), dict)
            or set(entry.get("annotations", {})) - {"config.digest", "containerd.io/distribution.source.docker.io"}
            or "blobs/sha256/" + entry["digest"][7:] not in items
            for entry in entries
        ) or {entry["digest"] for entry in entries} != set(IMAGES):
            raise ValueError("expected exactly the two pinned, untagged image descriptors")
        expected = {}
        try:
            for entry in entries:
                image = blob(entry, small=True)
                if "manifests" in image:
                    native = [item for item in image["manifests"] if item.get("platform", {}).get("os") == "linux"
                              and item.get("platform", {}).get("architecture") == "arm64"]
                    if len(native) != 1:
                        raise ValueError("expected one Linux arm64 image manifest")
                    image = blob(native[0], small=True)
                config = blob(image["config"], small=True)
                if (config["os"], config["architecture"]) != ("linux", "arm64"):
                    raise ValueError("offline image has the wrong platform")
                name = "blobs/" + image["config"]["digest"].replace(":", "/")
                expected[name] = [blob(layer) for layer in image["layers"]]
            rows = metadata("manifest.json")
            if not isinstance(rows, list) or len(rows) != len(IMAGES) or any(
                row["RepoTags"] not in (None, []) for row in rows
            ) or {row["Config"]: row["Layers"] for row in rows} != expected:
                raise ValueError("Docker entries differ from the pinned untagged images")
        except (AttributeError, KeyError, TypeError) as exc:
            raise ValueError("invalid pinned image metadata") from exc
        if metadata("oci-layout") != {"imageLayoutVersion": "1.0.0"}:
            raise ValueError("unsupported image archive layout")


def check_mode(path, mode, directory=False):
    info = path.lstat()
    expected = stat.S_IFDIR if directory else stat.S_IFREG
    if info.st_mode != expected | mode or info.st_uid != os.geteuid() or (not directory and info.st_nlink != 1):
        raise ValueError("bundle paths must be private and owned by the current user")
    return info


def measure(path, maximum):
    size = path.stat().st_size
    if not 0 < size <= maximum:
        raise ValueError("bundle file size is outside the supported range")
    with path.open("rb") as stream:
        digest = hashlib.file_digest(stream, "sha256").hexdigest()
    return {"bytes": size, "sha256": digest}


def inventory(root):
    return {name: measure(root / name, MAX_ARCHIVE if name == ARCHIVE else MAX_BINARY) for name in FILES}


def verify(root, trusted):
    if not SHA.fullmatch(trusted):
        raise ValueError("an independently supplied lowercase manifest SHA-256 is required")
    check_mode(root, 0o700, directory=True)
    for name in DIRECTORIES:
        check_mode(root / name, 0o700, directory=True)
    for name, mode in {MANIFEST: 0o600, **FILES}.items():
        check_mode(root / name, mode)
    expected_paths = tuple(Path(name) for name in (MANIFEST, *DIRECTORIES, *FILES))
    for directory in (Path("."), *(Path(name) for name in DIRECTORIES)):
        allowed = {path.name for path in expected_paths if path.parent == directory}
        if any(path.name not in allowed for path in (root / directory).iterdir()):
            raise ValueError("unexpected paths in offline bundle")
    manifest = root / MANIFEST
    if manifest.stat().st_size > MAX_METADATA:
        raise ValueError("offline bundle manifest is too large")
    data = manifest.read_bytes()
    if hashlib.sha256(data).hexdigest() != trusted:
        raise ValueError("offline bundle manifest digest mismatch")
    doc = decode(data)
    if not isinstance(doc, dict) or set(doc) != {"schema", "revision", "platform", "images", "files"} \
            or doc["schema"] != SCHEMA or not isinstance(doc["revision"], str) or not REVISION.fullmatch(doc["revision"]) \
            or doc["platform"] != "linux/arm64" or doc["images"] != list(IMAGES):
        raise ValueError("unsupported offline bundle manifest")
    if doc["files"] != inventory(root):
        raise ValueError("offline bundle file content mismatch")
    check_image_archive(root / ARCHIVE)
    return doc["revision"]


def pack(binaries, archive, revision, output):
    if not REVISION.fullmatch(revision):
        raise ValueError("a full Git revision is required")
    sources = {ARCHIVE: archive, **{f".local/bin/{name}": binaries / name for name in BINARIES}}
    for name, source in sources.items():
        info = source.lstat()
        if not stat.S_ISREG(info.st_mode) or not 0 < info.st_size <= (MAX_ARCHIVE if name == ARCHIVE else MAX_BINARY):
            raise ValueError("bundle input must be a bounded regular file")
    output.mkdir(mode=0o700)
    for name in DIRECTORIES:
        (output / name).mkdir(mode=0o700)
    for name, source in sources.items():
        destination = output / name
        with source.open("rb") as src, destination.open("xb") as dst:
            shutil.copyfileobj(src, dst, length=1 << 20)
        destination.chmod(FILES[name])
    check_image_archive(output / ARCHIVE)
    doc = {"schema": SCHEMA, "revision": revision, "platform": "linux/arm64", "images": list(IMAGES), "files": inventory(output)}
    data = (json.dumps(doc, sort_keys=True, separators=(",", ":")) + "\n").encode()
    with (output / MANIFEST).open("xb") as stream:
        stream.write(data)
    (output / MANIFEST).chmod(0o600)
    return hashlib.sha256(data).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    create = commands.add_parser("pack")
    create.add_argument("--binaries", type=Path, required=True)
    create.add_argument("--images", type=Path, required=True)
    create.add_argument("--revision", required=True)
    check = commands.add_parser("verify")
    check.add_argument("--manifest-sha256", required=True)
    for command in (create, check):
        command.add_argument("bundle", type=Path)
    args = parser.parse_args()
    try:
        if args.command == "pack":
            print(pack(args.binaries, args.images, args.revision, args.bundle))
        else:
            print("Verified offline judging bundle:", verify(args.bundle, args.manifest_sha256))
    except (OSError, ValueError, tarfile.TarError) as exc:
        parser.exit(1, f"offline bundle: {exc}\n")


if __name__ == "__main__":
    main()
