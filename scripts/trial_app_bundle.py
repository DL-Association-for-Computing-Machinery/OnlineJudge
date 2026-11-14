#!/usr/bin/env python3
"""Seal and verify the exported trial application without executing it."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import sys


SCHEMA = "oj.trial-application.v2"
MANIFEST = "manifest.json"
PROGRAMS = {"api", "worker", "worker-api", "migrate", "materialstore", "mail-worker"}
SITE = {"index.html", "THIRD-PARTY-NOTICES.txt"}
TEMPLATE = "nginx.conf.example"
ASSET = re.compile(r"[A-Za-z0-9_-][A-Za-z0-9_.-]{0,199}\.(js|css|ttf|woff2?)")
MACHINE = {"linux/amd64": 62, "linux/arm64": 183}
MAX_ASSETS = 512
MAX_MANIFEST = 128 << 10
MAX_PROGRAM = 64 << 20
MAX_STATIC = 4 << 20
MAX_TOTAL = 512 << 20


def require_directory(path, private):
    info = path.lstat()
    if not stat.S_ISDIR(info.st_mode) or (private and (stat.S_IMODE(info.st_mode) != 0o700
                                                       or info.st_uid != os.geteuid())):
        raise ValueError("unexpected directory type or permissions")


def names(path, limit):
    found = set()
    for entry in path.iterdir():
        found.add(entry.name)
        if len(found) > limit:
            raise ValueError("too many bundle entries")
    return found


def layout(root, sealed):
    for folder in ("", "bin", "site", "site/assets", "templates"):
        require_directory(root / folder, sealed)
    top = {"bin", "site", "templates"} | ({MANIFEST} if sealed else set())
    if (names(root, len(top)) != top or names(root / "bin", len(PROGRAMS)) != PROGRAMS
            or names(root / "site", 3) != SITE | {"assets"} or names(root / "templates", 1) != {TEMPLATE}):
        raise ValueError("unexpected application file set")
    assets = names(root / "site/assets", MAX_ASSETS)
    if not assets or not all(ASSET.fullmatch(name) for name in assets) or not any(
            name.endswith(".js") for name in assets):
        raise ValueError("unexpected static assets")
    return sorted([*(f"bin/{name}" for name in PROGRAMS), *(f"site/{name}" for name in SITE),
                   *(f"site/assets/{name}" for name in assets), f"templates/{TEMPLATE}"])


def open_file(path, limit, sealed_mode=None):
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    stream = os.fdopen(fd, "rb")
    try:
        info = os.fstat(fd)
        if not stat.S_ISREG(info.st_mode) or info.st_nlink != 1 or not 0 < info.st_size <= limit:
            raise ValueError("unexpected file type or size")
        if sealed_mode is not None and (stat.S_IMODE(info.st_mode) != sealed_mode or info.st_uid != os.geteuid()):
            raise ValueError("unexpected file ownership or permissions")
        return stream, info.st_size
    except (OSError, ValueError):
        stream.close()
        raise


def inventory(root, paths, platform, destination=None):
    entries = {}
    total = 0
    for name in paths:
        program = name.startswith("bin/")
        limit = MAX_PROGRAM if program else MAX_STATIC
        mode = 0o700 if program else 0o600
        stream, size = open_file(root / name, limit, mode if destination is None else None)
        with stream:
            total += size
            if total > MAX_TOTAL:
                raise ValueError("application exceeds bundle budget")
            if program:
                header = stream.read(64)
                if (len(header) != 64 or header[:7] != b"\x7fELF\x02\x01\x01"
                        or int.from_bytes(header[16:18], "little") != 2
                        or int.from_bytes(header[18:20], "little") != MACHINE[platform]):
                    raise ValueError("expected an executable ELF for the declared platform")
                stream.seek(0)
            digest = hashlib.sha256()
            if destination is None:
                target = None
            else:
                target = (destination / name).open("xb")
            try:
                count = 0
                while chunk := stream.read(1 << 20):
                    count += len(chunk)
                    if count > size:
                        raise ValueError("file changed during sealing or verification")
                    digest.update(chunk)
                    if target is not None:
                        target.write(chunk)
                if count != size:
                    raise ValueError("file changed during sealing or verification")
            finally:
                if target is not None:
                    target.close()
            if destination is not None:
                (destination / name).chmod(mode)
        entries[name] = {"bytes": size, "sha256": digest.hexdigest()}
    return entries


def encode(revision, platform, files):
    doc = {"schema": SCHEMA, "revision": revision, "platform": platform, "files": files}
    return (json.dumps(doc, sort_keys=True, separators=(",", ":")) + "\n").encode()


def pack(source, revision, platform, output):
    if not re.fullmatch(r"[a-f0-9]{40}", revision):
        raise ValueError("expected a full Git revision")
    if output.resolve().is_relative_to(source.resolve()):
        raise ValueError("bundle output must be outside the export")
    paths = layout(source, sealed=False)
    output.mkdir(mode=0o700)
    for folder in ("bin", "site", "site/assets", "templates"):
        (output / folder).mkdir(mode=0o700)
    data = encode(revision, platform, inventory(source, paths, platform, output))
    if len(data) > MAX_MANIFEST:
        raise ValueError("application manifest exceeds budget")
    with (output / MANIFEST).open("xb") as stream:
        stream.write(data)
    (output / MANIFEST).chmod(0o600)
    return hashlib.sha256(data).hexdigest()


def verify(root, trusted, platform):
    if not re.fullmatch(r"[a-f0-9]{64}", trusted):
        raise ValueError("expected an independently supplied SHA-256")
    paths = layout(root, sealed=True)
    stream, _ = open_file(root / MANIFEST, MAX_MANIFEST, 0o600)
    with stream:
        data = stream.read(MAX_MANIFEST + 1)
    if len(data) > MAX_MANIFEST or hashlib.sha256(data).hexdigest() != trusted:
        raise ValueError("application manifest digest mismatch")
    doc = json.loads(data)
    revision = doc.get("revision") if isinstance(doc, dict) else None
    if (not isinstance(revision, str) or not re.fullmatch(r"[a-f0-9]{40}", revision)
            or doc.get("schema") != SCHEMA or doc.get("platform") != platform):
        raise ValueError("unsupported application manifest or platform")
    if data != encode(revision, platform, inventory(root, paths, platform)):
        raise ValueError("application manifest or file content mismatch")
    return revision


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    create = commands.add_parser("pack")
    create.add_argument("--export", type=Path, required=True)
    create.add_argument("--revision", required=True)
    check = commands.add_parser("verify")
    check.add_argument("--manifest-sha256", required=True)
    for command in (create, check):
        command.add_argument("--platform", choices=MACHINE, required=True)
        command.add_argument("bundle", type=Path)
    args = parser.parse_args()
    try:
        if args.command == "pack":
            print(pack(args.export, args.revision, args.platform, args.bundle))
        else:
            print(verify(args.bundle, args.manifest_sha256, args.platform))
    except (OSError, ValueError, RecursionError):
        print("trial application bundle refused: invalid paths, platform or content", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
