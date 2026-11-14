#!/usr/bin/env python3
"""Assess three recorded mixed-training rounds; never send traffic."""

import argparse
import hashlib
import json
import math
import os
from pathlib import Path
import re
import stat
import sys


WINDOW = 900_000
READ_BUCKET = 10_000
TIERS = {"small", "medium", "heaviest"}
READ_KINDS = {"statement", "result", "history"}
MAX_RECORD = 24 << 20
IDENT = re.compile(r"[a-zA-Z0-9_.:-]{1,80}")
SHA = re.compile(r"[0-9a-f]{64}")


def unique(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate field")
        result[key] = value
    return result


def read(path):
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(fd, "rb") as stream:
        if not stat.S_ISREG(os.fstat(fd).st_mode):
            raise ValueError("expected regular record")
        raw = stream.read(MAX_RECORD + 1)
    if len(raw) > MAX_RECORD:
        raise ValueError("oversized record")
    return json.loads(raw, object_pairs_hook=unique), hashlib.sha256(raw).hexdigest()


def exact(obj, fields):
    if not isinstance(obj, dict) or set(obj) != set(fields.split()):
        raise ValueError("unexpected record fields")


def number(value, low, high):
    if type(value) is not int or not low <= value <= high:
        raise ValueError("number outside supported range")
    return value


def label(value, pattern=IDENT):
    if not isinstance(value, str) or not pattern.fullmatch(value):
        raise ValueError("invalid identifier")
    return value


def timings(values, attempts):
    values = sorted(values)
    # Failed or missing observations rank after finite observations. Also
    # report the count of actual finite measurements for each population.
    result = {"attempts": attempts, "samples": len(values)}
    for p in (50, 95, 99):
        index = math.ceil(attempts * p / 100) - 1
        result[f"p{p}_ms"] = values[index] if 0 <= index < len(values) else None
    return result


def assess_round(record, versions):
    exact(record, "id run_started_utc_ms reads submissions observed_submissions peaks")
    label(record["id"])
    number(record["run_started_utc_ms"], 1, 10**14)
    reads, submissions, observed = (record[key] for key in ("reads", "submissions", "observed_submissions"))
    if not (isinstance(reads, list) and len(reads) <= 90_000
            and isinstance(submissions, list) and len(submissions) <= 3000
            and isinstance(observed, list) and len(observed) <= 3000):
        raise ValueError("unsupported sample count")
    exact(record["peaks"], "cpu_millicores memory_bytes disk_bytes connections")
    for value in record["peaks"].values():
        number(value, 0, 10**15)

    slots = set()
    read_ms = {kind: [] for kind in sorted(READ_KINDS)}
    read_counts = dict.fromkeys(read_ms, 0)
    for row in reads:
        exact(row, "subject kind start_ms elapsed_ms ok")
        actor = label(row["subject"])
        kind = label(row["kind"])
        bucket = number(row["start_ms"], 0, WINDOW - 1) // READ_BUCKET
        if (actor, bucket) in slots or type(row["ok"]) is not bool or kind not in READ_KINDS:
            raise ValueError("duplicate read slot or invalid outcome")
        slots.add((actor, bucket))
        elapsed = number(row["elapsed_ms"], 0, WINDOW)
        read_counts[kind] += 1
        if row["ok"]:
            read_ms[kind].append(elapsed)
    actors = {actor for actor, _ in slots}
    failures = []
    if len(actors) < 100 or len(slots) != len(actors) * (WINDOW // READ_BUCKET):
        failures.append("read_schedule")

    bursts = [set(), set(), set()]
    requests, ids, accepted = set(), set(), {}
    terminal_ms, queue_ms, confirmation_ms = [], [], []
    counts = {tier: 0 for tier in sorted(TIERS)}
    by_version = dict.fromkeys(versions, 0)
    for row in submissions:
        exact(row, "key subject version start_ms confirmation_ms id queue_ms terminal_ms verdict")
        key, actor, version = (label(row[k]) for k in ("key", "subject", "version"))
        start = number(row["start_ms"], 0, WINDOW - 1)
        if key in requests or actor not in actors or version not in versions:
            raise ValueError("invalid submission binding")
        requests.add(key)
        burst, offset = divmod(start, 300_000)
        if offset >= 60_000 or actor in bursts[burst]:
            raise ValueError("submission outside its declared burst")
        bursts[burst].add(actor)
        counts[versions[version]] += 1
        by_version[version] += 1
        elapsed = number(row["confirmation_ms"], 0, WINDOW)
        if row["id"] is None:
            if any(row[k] is not None for k in ("queue_ms", "terminal_ms", "verdict")):
                raise ValueError("unaccepted submission has a verdict")
            continue
        ident = label(row["id"])
        if ident in ids:
            failures.append("duplicate_submission")
        ids.add(ident)
        accepted[key] = ident
        confirmation_ms.append(elapsed)
        if row["terminal_ms"] is None:
            if row["verdict"] is not None or row["queue_ms"] is not None:
                raise ValueError("partial terminal evidence")
            continue
        terminal = number(row["terminal_ms"], 0, WINDOW)
        queue = number(row["queue_ms"], 0, terminal)
        queue_ms.append(queue)
        if row["verdict"] not in ("AC", "WA", "CE", "RE", "TLE", "MLE", "OLE", "SE"):
            raise ValueError("unsupported verdict")
        if row["verdict"] == "SE":
            failures.append("system_error")
        terminal_ms.append(terminal)
    ledger = {}
    for row in observed:
        exact(row, "key id")
        key, ident = label(row["key"]), label(row["id"])
        if key in ledger:
            failures.append("submission_reconciliation")
        ledger[key] = ident
    if ledger != accepted:
        failures.append("submission_reconciliation")
    if any(len(burst) != 30 for burst in bursts) or not all(counts.values()):
        failures.append("submission_schedule_or_material_mix")

    measures = {"read": timings([value for values in read_ms.values() for value in values], len(reads)),
                "confirmation": timings(confirmation_ms, len(submissions)),
                "terminal": timings(terminal_ms, len(confirmation_ms))}
    for name, limit in (("read", 1000), ("confirmation", 1000), ("terminal", 60_000)):
        p95 = measures[name]["p95_ms"]
        if p95 is None or p95 > limit:
            failures.append(name + "_p95")
    for name in ("read", "confirmation"):
        attempts, count = (measures[name][k] for k in ("attempts", "samples"))
        measures[name]["errors"] = attempts - count
        measures[name]["error_rate"] = (attempts - count) / attempts if attempts else None
        if not attempts or 100 * (attempts - count) > attempts:
            failures.append(name + "_error_rate")
    if len(terminal_ms) != len(confirmation_ms) or max(terminal_ms, default=0) > 180_000:
        failures.append("missing_or_late_terminal")
    return {"id": record["id"], "actors": len(actors), "measures": measures,
            "reads_by_kind": {kind: timings(values, read_counts[kind]) for kind, values in read_ms.items()},
            "longest_queue_ms": max(queue_ms, default=None), "peaks": record["peaks"],
            "materials_by_tier": counts, "materials_by_version": by_version, "failures": sorted(set(failures))}


def assess(doc):
    exact(doc, "schema release_sha256 topology_sha256 origin_scope versions rounds")
    if doc["schema"] != "oj.p6-mixed-load.v1" or doc["origin_scope"] not in ("local", "trial"):
        raise ValueError("unsupported report scope")
    for key in ("release_sha256", "topology_sha256"):
        label(doc[key], SHA)
    versions, rounds = doc["versions"], doc["rounds"]
    if (not isinstance(versions, dict) or not 3 <= len(versions) <= 100
            or {label(tier) for tier in versions.values()} != TIERS
            or not isinstance(rounds, list) or len(rounds) != 3):
        raise ValueError("expected three rounds and a fixed material inventory")
    for version in versions:
        label(version)
    reports = [assess_round(record, versions) for record in rounds]
    if len({item["id"] for item in reports}) != 3:
        raise ValueError("round IDs must be distinct")
    starts = sorted(row["run_started_utc_ms"] for row in rounds)
    if any(b - a < WINDOW for a, b in zip(starts, starts[1:])):
        raise ValueError("measurement windows overlap")
    return {"scope": "recorded_mixed_load_only", "origin_scope": doc["origin_scope"],
            "release_sha256": doc["release_sha256"], "topology_sha256": doc["topology_sha256"],
            "rounds": reports, "meets_recorded_thresholds": all(not item["failures"] for item in reports)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("record", type=Path)
    args = parser.parse_args()
    try:
        doc, digest = read(args.record)
        result = assess(doc)
    except (OSError, ValueError, RecursionError):
        print("P6 mixed-load assessment refused: invalid or incomplete record", file=sys.stderr)
        return 2
    print(json.dumps({**result, "record_sha256": digest}, sort_keys=True))
    return 0 if result["meets_recorded_thresholds"] else 1


if __name__ == "__main__":
    sys.exit(main())
