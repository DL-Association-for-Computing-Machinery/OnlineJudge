#!/usr/bin/env python3
"""Assess the timing and completeness of an operator-recorded M1 restore."""

import argparse
from datetime import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import sys


CHECKS = ("source_isolated", "materials_verified", "old_credentials_rejected",
          "service_restored", "historical_submission_rejudged")
SHA = re.compile(r"[0-9a-f]{64}")
IDENTIFIER = re.compile(r"[a-zA-Z0-9_.-]{1,80}")
UTC = re.compile(r"[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(?:\.[0-9]{1,9})?Z")
SECOND = 10**9


def unique_pairs(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate JSON field")
        result[key] = value
    return result


def read_file(path, limit, *, dir_fd=None):
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=dir_fd)
    with os.fdopen(fd, "rb") as stream:
        if not stat.S_ISREG(os.fstat(fd).st_mode):
            raise ValueError("expected regular local file")
        raw = stream.read(limit + 1)
    if not 0 < len(raw) <= limit:
        raise ValueError("empty or oversized evidence file")
    return raw


def read_json(path, limit):
    raw = read_file(path, limit)
    value = json.loads(raw, object_pairs_hook=unique_pairs)
    if not isinstance(value, dict):
        raise ValueError("expected JSON object")
    return raw, value


def timestamp(value):
    if not isinstance(value, str) or not UTC.fullmatch(value):
        raise ValueError("expected UTC timestamp")
    seconds, _, fraction = value[:-1].partition(".")
    # Go manifests can include nanoseconds. Keep threshold comparisons exact.
    moment = datetime.fromisoformat(seconds)
    elapsed = moment - datetime(1970, 1, 1)
    return (elapsed.days * 86400 + elapsed.seconds) * SECOND + int(fraction.ljust(9, "0"))


def valid(value, pattern):
    return isinstance(value, str) and pattern.fullmatch(value) is not None


def assess(record, manifest, manifest_sha, evidence_dir):
    expected = {"format", "source_domain", "backup_domain", "restore_domain", "manifest_sha256",
                "offsite_verified_at", "fault_declared_at", "checks"}
    if set(record) != expected or record["format"] != "oj.m1-restore-evidence.v1":
        raise ValueError("unexpected restore evidence fields")
    if not valid(record["manifest_sha256"], SHA) or record["manifest_sha256"] != manifest_sha:
        raise ValueError("backup manifest digest mismatch")
    if manifest.get("format") != "oj.logical-backup.v1":
        raise ValueError("unexpected backup manifest format")
    domains = [record[key] for key in ("source_domain", "backup_domain", "restore_domain")]
    if not all(valid(domain, IDENTIFIER) for domain in domains):
        raise ValueError("expected stable domain identifiers")
    captured = timestamp(manifest.get("captured_at"))
    offsite = timestamp(record["offsite_verified_at"])
    fault = timestamp(record["fault_declared_at"])
    if not captured <= offsite <= fault:
        raise ValueError("snapshot must be verified offsite by the fault declaration")

    checks = record["checks"]
    if not isinstance(checks, dict) or set(checks) != set(CHECKS):
        raise ValueError("all five observations are required")
    checked_at = {}
    evidence_digests = set()
    failures = []
    for name in CHECKS:
        check = checks[name]
        if (not isinstance(check, dict) or set(check) != {"passed", "at", "evidence_sha256"}
                or type(check["passed"]) is not bool or not valid(check["evidence_sha256"], SHA)):
            raise ValueError("invalid observation")
        raw = read_file(name + ".txt", 1 << 20, dir_fd=evidence_dir)
        digest = hashlib.sha256(raw).hexdigest()
        if digest != check["evidence_sha256"] or digest in evidence_digests:
            raise ValueError("mismatched or reused observation")
        evidence_digests.add(digest)
        at = timestamp(check["at"])
        if name != "source_isolated" and at < fault:
            raise ValueError("observations must follow the fault declaration")
        checked_at[name] = at
        if not check["passed"]:
            failures.append(name)

    if domains[0].lower() in (domain.lower() for domain in domains[1:]):
        failures.append("source_failure_domain_shared")
    if checked_at["source_isolated"] > min(checked_at.values()):
        failures.append("source_isolation_too_late")
    if checked_at["materials_verified"] > checked_at["service_restored"] or checked_at["service_restored"] > checked_at["historical_submission_rejudged"]:
        failures.append("restore_observations_out_of_order")
    if checked_at["old_credentials_rejected"] > checked_at["service_restored"]:
        failures.append("old_credentials_checked_after_restore")
    rpo = fault - captured
    rto = max(checked_at.values()) - fault
    if rpo > 24 * 60 * 60 * SECOND:
        failures.append("rpo_exceeded")
    if rto > 4 * 60 * 60 * SECOND:
        failures.append("rto_exceeded")
    return {"format": "oj.m1-restore-assessment.v1", "scope": "recorded_evidence_only",
            "manifest_sha256": manifest_sha, "rpo_seconds": rpo / SECOND, "rto_seconds": rto / SECOND,
            "eligible_for_review": not failures, "failures": failures}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("record", type=Path)
    parser.add_argument("manifest", type=Path)
    parser.add_argument("observations", type=Path, help="directory holding the five original, sanitized observation files")
    args = parser.parse_args()
    try:
        _, record = read_json(args.record, 64 << 10)
        raw, manifest = read_json(args.manifest, 16 << 10)
        evidence_dir = os.open(args.observations, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        try:
            result = assess(record, manifest, hashlib.sha256(raw).hexdigest(), evidence_dir)
        finally:
            os.close(evidence_dir)
    except (OSError, ValueError, RecursionError):
        print("M1 restore assessment refused: invalid or inconsistent evidence", file=sys.stderr)
        return 2
    print(json.dumps(result, sort_keys=True))
    return 0 if result["eligible_for_review"] else 1


if __name__ == "__main__":
    sys.exit(main())
