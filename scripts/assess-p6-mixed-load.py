#!/usr/bin/env python3
"""Assess three recorded mixed-training rounds; never send traffic."""

import argparse
import json
from pathlib import Path
import sys

from p6_evidence import SHA, exact, label, number, read, timings
from p6_training_acceptance import independent_round


WINDOW = 900_000
READ_BUCKET = 10_000
TIERS = {"small", "medium", "heaviest"}
READ_KINDS = {"statement", "result", "history"}


def assess_round(record, versions, independent=None):
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
    requests, accepted_versions, accepted = set(), {}, {}
    acceptance_windows = {}
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
        if ident in accepted_versions:
            failures.append("duplicate_submission")
        accepted_versions[ident] = version
        accepted[key] = ident
        acceptance_windows[ident] = (start, start + elapsed)
        confirmation_ms.append(elapsed)
        if row["terminal_ms"] is None:
            if row["verdict"] is not None or row["queue_ms"] is not None:
                raise ValueError("partial terminal evidence")
            continue
        terminal = number(row["terminal_ms"], 0, WINDOW)
        queue = number(row["queue_ms"], 0, terminal)
        acceptance_windows[ident] = (start, start + min(elapsed, queue))
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
    evidence = None
    if independent is not None:
        receipts, digest = independent
        database = {ident: receipt.version for ident, receipt in receipts.items()}
        common = accepted_versions.keys() & database.keys()
        time_mismatches = sum(
            not acceptance_windows[ident][0] <= receipts[ident].offset_ms <= acceptance_windows[ident][1]
            for ident in common
        )
        evidence = {
            "sha256": digest,
            "accepted": len(database),
            "unreconciled_database_receipts": len(database.keys() - accepted_versions.keys()),
            "client_receipts_missing_from_database": len(accepted_versions.keys() - database.keys()),
            "material_mismatches": sum(accepted_versions[ident] != database[ident]
                                      for ident in common),
            "acceptance_time_mismatches": time_mismatches,
        }
        if database != accepted_versions or time_mismatches:
            failures.append("independent_acceptance_reconciliation")
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
    return {"id": record["id"], "actors": len(actors), "measures": measures, "independent_acceptance": evidence,
            "reads_by_kind": {kind: timings(values, read_counts[kind]) for kind, values in read_ms.items()},
            "longest_queue_ms": max(queue_ms, default=None), "peaks": record["peaks"],
            "materials_by_tier": counts, "materials_by_version": by_version, "failures": sorted(set(failures))}


def assess(doc, independent=()):
    exact(doc, "schema release_sha256 topology_sha256 origin_scope versions rounds")
    if doc["schema"] != "oj.p6-mixed-load.v1" or doc["origin_scope"] not in ("local", "trial"):
        raise ValueError("unsupported report scope")
    if len(independent) not in (0, 3) or (doc["origin_scope"] == "trial" and len(independent) != 3):
        raise ValueError("expected three independent observations for trial")
    for key in ("release_sha256", "topology_sha256"):
        label(doc[key], SHA)
    versions, rounds = doc["versions"], doc["rounds"]
    if (not isinstance(versions, dict) or not 3 <= len(versions) <= 100
            or {label(tier) for tier in versions.values()} != TIERS
            or not isinstance(rounds, list) or len(rounds) != 3):
        raise ValueError("expected three rounds and a fixed material inventory")
    for version in versions:
        label(version)
    reports, observed_ids = [], set()
    for index, record in enumerate(rounds):
        exact(record, "id run_started_utc_ms reads submissions observed_submissions peaks")
        start = number(record["run_started_utc_ms"], 1, 10**14)
        evidence = independent_round(independent, index, start, start + WINDOW, observed_ids)
        reports.append(assess_round(record, versions, evidence))
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
    parser.add_argument("--acceptance", type=Path, nargs=3, metavar="OBSERVATION",
                        help="three raw training-acceptance exports in record order; required for trial")
    args = parser.parse_args()
    try:
        doc, digest = read(args.record)
        result = assess(doc, [read(path) for path in args.acceptance or ()])
    except (OSError, ValueError, RecursionError):
        print("P6 mixed-load assessment refused: invalid or incomplete record", file=sys.stderr)
        return 2
    print(json.dumps({**result, "record_sha256": digest}, sort_keys=True))
    return 0 if result["meets_recorded_thresholds"] else 1


if __name__ == "__main__":
    sys.exit(main())
