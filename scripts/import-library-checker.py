#!/usr/bin/env python3
"""Adapt the pinned private source corpus into self-contained private drafts.

No network, program execution, database writes or publication. Requires Python
3.11+, a stable private corpus, and a new output outside the checkout.
"""

import argparse
import json
import os
from pathlib import Path
import sys
import tempfile
import tomllib

sys.dont_write_bytecode = True

from library_checker_statement import convert
from problem_draft_files import canonical, copy_checked, inventory, verify_bundle, write_new
from problem_source_common import MAX_REPORT, digest, expected_materials, parse_lock, read_regular
from problem_source_export import validate_report


ROOT = Path(__file__).resolve().parent.parent
BLOCKERS = ["statement-rendering-and-formula-validation",
            "production-checker-compatibility-and-resource-admission",
            "independent-problem-validation", "authorized-non-author-review",
            "authorized-atomic-publication"]


def run(corpus, output):
    lock_bytes = read_regular(ROOT / "config/problem-sources", "library-checker.json")
    lock = parse_lock(lock_bytes)
    # Resolve parents, but never erase a final symlink by resolving it first.
    corpus = corpus.parent.resolve(strict=True) / corpus.name
    output = output.parent.resolve(strict=True) / output.name
    if (output.exists() or output.is_symlink() or output.is_relative_to(ROOT.resolve())
            or output.is_relative_to(corpus) or corpus.is_relative_to(output)):
        raise ValueError("output must be new and outside checkout and corpus")
    if (corpus.parent / "INCOMPLETE").exists():
        raise ValueError("source preparation is incomplete")
    # Verify pinned metadata first, before deriving any data paths or mappings.
    with tempfile.TemporaryDirectory(prefix="oj-draft-metadata-") as temporary:
        metadata = Path(temporary)
        for name, entry in lock["files"].items():
            data = read_regular(corpus, "source/" + name)
            if len(data) != entry["size"] or digest(data) != entry["sha256"]:
                raise ValueError("source metadata differs from lock: " + name)
            write_new(metadata, name, data)
        # Derive paths/order only from the verified private snapshot. Reopening
        # mutable input here could bind a valid copy to an unpinned inventory.
        expected, catalog = expected_materials(metadata, lock)
    inventory(corpus, set(expected) | {"catalog.json", "validation.json"})
    original_catalog = read_regular(corpus, "catalog.json", MAX_REPORT)
    document = json.loads(original_catalog)
    if (document.get("schema") != "oj.private-source-corpus.v1"
            or document.get("status") != "private-unpublished"
            or document.get("repository") != lock["repository"]
            or document.get("commit") != lock["commit"]
            or document.get("source_lock_sha256") != digest(lock_bytes)
            or document.get("problems") != catalog):
        raise ValueError("source catalog differs from pinned mapping")
    report_bytes = read_regular(corpus, "validation.json", MAX_REPORT)
    validate_report(json.loads(report_bytes), lock, catalog, inventory=expected)
    output.mkdir(mode=0o700)
    write_new(output, "INCOMPLETE", b"Import incomplete. Do not use or publish.\n")
    partial = output / "bundle.partial"
    partial.mkdir(mode=0o700)
    objects, budget, files = {}, [0], {}
    def record(relative, data):
        entry = write_new(partial, relative, data)
        files[relative] = {k: entry[k] for k in ("sha256", "size")}
    for relative, entry in expected.items():
        objects[relative] = copy_checked(corpus, relative, partial, entry, budget)
        item = objects[relative]
        files[item["path"]] = {k: item[k] for k in ("sha256", "size")}
    record("provenance/source-lock.json", lock_bytes)
    record("provenance/source-catalog.json", original_catalog)
    record("provenance/source-validation.json", report_bytes)
    # These are evidence from source preparation, never an OJ validation pass.
    record("provenance/material-map.json", canonical(objects))
    summaries = []
    for problem in catalog:
        pid = problem["source_id"].split(":", 1)[1]
        base = problem["path"]
        # Re-read metadata from our verified immutable copy, not mutable inputs.
        def source_bytes(name):
            return (partial / objects["source/" + base + "/" + name]["path"]).read_bytes()
        config = tomllib.loads(source_bytes("info.toml").decode())
        samples = {}
        for case in problem["cases"]:
            if case["sample"]:
                samples[case["id"]] = tuple(
                    read_regular(partial, objects[f"problems/{pid}/{folder}/{case['id']}{suffix}"]["path"], 64 << 10).decode()
                    for folder, suffix in (("in", ".in"), ("out", ".out")))
        markdown, examples = convert(source_bytes("task.md").decode(), config["params"], samples)
        statement = {"path": "statements/" + pid + ".md", "language": "en",
                     "sha256": digest(markdown.encode()), "size": len(markdown.encode())}
        record(statement["path"], markdown.encode())
        cases, source_mapping = [], []
        for index, case in enumerate(problem["cases"], 1):
            case_id = "case-" + str(index).zfill(4)
            data = {"id": case_id, "sample": case["sample"]}
            for kind, folder, suffix in (("input", "in", ".in"), ("answer", "out", ".out")):
                data[kind] = objects[f"problems/{pid}/{folder}/{case['id']}{suffix}"]
            cases.append(data)
            source_mapping.append({"case_id": case_id, "upstream_case": case["id"]})
        case_ids = {m["upstream_case"]: m["case_id"] for m in source_mapping}
        draft = {"schema": "oj.problem-draft.v1", "state": "private-draft",
                 "title": problem["title_zh"],
                 "statement": statement, "examples": [case_ids[name] for name in examples],
                 "test_cases": cases,
                 "judging": {"mode": "tokens", "case_sensitive": True,
                             "token_comparison": "exact-bytes", "numeric_equivalence": False,
                             "compatibility": "configured-not-validated",
                             "production_profile": None, "limits": None},
                 "provenance": {"source_id": problem["source_id"], "repository": lock["repository"],
                                "commit": lock["commit"], "source_lock_sha256": digest(lock_bytes),
                                "source_path": base, "source_case_mapping": source_mapping,
                                "original_title": problem["title"],
                                "upstream_time_limit_seconds": problem["upstream_time_limit_seconds"],
                                "original_files": {n: v for n, v in objects.items()
                                                   if n.startswith(("source/" + base + "/", "source/common/"))
                                                   or n in {"source/LICENSE", "source/generate.py"}}},
                 "validation": {"status": "not-run", "publication_blockers": BLOCKERS},
                 "adapter": "library-checker-pinned-v1"}
        version = digest(canonical(draft))
        draft["version"] = version
        path = "drafts/" + pid + ".json"
        record(path, canonical(draft))
        summaries.append({"source_id": problem["source_id"], "draft": path, "version": version,
                          "test_cases": len(cases), "examples": len(examples)})
    index = {"schema": "oj.private-draft-bundle.v1", "state": "private-unpublished",
             "problems": summaries, "copied_source_bytes": budget[0], "files": files,
             "access": "All paths are bundle-relative private storage; only an authorized future service may expose statement and sample roles."}
    write_new(partial, "index.json", canonical(index))
    verify_bundle(partial, allow_incomplete=True)
    partial.rename(output / "bundle")
    (output / "INCOMPLETE").unlink()
    return index


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--corpus", type=Path)
    parser.add_argument("--output", type=Path)
    parser.add_argument("--verify-bundle", type=Path, help="verify a complete relocated bundle without the source corpus")
    args = parser.parse_args()
    os.umask(0o077)
    if args.verify_bundle:
        if args.corpus or args.output:
            parser.error("verification cannot be combined with import options")
        if (args.verify_bundle.parent / "INCOMPLETE").exists():
            raise ValueError("draft import is incomplete")
        index = verify_bundle(args.verify_bundle)
        print(json.dumps({"verified": True, "problems": len(index["problems"]),
                          "publication": "blocked"}))
        return
    if not args.corpus or not args.output:
        parser.error("import requires --corpus and --output")
    index = run(args.corpus, args.output)
    print(json.dumps({"bundle": str(args.output / "bundle"),
                      "problems": len(index["problems"]),
                      "test_cases": sum(p["test_cases"] for p in index["problems"])}))


if __name__ == "__main__":
    main()
