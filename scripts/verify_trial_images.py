#!/usr/bin/env python3
"""Verify a trial Docker archive without extracting or importing any image."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import sys
import tarfile

from offline_judge_bundle import BLOB, SHA, decode


ROLES = {"nginx", "postgres", "redis"}
OCI_MANIFEST = "application/vnd.oci.image.manifest.v1+json"
DOCKER_MANIFEST = "application/vnd.docker.distribution.manifest.v2+json"
SUBJECT = "io.containerd.manifest.subject"
RUNTIME_LAYERS = {
    "application/vnd.oci.image.layer.v1.tar",
    "application/vnd.oci.image.layer.v1.tar+gzip",
    "application/vnd.oci.image.layer.v1.tar+zstd",
    "application/vnd.docker.image.rootfs.diff.tar.gzip",
}
MAX_ARCHIVE = 2 << 30
MAX_METADATA = 128 << 10


def blob_path(digest):
    if not isinstance(digest, str) or not digest.startswith("sha256:") or not SHA.fullmatch(digest[7:]):
        raise ValueError("invalid blob digest")
    return "blobs/sha256/" + digest[7:]


def open_archive(path):
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    stream = os.fdopen(fd, "rb")
    info = os.fstat(fd)
    if not stat.S_ISREG(info.st_mode) or not 0 < info.st_size <= MAX_ARCHIVE:
        stream.close()
        raise ValueError("archive must be a bounded regular file")
    return stream


def verify(path, archive_sha, platform, roles):
    if not SHA.fullmatch(archive_sha):
        raise ValueError("expected trusted archive SHA-256")
    if set(roles) not in (ROLES, ROLES | {"sandbox"}) or len(set(roles.values())) != len(roles):
        raise ValueError("expected distinct configuration digests for the runtime roles")
    expected = {blob_path(digest): role for role, digest in roles.items()}
    with open_archive(path) as stream:
        if hashlib.file_digest(stream, "sha256").hexdigest() != archive_sha:
            raise ValueError("archive digest mismatch")
        stream.seek(0)
        with tarfile.open(fileobj=stream, mode="r:") as archive:
            check_archive(archive, platform, expected)
    return {"platform": platform, "archive_sha256": archive_sha, "configuration_digests": roles}


def check_archive(archive, platform, expected):
    items = {}
    for item in archive:
        name = item.name
        if name in items or len(items) >= 512:
            raise ValueError("duplicate or excessive archive entries")
        if name in {"blobs", "blobs/sha256"}:
            valid = item.isdir()
        elif name in {"index.json", "manifest.json", "oci-layout"}:
            valid = item.isreg() and 0 < item.size <= MAX_METADATA
        else:
            valid = BLOB.fullmatch(name) and item.isreg() and not item.issparse() and item.size > 0
        if not valid:
            raise ValueError("unexpected archive member")
        items[name] = item

    def metadata(name):
        item = items[name]
        if item.size > MAX_METADATA:
            raise ValueError("oversized metadata")
        with archive.extractfile(item) as part:
            return decode(part.read(MAX_METADATA + 1))

    seen = set()

    def blob(desc):
        path = blob_path(desc["digest"])
        item = items[path]
        if type(desc["size"]) is not int or item.size != desc["size"]:
            raise ValueError("invalid blob size")
        if path not in seen:
            with archive.extractfile(item) as part:
                if hashlib.file_digest(part, "sha256").hexdigest() != desc["digest"][7:]:
                    raise ValueError("blob digest mismatch")
            seen.add(path)
        return path

    if metadata("oci-layout") != {"imageLayoutVersion": "1.0.0"}:
        raise ValueError("unsupported OCI layout")
    index = metadata("index.json")
    entries = index["manifests"]
    if index["schemaVersion"] != 2 or not isinstance(entries, list) or not 0 < len(entries) <= 32:
        raise ValueError("unsupported archive index")
    runtime, attachments, descriptors = {}, [], set()
    for desc in entries:
        manifest_path = blob(desc)
        if manifest_path in descriptors or desc["mediaType"] not in (OCI_MANIFEST, DOCKER_MANIFEST):
            raise ValueError("unexpected index descriptor")
        descriptors.add(manifest_path)
        annotations = desc.get("annotations", {})
        if {"org.opencontainers.image.ref.name", "io.containerd.image.name"} & annotations.keys():
            raise ValueError("archive must be untagged")
        manifest = metadata(manifest_path)
        if manifest["schemaVersion"] != 2 or not isinstance(manifest["layers"], list) or not manifest["layers"]:
            raise ValueError("unsupported image manifest")
        config_path = blob(manifest["config"])
        config = metadata(config_path)
        if SUBJECT in annotations:
            if (config["os"], config["architecture"]) != ("unknown", "unknown") or not all(
                layer["mediaType"] == "application/vnd.in-toto+json" for layer in manifest["layers"]
            ):
                raise ValueError("unexpected attachment configuration or layers")
            attachments.append(blob_path(annotations[SUBJECT]))
            for layer in manifest["layers"]:
                blob(layer)
            continue
        if (config["os"] + "/" + config["architecture"] != platform
                or config_path not in expected or config_path in runtime
                or not all(layer["mediaType"] in RUNTIME_LAYERS for layer in manifest["layers"])):
            raise ValueError("unexpected runnable image or platform")
        runtime[config_path] = (manifest_path, [blob(layer) for layer in manifest["layers"]])

    if set(runtime) != set(expected) or not set(attachments) <= {item[0] for item in runtime.values()}:
        raise ValueError("runtime or attachment inventory mismatch")
    rows = metadata("manifest.json")
    if not isinstance(rows, list) or len(rows) != len(runtime) or any(
        not isinstance(row, dict) or set(row) != {"Config", "RepoTags", "Layers"}
        or row["RepoTags"] not in (None, []) for row in rows
    ) or {row["Config"]: row["Layers"] for row in rows} != {key: val[1] for key, val in runtime.items()}:
        raise ValueError("Docker import inventory mismatch")
    if {name for name in items if BLOB.fullmatch(name)} != seen:
        raise ValueError("unreferenced archive blobs")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("--sha256", required=True, help="Archive digest from an independent trusted channel")
    parser.add_argument("--platform", choices=("linux/arm64", "linux/amd64"), required=True)
    for role in sorted(ROLES | {"sandbox"}):
        parser.add_argument("--" + role, required=role in ROLES, help="Expected sha256 image configuration digest")
    args = parser.parse_args()
    roles = {role: getattr(args, role) for role in sorted(ROLES | {"sandbox"}) if getattr(args, role) is not None}
    try:
        print(json.dumps(verify(args.archive, args.sha256, args.platform, roles), sort_keys=True))
    except (OSError, ValueError, KeyError, TypeError, AttributeError, RecursionError, tarfile.TarError):
        print("trial runtime archive refused: invalid digest, platform or inventory", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
