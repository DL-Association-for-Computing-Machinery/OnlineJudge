#!/usr/bin/env python3
"""Seal or verify the pairing of a trial application and its runtime archive."""

import argparse
import hashlib
import json
from pathlib import Path
import re
import sys
import tarfile

import trial_app_bundle as application
import verify_trial_images as runtime
from offline_judge_bundle import decode
from p6_evidence import SHA, exact, label


SCHEMA = "oj.trial-release.v1"
REVISION = re.compile(r"[0-9a-f]{40}")
MAX_MANIFEST = 8 << 10


def encode(doc):
    return (json.dumps(doc, sort_keys=True, separators=(",", ":")) + "\n").encode()


def inspect(doc, app, archive):
    exact(doc, "schema revision platform application_sha256 runtime_sha256 image_configurations")
    if doc["schema"] != SCHEMA or doc["platform"] not in tuple(application.MACHINE):
        raise ValueError("unsupported release schema or platform")
    revision = label(doc["revision"], REVISION)
    app_sha = label(doc["application_sha256"], SHA)
    runtime_sha = label(doc["runtime_sha256"], SHA)
    roles = doc["image_configurations"]
    if not isinstance(roles, dict) or set(roles) not in (runtime.ROLES, runtime.ROLES | {"sandbox"}):
        raise ValueError("unexpected runtime roles")
    if application.verify(app, app_sha, doc["platform"]) != revision:
        raise ValueError("release and application revisions differ")
    return runtime.verify(archive, runtime_sha, doc["platform"], roles)["manifest_digests"]


def pack(doc, manifest, app, archive):
    # A release manifest is a separate file, never an untracked member of the app.
    if manifest.resolve().is_relative_to(app.resolve()):
        raise ValueError("release manifest must be outside the application")
    inspect(doc, app, archive)
    data = encode(doc)
    with manifest.open("xb") as stream:
        stream.write(data)
    return hashlib.sha256(data).hexdigest()


def verify(manifest, expected, app, archive):
    label(expected, SHA)
    stream, _ = application.open_file(manifest, MAX_MANIFEST)
    with stream:
        data = stream.read(MAX_MANIFEST + 1)
    if len(data) > MAX_MANIFEST or hashlib.sha256(data).hexdigest() != expected:
        raise ValueError("release manifest digest mismatch")
    doc = decode(data)
    if encode(doc) != data:
        raise ValueError("noncanonical release manifest")
    image_manifests = inspect(doc, app, archive)
    return {**doc, "release_sha256": expected, "image_manifests": image_manifests}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    create = commands.add_parser("pack")
    check = commands.add_parser("verify")
    for command in (create, check):
        command.add_argument("manifest", type=Path)
        command.add_argument("--app", type=Path, required=True)
        command.add_argument("--runtime", type=Path, required=True)
    create.add_argument("--revision", required=True)
    create.add_argument("--platform", choices=tuple(application.MACHINE), required=True)
    create.add_argument("--app-sha256", required=True)
    create.add_argument("--runtime-sha256", required=True)
    for role in sorted(runtime.ROLES | {"sandbox"}):
        create.add_argument("--" + role, required=role in runtime.ROLES)
    check.add_argument("--sha256", required=True, help="Release digest from an independent trusted channel")
    args = parser.parse_args()
    try:
        if args.command == "pack":
            roles = {role: getattr(args, role) for role in sorted(runtime.ROLES | {"sandbox"})
                     if getattr(args, role) is not None}
            doc = dict(schema=SCHEMA, revision=args.revision, platform=args.platform,
                       application_sha256=args.app_sha256, runtime_sha256=args.runtime_sha256,
                       image_configurations=roles)
            print(pack(doc, args.manifest, args.app, args.runtime))
        else:
            print(json.dumps(verify(args.manifest, args.sha256, args.app, args.runtime), sort_keys=True))
    except (OSError, ValueError, KeyError, TypeError, AttributeError, RecursionError, tarfile.TarError):
        print("trial release refused: invalid or mismatched manifest, application or runtime", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
