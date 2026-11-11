#!/usr/bin/env python3
"""Make one private, fixed Z-algorithm revision with site-authored scale cases."""

import argparse
import json
import os
from pathlib import Path
import sys

sys.dont_write_bytecode = True

from problem_draft_files import canonical, copy_checked, verify_bundle, write_new
from problem_source_common import digest, read_regular


ROOT = Path(__file__).resolve().parent.parent
SOURCE = "library-checker:zalgorithm"
LOCAL_DATA = "oj-local:zalgorithm-scale-v1"
BASE_VERSION = "151da5a28d0bd733fe4837547ec909137a36b0e532409c93dfd7bf3c2a7e6b41"
REVISED_VERSION = "965239246cbdbd63f1641cfcf551d9c8f3a65cd558e194668ad845cac1e39711"
BASE_INDEX = "8d99112ec81a50a29e3d0eb8b5f0e97a68f1ccad75c3c9ed34ec3555b265b1ef"
HISTORICAL = ("provenance/source-lock.json", "provenance/source-catalog.json",
              "provenance/source-validation.json", "provenance/material-map.json")


def prepare(base, output):
    base = base.parent.resolve(strict=True) / base.name
    output = output.parent.resolve(strict=True) / output.name
    # Compare directory identities so aliases on case-insensitive volumes cannot
    # send new files into the immutable base bundle or the checkout.
    if (output.exists() or output.is_symlink()
            or any(p.samefile(base) or p.samefile(ROOT) for p in output.parents)):
        raise ValueError("output must be new and outside checkout and input bundle")
    index = verify_bundle(base)
    if digest(canonical(index)) != BASE_INDEX:
        raise ValueError("requires the reviewed v6 bundle")
    files = index["files"]

    def original(name):
        raw = read_regular(base, name, 1 << 20)
        if files[name] != {"sha256": digest(raw), "size": len(raw)}:
            raise ValueError("input changed during copy: " + name)
        return raw

    entry, = (row for row in index["problems"] if row["source_id"] == SOURCE)
    raw = original(entry["draft"])
    draft = json.loads(raw)
    original_version = draft.pop("version")
    provenance = {name: original(name) for name in HISTORICAL}
    if (original_version != BASE_VERSION or digest(canonical(draft)) != BASE_VERSION
            or provenance["provenance/source-lock.json"] != (ROOT / "config/problem-sources/library-checker.json").read_bytes()):
        raise ValueError("requires the reviewed v6 Z-algorithm base")
    objects = {ref["path"] for ref in draft["provenance"]["original_files"].values()}
    for case in draft["test_cases"]:
        objects.update(case[kind]["path"] for kind in ("input", "answer"))

    output.mkdir(mode=0o700)
    write_new(output, "INCOMPLETE", b"Supplement preparation incomplete. Do not use.\n")
    partial = output / "bundle.partial"
    partial.mkdir(mode=0o700)
    copied, copied_bytes = {}, [0]

    def record(name, data):
        identity = {"sha256": digest(data), "size": len(data)}
        if name not in copied:
            write_new(partial, name, data)
            copied[name] = identity
        elif copied[name] != identity:
            raise ValueError("object identity conflict")
        return {"path": name, **identity}

    for name in sorted(objects):
        ref = copy_checked(base, name, partial, files[name], copied_bytes)
        if ref["path"] != name:
            raise ValueError("unexpected original object path")
        copied[name] = files[name]
    for name, data in provenance.items():
        record(name, data)
    statement = draft["statement"]["path"]
    record(statement, original(statement))
    draft["local_data"] = LOCAL_DATA
    for n in (1, 500000):
        inp = b"a" * n + b"\n"
        expected = (" ".join(map(str, range(n, 0, -1))) + "\n").encode("ascii")
        draft["test_cases"].append({"id": f"case-{len(draft['test_cases']) + 1:04d}", "sample": False,
                                    "input": record("objects/sha256/" + digest(inp), inp),
                                    "answer": record("objects/sha256/" + digest(expected), expected)})
    version = digest(canonical(draft))
    if version != REVISED_VERSION:
        raise ValueError("generated revision is not the reviewed fixed supplement")
    draft["version"] = version
    record(entry["draft"], canonical(draft))
    revised = {**index, "files": copied, "copied_source_bytes": copied_bytes[0],
               "problems": [{**entry, "version": version, "test_cases": len(draft["test_cases"])}]}
    write_new(partial, "index.json", canonical(revised))
    verify_bundle(partial, allow_incomplete=True)
    partial.rename(output / "bundle")
    (output / "INCOMPLETE").unlink()
    return version


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-bundle", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    os.umask(0o077)
    print(json.dumps({"version": prepare(args.base_bundle, args.output), "publication": "blocked"}))


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError) as exc:
        sys.exit(f"Z-algorithm supplement failed: {exc}")
