"""Prepare reviewed single-problem supplements from the immutable v6 bundle."""

import argparse
import json
import os
from pathlib import Path
import sys
from typing import NamedTuple

from problem_draft_files import canonical, copy_checked, verify_bundle, write_new
from problem_source_common import digest, read_regular


ROOT = Path(__file__).resolve().parent.parent
BASE_INDEX = "8d99112ec81a50a29e3d0eb8b5f0e97a68f1ccad75c3c9ed34ec3555b265b1ef"
HISTORICAL = ("provenance/source-lock.json", "provenance/source-catalog.json",
              "provenance/source-validation.json", "provenance/material-map.json")


class Supplement(NamedTuple):
    label: str
    base_version: str
    version: str
    index: str


SUPPLEMENTS = {
    "associative_array": Supplement("oj-local:associative-array-boundaries-v1",
                                    "cb50459ffb3537de06ff6b931ea9047842d92a9420c39e37f9fb3efaa73e1271",
                                    "4d6dd8e3fe97a0e1ec6585ff538219d811f660bdfff7cc2172bf6092cf858928",
                                    "d5a23dd68deccc0ff55f928344287b96e3aa9f3cc023c727d0ad822dae140a1d"),
    "zalgorithm": Supplement("oj-local:zalgorithm-scale-v1",
                             "151da5a28d0bd733fe4837547ec909137a36b0e532409c93dfd7bf3c2a7e6b41",
                             "965239246cbdbd63f1641cfcf551d9c8f3a65cd558e194668ad845cac1e39711",
                             "4a02a1c07dbddf2a4bb532f8a6d850c2a979c07ced6cd4533219788317fa8191"),
    "suffixarray": Supplement("oj-local:suffixarray-structures-v1",
                              "b7622607e0db239445f7b75b14bc9edc10b7992f1c6b6254fd4f784aa5cb0d97",
                              "7ddd631ac3f2f76e4cf56f85b23ee71fbdc0d5f54c40bdc968d27e933297b777",
                              "93b0f9b3ff0085d288186d4420a8ec97b36bba89f5467ac65359262cb1a9ae18"),
    "aplusb": Supplement("oj-local:aplusb-boundaries-v1",
                         "87ebb9680aa3abe190b61d4a5ccac1dc83e48b29656364e445e36fabe7467cc9",
                         "74c7797f5484daf9df01b1be2e5a537eba7b05b1ef20efc08110c61e3c1490fd",
                         "d8426964248ef07e1aae00ab12c8de690a9fdae147117b0251fa853cb4769c7b"),
    "unionfind": Supplement("oj-local:unionfind-singleton-v1",
                            "fd871a02594fcd663790cd0530153df2ad18bf95110559ce552a121c34705bed",
                            "42e62825925bf727c394f3435c2b50f850758576c392d082fb628f7219e4f07d",
                            "c5857fa3048311c6bcdb40dfde511c85c843c5feacedc56a552534436aa503d5"),
    "static_range_sum": Supplement("oj-local:static-range-sum-singleton-v1",
                                   "4d3adee969250c1834a61d59b456649f0c2cd0445902ad386b4a6f3e351a2f93",
                                   "a93676964a96fb5dd0320441307eb0bb11ed41b0a1e1d4441c47d124a644e375",
                                   "d518d555c9f33d57c574b06ac41fcd4b713fa31bd3f04d18e3daf329a4744841"),
}


def boundary_cases(problem):
    if problem == "associative_array":
        maximum = 10**18
        yield f"1\n1 {maximum}\n".encode("ascii"), b"0\n"
        yield f"2\n0 {maximum} {maximum}\n1 {maximum}\n".encode("ascii"), f"{maximum}\n".encode("ascii")
    elif problem == "zalgorithm":
        for n in (1, 500000):
            yield b"a" * n + b"\n", (" ".join(map(str, range(n, 0, -1))) + "\n").encode("ascii")
    elif problem == "suffixarray":
        yield b"abacabae" * 2 + b"\n", b"8 0 12 4 10 2 14 6 9 1 13 5 11 3 15 7\n"
        yield b"ab" * 9 + b"\n", b"16 14 12 10 8 6 4 2 0 17 15 13 11 9 7 5 3 1\n"
    elif problem == "aplusb":
        for a, b in ((0, 0), (0, 1), (1, 0), (10**9, 10**9 - 1), (10**9 - 1, 10**9)):
            yield f"{a} {b}\n".encode("ascii"), f"{a+b}\n".encode("ascii")
    elif problem == "unionfind":
        yield b"1 1\n1 0 0\n", b"1\n"
        yield b"1 1\n0 0 0\n", b""
    elif problem == "static_range_sum":
        for value in (0, 10**9):
            yield f"1 1\n{value}\n0 1\n".encode("ascii"), f"{value}\n".encode("ascii")
    else:
        raise ValueError("unrecognized supplement")


def prepare(problem, base, output):
    supplement = SUPPLEMENTS[problem]
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

    entry, = (row for row in index["problems"] if row["source_id"] == f"library-checker:{problem}")
    raw = original(entry["draft"])
    draft = json.loads(raw)
    original_version = draft.pop("version")
    provenance = {name: original(name) for name in HISTORICAL}
    if (original_version != supplement.base_version or digest(canonical(draft)) != supplement.base_version
            or provenance["provenance/source-lock.json"] != (ROOT / "config/problem-sources/library-checker.json").read_bytes()):
        raise ValueError("requires the reviewed v6 problem base")
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
    draft["local_data"] = supplement.label
    for inp, expected in boundary_cases(problem):
        draft["test_cases"].append({"id": f"case-{len(draft['test_cases']) + 1:04d}", "sample": False,
                                    "input": record("objects/sha256/" + digest(inp), inp),
                                    "answer": record("objects/sha256/" + digest(expected), expected)})
    version = digest(canonical(draft))
    if version != supplement.version:
        raise ValueError("generated revision is not the reviewed fixed supplement")
    draft["version"] = version
    record(entry["draft"], canonical(draft))
    revised = {**index, "files": copied, "copied_source_bytes": copied_bytes[0],
               "problems": [{**entry, "version": version, "test_cases": len(draft["test_cases"])}]}
    raw = canonical(revised)
    if digest(raw) != supplement.index:
        raise ValueError("generated index is not the reviewed fixed supplement")
    write_new(partial, "index.json", raw)
    verify_bundle(partial, allow_incomplete=True)
    partial.rename(output / "bundle")
    (output / "INCOMPLETE").unlink()
    return version


def main(problem):
    parser = argparse.ArgumentParser(description=f"Prepare the reviewed private {problem} boundary supplement.")
    parser.add_argument("--base-bundle", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    os.umask(0o077)
    try:
        print(json.dumps({"version": prepare(problem, args.base_bundle, args.output), "publication": "blocked"}))
    except (OSError, ValueError) as exc:
        sys.exit(f"{problem} supplement failed: {exc}")
