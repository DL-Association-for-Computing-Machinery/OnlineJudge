"""Private, bounded file snapshots for offline problem draft import."""

import hashlib
import json
import os
import re
from pathlib import Path
import stat

from problem_source_common import MAX_DATA_FILE, MAX_EXPORT, digest, read_regular, safe_path


def canonical(value):
    return (json.dumps(value, ensure_ascii=False, sort_keys=True,
                       separators=(",", ":"), allow_nan=False) + "\n").encode()


def write_new(root, relative, data):
    path = root / str(safe_path(relative))
    private_parents(root, path.parent)
    with os.fdopen(os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), "wb") as stream:
        stream.write(data)
    return {"path": relative, "size": len(data), "sha256": digest(data)}


def private_parents(root, parent):
    current = root
    for part in parent.relative_to(root).parts:
        current = current / part
        current.mkdir(mode=0o700, exist_ok=True)


def inventory(root, expected, *, private=False):
    """Reject aliases, links, devices and extra files before copying any bytes."""
    if not stat.S_ISDIR(root.lstat().st_mode):
        raise ValueError("corpus root must be an ordinary directory")
    if private and stat.S_IMODE(root.stat().st_mode) != 0o700:
        raise ValueError("bundle directory must be private")
    seen = set()
    directories = {str(p) for name in expected for p in Path(name).parents}
    for parent, dirs, files in os.walk(root, followlinks=False):
        for name in dirs + files:
            path = Path(parent) / name
            relative = path.relative_to(root).as_posix()
            safe_path(relative)
            mode = path.lstat().st_mode
            if private and stat.S_IMODE(mode) != (0o700 if stat.S_ISDIR(mode) else 0o600):
                raise ValueError("bundle member must be private")
            if stat.S_ISDIR(mode) and relative in directories:
                continue
            if not stat.S_ISREG(mode) or relative not in expected:
                raise ValueError("unexpected corpus member: " + relative)
            seen.add(relative)
            if len(seen) > len(expected):
                raise ValueError("corpus inventory exceeds budget")
    if seen != set(expected):
        raise ValueError("incomplete corpus inventory")


def copy_checked(source, relative, output, entry, budget):
    """Hash the exact stream written, using no-follow descriptors at every hop.

    Inputs must be a stable private snapshot. Concurrent content changes are
    harmless only when the copied bytes still match the pinned digest.
    """
    parts = safe_path(relative).parts
    fd = os.open(source, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
    try:
        for part in parts[:-1]:
            child = os.open(part, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=fd)
            os.close(fd)
            fd = child
        item = os.open(parts[-1], os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=fd)
        with os.fdopen(item, "rb") as stream:
            info = os.fstat(stream.fileno())
            if not stat.S_ISREG(info.st_mode) or not 0 <= info.st_size <= MAX_DATA_FILE:
                raise ValueError("invalid corpus file type or size")
            if "size" in entry and info.st_size != entry["size"]:
                raise ValueError("corpus file size differs from lock")
            checksum = hashlib.sha256()
            temporary = output / "copy.partial"
            size = 0
            with os.fdopen(os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), "wb") as target:
                while chunk := stream.read(1 << 20):
                    size += len(chunk)
                    budget[0] += len(chunk)
                    if size > MAX_DATA_FILE or budget[0] > MAX_EXPORT:
                        raise ValueError("draft material copy exceeds budget")
                    checksum.update(chunk)
                    target.write(chunk)
    finally:
        os.close(fd)
    sha = checksum.hexdigest()
    if sha != entry["sha256"] or ("size" in entry and size != entry["size"]):
        raise ValueError("corpus bytes differ from pinned hash: " + relative)
    material = "objects/sha256/" + sha
    destination = output / material
    private_parents(output, destination.parent)
    if destination.exists():
        # Only our own earlier verified copy can exist in this newly reserved
        # private output directory. Do not hardlink mutable input files.
        temporary.unlink()
    else:
        temporary.rename(destination)
    return {"path": material, "sha256": sha, "size": size}


def statement_image_references(draft):
    """An explicit role, bound to this problem's originals, never a URL fetch."""
    images = draft["statement"].get("images", {})
    if not isinstance(images, dict) or len(images) > 16 or ("images" in draft["statement"] and not images):
        raise ValueError("invalid statement image manifest")
    for source, reference in images.items():
        parts = source.split("/")
        original = "source/" + draft["provenance"]["source_path"] + "/" + source
        if (len(source) > 240 or not re.fullmatch(r"[A-Za-z0-9_./-]+\.(?:png|jpg|jpeg)", source)
                or any(not p or p.startswith(".") for p in parts)
                or not isinstance(reference, dict) or set(reference) != {"path", "sha256", "size"}
                or reference != draft["provenance"]["original_files"].get(original)
                or not isinstance(reference["size"], int) or isinstance(reference["size"], bool)
                or not 1 <= reference["size"] <= 256 << 10):
            raise ValueError("statement image must bind a canonical source-local original")
    return list(images.values())


def verify_bundle(root, *, allow_incomplete=False):
    """Verify a completed bundle without consulting its original source path.

    This checks internal integrity, not an author signature or publication
    authorization. Callers must still enforce the draft state and access rules.
    """
    if not allow_incomplete and (root.parent / "INCOMPLETE").exists():
        raise ValueError("draft import is incomplete")
    index = json.loads(read_regular(root, "index.json"))
    if (index.get("schema") != "oj.private-draft-bundle.v1"
            or index.get("state") != "private-unpublished"):
        raise ValueError("invalid draft bundle state")
    files = index["files"]
    inventory(root, set(files) | {"index.json"}, private=True)
    total = 0
    for path, entry in files.items():
        data = read_regular(root, path, MAX_DATA_FILE)
        total += len(data)
        if total > MAX_EXPORT or len(data) != entry["size"] or digest(data) != entry["sha256"]:
            raise ValueError("draft bundle integrity failure")
    summaries = index["problems"]
    paths = [p["draft"] for p in summaries]
    identities = [p["source_id"] for p in summaries]
    if (not paths or len(set(paths)) != len(paths) or len(set(identities)) != len(identities)
            or set(paths) != {p for p in files if p.startswith("drafts/")}):
        raise ValueError("draft index must enumerate each draft once")
    for problem in summaries:
        draft = json.loads(read_regular(root, problem["draft"]))
        version = draft.pop("version")
        if (version != problem["version"] or digest(canonical(draft)) != version
                or draft["schema"] != "oj.problem-draft.v1" or draft["state"] != "private-draft"
                or draft["validation"]["status"] != "not-run"
                or not draft["validation"]["publication_blockers"]):
            raise ValueError("invalid private draft version or validation state")
        if (problem["source_id"] != draft["provenance"]["source_id"]
                or problem["test_cases"] != len(draft["test_cases"])
                or problem["examples"] != len(draft["examples"])):
            raise ValueError("draft index summary differs from content")
        references = [draft["statement"], *draft["provenance"]["original_files"].values()]
        references.extend(statement_image_references(draft))
        for case in draft["test_cases"]:
            references.extend([case["input"], case["answer"]])
        for reference in references:
            if files.get(reference["path"]) != {k: reference[k] for k in ("size", "sha256")}:
                raise ValueError("unbound draft reference")
        samples = {c["id"] for c in draft["test_cases"] if c["sample"]}
        if set(draft["examples"]) != samples or len(draft["examples"]) != len(samples):
            raise ValueError("draft examples differ from sample roles")
    return index
