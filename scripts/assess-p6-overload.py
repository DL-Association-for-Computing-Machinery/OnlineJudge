#!/usr/bin/env python3
"""Reconcile three recorded P6 overload rounds without sending requests."""

import argparse
from bisect import bisect_right
import json
from pathlib import Path
import sys

from p6_evidence import SHA, exact, label, number, read, timings
from p6_training_acceptance import independent_round


DAY_MS = 86_400_000
RECOVERY_MS = 300_000
MAX_ROWS = 20_000
VERDICTS = ("AC", "WA", "CE", "RE", "TLE", "MLE", "OLE", "SE")


def rows(value):
    if not isinstance(value, list) or len(value) > MAX_ROWS:
        raise ValueError("unsupported observation count")
    return value


def assess_round(client, server, limit, observations, index, seen):
    exact(client, "id started_utc_ms stopped_ms ended_ms attempts")
    exact(server, "id saturated_ms unfinished drain_ms ledger peaks oom_kills abnormal_exits")
    round_id = label(client["id"])
    if round_id != server["id"]:
        raise ValueError("round mismatch")
    start = number(client["started_utc_ms"], 1, 10**14)
    end = number(client["ended_ms"], 1, DAY_MS)
    independent = independent_round(observations, index, start, start + end, seen)
    stop = number(client["stopped_ms"], 1, end)
    full = number(server["saturated_ms"], 0, stop - 1)
    occupied = number(server["unfinished"], 0, MAX_ROWS)
    drain = number(server["drain_ms"], stop, end) if server["drain_ms"] is not None else None
    exact(server["peaks"], "cpu_millicores memory_bytes disk_bytes connections")
    for value in server["peaks"].values():
        number(value, 0, 10**15)
    failures = set()
    oom = number(server["oom_kills"], 0, MAX_ROWS)
    exits = number(server["abnormal_exits"], 0, MAX_ROWS)
    if oom or exits:
        failures.add("process_health")

    ledger = {}
    seen_ids = set()
    ledger_offsets = {}
    accepted_times, terminal_times = [], []
    queue_ms = []
    for item in rows(server["ledger"]):
        exact(item, "key id accepted_ms queued_ms terminal_ms read_ms verdict")
        key, ident = label(item["key"]), label(item["id"])
        if key in ledger or ident in seen_ids:
            failures.add("duplicate_ledger_entry")
        ledger[key] = item
        seen_ids.add(ident)
        accepted = number(item["accepted_ms"], 0, end)
        ledger_offsets[ident] = accepted
        accepted_times.append(accepted)
        terminal, read_at, verdict = (item[k] for k in ("terminal_ms", "read_ms", "verdict"))
        queued = item["queued_ms"]
        if queued is not None:
            queue_ms.append(number(queued, 0, end - accepted))
        if terminal is not None:
            number(terminal, accepted + (queued or 0), end)
            terminal_times.append(terminal)
        if read_at is not None:
            number(read_at, terminal if terminal is not None else accepted, end)
        if terminal is None or read_at is None or queued is None or verdict not in VERDICTS:
            failures.add("missing_terminal_readback")
        elif verdict == "SE":
            failures.add("system_error")
        if drain is not None and accepted <= stop and (terminal is None or terminal > drain):
            failures.add("drain_unproven")
    accepted_times.sort()
    terminal_times.sort()

    evidence = None
    if independent is not None:
        database, digest = independent
        evidence = {
            "sha256": digest,
            "accepted": len(database),
            "unreconciled_database_receipts": len(database.keys() - ledger_offsets.keys()),
            "ledger_receipts_missing_from_database": len(ledger_offsets.keys() - database.keys()),
            "acceptance_time_mismatches": sum(ledger_offsets[ident] != database[ident].offset_ms
                                              for ident in ledger_offsets.keys() & database.keys()),
        }
        if any(evidence[key] for key in ("unreconciled_database_receipts", "ledger_receipts_missing_from_database",
                                         "acceptance_time_mismatches")):
            failures.add("independent_acceptance_reconciliation")

    def unfinished_at(at):
        return bisect_right(accepted_times, at) - bisect_right(terminal_times, at)

    if occupied != limit or unfinished_at(full) != limit:
        failures.add("global_queue_not_full")
    if drain is None:
        failures.add("drain_unproven")

    attempts = {}
    latencies = {code: [] for code in ("created", "replayed", "rejected", "unexpected")}
    unexpected = 0
    no_response = 0
    for item in rows(client["attempts"]):
        exact(item, "key request_sha256 started_ms finished_ms status code id")
        key = label(item["key"])
        label(item["request_sha256"], SHA)
        sent = number(item["started_ms"], 0, end)
        done = number(item["finished_ms"], sent, end)
        status = number(item["status"], 100, 599) if item["status"] is not None else None
        if item["code"] is not None:
            label(item["code"])
        if item["id"] is not None:
            label(item["id"])
        if status in (200, 202) and item["code"] is None and item["id"] is not None:
            outcome = "created" if status == 202 else "replayed"
        elif status == 429 and item["code"] == "BACKLOG_LIMIT" and item["id"] is None:
            outcome = "rejected"
        else:
            outcome = "unexpected"
            unexpected += 1
            no_response += status is None
        latencies[outcome].append(done - sent)
        attempts.setdefault(key, []).append((sent, done, outcome, item))
    if unexpected:
        failures.add("unexpected_response")

    matched = set()
    saw_rejected = saw_replay = saw_retry = False
    fresh = []
    for key, events in attempts.items():
        events.sort(key=lambda event: event[0])
        if (len({item["request_sha256"] for _, _, _, item in events}) != 1
                or any(a[0] == b[0] or a[1] > b[0] for a, b in zip(events, events[1:]))):
            raise ValueError("same-key evidence is ambiguous")
        persisted = ledger.get(key)
        created = False
        rejected_under_pressure = False
        for sent, done, outcome, item in events:
            if outcome == "rejected":
                if created or (persisted and persisted["accepted_ms"] < done):
                    failures.add("rejection_wrote_or_rejected_replay")
                if full <= sent and done <= stop and unfinished_at(sent) == limit:
                    saw_rejected = rejected_under_pressure = True
            elif outcome == "created":
                if (created or not persisted or item["id"] != persisted["id"]
                        or not sent <= persisted["accepted_ms"] <= done):
                    failures.add("submission_reconciliation")
                else:
                    matched.add(key)
                    if sent >= stop:
                        if rejected_under_pressure:
                            saw_retry = True
                        if len(events) == 1:
                            fresh.append(done - stop)
                created = True
            elif outcome == "replayed":
                if not created or not persisted or item["id"] != persisted["id"]:
                    failures.add("replay_changed_receipt")
                elif full <= sent and done <= stop and unfinished_at(sent) == limit:
                    saw_replay = True
    if matched != set(ledger):
        failures.add("submission_reconciliation")
    if not saw_rejected:
        failures.add("missing_controlled_rejection")
    if not saw_replay:
        failures.add("missing_replay_during_pressure")
    if not saw_retry:
        failures.add("missing_rejected_key_recovery")
    recovery_ms = min(fresh, default=None)
    if recovery_ms is None or recovery_ms > RECOVERY_MS:
        failures.add("late_or_missing_recovery")
    total = sum(map(len, latencies.values()))
    return start, start + end, {
        "id": round_id, "recovery_ms": recovery_ms, "drain_after_stop_ms": None if drain is None else drain - stop,
        "longest_queue_ms": max(queue_ms, default=None), "peaks": server["peaks"], "unfinished_at_peak": occupied,
        "accepted": len(ledger), "independent_acceptance": evidence,
        "unexpected_error_rate": unexpected / total if total else None,
        "requests_without_response": no_response,
        "timings": {code: timings(values, len(values)) for code, values in latencies.items()},
        "failures": sorted(failures),
    }


def assess(client, server, server_digest, observations=()):
    exact(client, "schema origin_scope release_sha256 topology_sha256 config_sha256 ledger_sha256 global_limit rounds")
    exact(server, "schema release_sha256 topology_sha256 config_sha256 rounds")
    if (client["schema"] != "oj.p6-overload.v1" or server["schema"] != "oj.p6-overload-ledger.v1"
            or client["origin_scope"] not in ("local", "trial") or client["ledger_sha256"] != server_digest):
        raise ValueError("unsupported scope or ledger digest mismatch")
    if len(observations) not in (0, 3) or (client["origin_scope"] == "trial" and len(observations) != 3):
        raise ValueError("expected three independent observations for trial")
    for key in ("release_sha256", "topology_sha256", "config_sha256"):
        if label(client[key], SHA) != server[key]:
            raise ValueError("release, topology or configuration mismatch")
    limit = number(client["global_limit"], 1, MAX_ROWS)
    client_rounds, server_rounds = rows(client["rounds"]), rows(server["rounds"])
    if len(client_rounds) != 3 or len(server_rounds) != 3:
        raise ValueError("expected three complete rounds")
    seen = set()
    rounds = sorted((assess_round(a, b, limit, observations, index, seen)
                     for index, (a, b) in enumerate(zip(client_rounds, server_rounds))),
                    key=lambda result: result[0])
    if (len({report["id"] for _, _, report in rounds}) != 3
            or any(a[1] > b[0] for a, b in zip(rounds, rounds[1:]))):
        raise ValueError("duplicate or overlapping rounds")
    reports = [report for _, _, report in rounds]
    return {"scope": "recorded_overload_only", "origin_scope": client["origin_scope"], "rounds": reports,
            "release_sha256": client["release_sha256"], "topology_sha256": client["topology_sha256"],
            "config_sha256": client["config_sha256"], "global_limit": limit,
            "meets_recorded_thresholds": all(not item["failures"] for item in reports)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("client_record", type=Path)
    parser.add_argument("server_ledger", type=Path)
    parser.add_argument("--acceptance", type=Path, nargs=3, metavar="OBSERVATION",
                        help="three raw training-acceptance exports in record order; required for trial")
    args = parser.parse_args()
    try:
        client, client_sha = read(args.client_record)
        server, server_sha = read(args.server_ledger)
        result = assess(client, server, server_sha, [read(path) for path in args.acceptance or ()])
    except (OSError, ValueError, RecursionError):
        print("P6 overload assessment refused: invalid or unbound record", file=sys.stderr)
        return 2
    print(json.dumps({**result, "client_sha256": client_sha, "ledger_sha256": server_sha}, sort_keys=True))
    return 0 if result["meets_recorded_thresholds"] else 1


if __name__ == "__main__":
    sys.exit(main())
