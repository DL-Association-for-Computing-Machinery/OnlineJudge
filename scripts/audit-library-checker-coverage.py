#!/usr/bin/env python3
"""Inventory input coverage in the reviewed private Library Checker bundles."""

import argparse
import json
from pathlib import Path
import sys

sys.dont_write_bytecode = True

from library_checker_coverage import BOUNDARIES, STRUCTURES, observe
from library_checker_supplement import BASE_INDEX, SUPPLEMENTS
from problem_draft_files import canonical, verify_bundle
from problem_source_common import MAX_DATA_FILE, digest, read_regular


SOURCE_LOCK = "fe2722edade748228c9add9c54704c09861fa6be649b5cdf59e1f214b5ee9617"


def audit(bundle):
    index_hash = digest(read_regular(bundle, "index.json"))
    if index_hash != BASE_INDEX and not any(index_hash == item.index for item in SUPPLEMENTS.values()):
        raise ValueError("coverage rules apply only to the reviewed bundle indexes")
    index = verify_bundle(bundle)
    if digest(canonical(index)) != index_hash:
        raise ValueError("bundle index changed during verification")
    problems = []
    for entry in index["problems"]:
        source = entry["source_id"].removeprefix("library-checker:")
        raw = read_regular(bundle, entry["draft"])
        if digest(raw) != index["files"][entry["draft"]]["sha256"]:
            raise ValueError("draft changed during audit")
        draft = json.loads(raw)
        if draft["provenance"]["source_lock_sha256"] != SOURCE_LOCK:
            raise ValueError("unexpected source lock")
        cases = draft["test_cases"]
        rules = sorted(set(BOUNDARIES[source]) | set(STRUCTURES[source]))
        results = {rule: {role: {"count": 0, "first": None} for role in ("sample", "non_sample")}
                   for rule in rules}
        for i, case in enumerate(cases, 1):
            if case["id"] != f"case-{i:04d}":
                raise ValueError("unexpected ordered case ID")
            inp = case["input"]
            data = read_regular(bundle, inp["path"], MAX_DATA_FILE)
            if len(data) != inp["size"] or digest(data) != inp["sha256"]:
                raise ValueError("input changed during audit")
            role = "sample" if case["sample"] else "non_sample"
            witness = {"case": case["id"], "input_sha256": inp["sha256"],
                       "answer_sha256": case["answer"]["sha256"]}
            observed = observe(source, data)
            for rule in observed:
                match = results[rule][role]
                if match["count"] == 0:
                    match["first"] = witness
                match["count"] += 1
        problems.append({"source_id": entry["source_id"], "content_version": draft["version"],
                         "points": len(cases), "samples": len(draft["examples"]), "rules": results})
    return {"schema": "oj.lc-input-coverage.v1", "bundle_index_sha256": index_hash,
            "source_lock_sha256": SOURCE_LOCK, "problems": problems}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bundle", required=True, type=Path)
    args = parser.parse_args()
    print(json.dumps(audit(args.bundle), indent=2, sort_keys=True))


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError) as exc:
        sys.exit(f"Input coverage audit failed: {exc}")
