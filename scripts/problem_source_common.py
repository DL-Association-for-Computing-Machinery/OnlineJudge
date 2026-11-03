"""Bounded, non-executing readers for the pinned Library Checker source corpus."""

import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import stat
import tomllib


MAX_SOURCE_FILE = 1 << 20
MAX_DATA_FILE = 128 << 20
MAX_EXPORT = 2 << 30
MAX_REPORT = 128 << 10


def digest(data):
    return hashlib.sha256(data).hexdigest()


def safe_path(value):
    """Only portable relative paths, without aliases or traversal, are accepted."""
    if not isinstance(value, str) or not re.fullmatch(r"[A-Za-z0-9_./-]{1,240}", value):
        raise ValueError("invalid material path")
    path = PurePosixPath(value)
    if not path.parts or path.is_absolute() or str(path) != value or any(p in {".", ".."} for p in path.parts):
        raise ValueError("non-canonical material path")
    return path


def read_regular(root, relative, limit=MAX_SOURCE_FILE):
    """Read only bounded regular files under a private, stable snapshot root."""
    parts = safe_path(relative).parts
    fd = os.open(root, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
    try:
        for part in parts[:-1]:
            next_fd = os.open(part, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=fd)
            os.close(fd)
            fd = next_fd
        file_fd = os.open(parts[-1], os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=fd)
        with os.fdopen(file_fd, "rb") as stream:
            info = os.fstat(stream.fileno())
            if not stat.S_ISREG(info.st_mode) or info.st_size > limit:
                raise ValueError("invalid material type or size")
            data = stream.read(limit + 1)
    finally:
        os.close(fd)
    if len(data) > limit:
        raise ValueError("material exceeds budget")
    return data


def load_lock(path):
    return parse_lock(read_regular(path.parent, path.name))


def parse_lock(data):
    """Validate the same frozen lock bytes used for snapshots and provenance."""
    lock = json.loads(data)
    if lock.get("schema") != "oj.library-checker-source.v1" or not re.fullmatch(
        r"[0-9a-f]{40}", lock.get("commit", "")
    ) or lock.get("repository") != "https://github.com/yosupo06/library-checker-problems":
        raise ValueError("unsupported source lock")
    if len(lock["problems"]) != 10 or not 1 <= len(lock["files"]) <= 300:
        raise ValueError("invalid source inventory")
    for name, entry in lock["files"].items():
        safe_path(name)
        if not re.fullmatch(r"[0-9a-f]{64}", entry["sha256"]) or not 0 <= entry["size"] <= MAX_SOURCE_FILE:
            raise ValueError("invalid source digest or size")
    ids = [p["path"].split("/")[-1] for p in lock["problems"]]
    if len(set(ids)) != 10:
        raise ValueError("duplicate source identity")
    for problem in lock["problems"]:
        safe_path(problem["path"])
        for required in ("task.md", "info.toml", "hash.json", "checker.cpp", "verifier.cpp", "sol/correct.cpp"):
            if problem["path"] + "/" + required not in lock["files"]:
                raise ValueError("incomplete source problem")
    for required in ("LICENSE", "generate.py", "common/testlib.h"):
        if required not in lock["files"]:
            raise ValueError("missing source license or dependency")
    return lock


def snapshot(source, target, lock):
    """Copy only lock-listed bytes. Extra checkout files are never executed."""
    total = 0
    for name, entry in lock["files"].items():
        data = read_regular(source, name)
        if len(data) != entry["size"] or digest(data) != entry["sha256"]:
            raise ValueError("source differs from lock: " + name)
        total += len(data)
        if total > 16 << 20:
            raise ValueError("source snapshot exceeds budget")
        path = target / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(data)


def expected_materials(source, lock):
    """Derive the export allowlist from pinned inputs, never container reports."""
    expected = {"source/" + name: dict(entry) for name, entry in lock["files"].items()}
    catalog = []
    for problem in lock["problems"]:
        base = problem["path"]
        pid = base.split("/")[-1]
        config = tomllib.loads(read_regular(source, base + "/info.toml").decode())
        hashes = json.loads(read_regular(source, base + "/hash.json"))
        cases = []
        case_names = set()
        for test in config["tests"]:
            name = str(safe_path(test["name"]))
            if "/" in name or not isinstance(test["number"], int) or not 1 <= test["number"] <= 100:
                raise ValueError("unsupported test group")
            for i in range(test["number"]):
                case = PurePosixPath(name).stem + "_" + str(i).zfill(2)
                if case in case_names:
                    raise ValueError("duplicate test identity")
                case_names.add(case)
                cases.append({"id": case, "sample": name == "example.in"})
                for folder, suffix in (("in", ".in"), ("out", ".out")):
                    sha = hashes[case + suffix]
                    if not re.fullmatch(r"[0-9a-f]{64}", sha):
                        raise ValueError("invalid data digest")
                    expected[f"problems/{pid}/{folder}/{case}{suffix}"] = {"sha256": sha}
        if set(hashes) != {c + suffix for c in case_names for suffix in (".in", ".out")}:
            raise ValueError("test configuration and pinned hashes differ")
        catalog.append({"source_id": "library-checker:" + pid, "path": base,
                        "title": config["title"], "title_zh": problem["title_zh"],
                        "upstream_time_limit_seconds": config["timelimit"], "cases": cases,
                        "checker": "upstream-testlib-case-sensitive-tokens"})
    return expected, catalog
